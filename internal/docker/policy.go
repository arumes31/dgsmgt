package docker

import (
	"errors"
	"fmt"
	"io/fs"
	"path"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/distribution/reference"
	"github.com/docker/go-connections/nat"
)

const (
	// maxPortSpecs bounds the number of port specifications per container.
	maxPortSpecs = 64
	// maxPortMappings bounds the total number of container ports a request may
	// expand to, so compact ranges cannot make the parser allocate huge maps.
	maxPortMappings = 256
)

// protectedHostPaths hold container runtime control sockets or kernel
// interfaces. A bind root that is one of them, an ancestor of one of them or
// inside one of them could expose daemon authority through a directory mount.
var protectedHostPaths = []string{
	"/run",
	"/var/run",
	"/var/lib/docker",
	"/var/lib/containerd",
	"/proc",
	"/sys",
	"/dev",
}

// Policy restricts what an admin can request when creating a container.
// Admin access to the panel must not automatically mean root on the Docker host.
type Policy struct {
	// BindRoot is the only host directory (and its subdirectories) that may be
	// bind mounted. Empty disables host bind mounts; named volumes still work.
	// The directory must be mounted into the panel at the same path so that bind
	// sources can be resolved (symlinks included) before they are handed to Docker.
	BindRoot string
	// AllowedImagePrefixes limits images to these normalized prefixes
	// (e.g. "docker.io/library/", "ghcr.io/myorg/"). Empty allows any image.
	AllowedImagePrefixes []string

	// evalSymlinks resolves a host path; nil uses the panel's filesystem.
	evalSymlinks func(string) (string, error)
}

var volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// Validate checks the policy configuration itself. It fails closed when the
// bind root could expose protected host interfaces or cannot be verified.
func (p Policy) Validate() error {
	if p.BindRoot == "" {
		return nil
	}
	if !strings.HasPrefix(p.BindRoot, "/") {
		return fmt.Errorf("ALLOWED_BIND_ROOT must be an absolute path")
	}
	root := path.Clean(p.BindRoot)
	if root == "/" {
		return fmt.Errorf("ALLOWED_BIND_ROOT must not be '/'")
	}
	if overlapsProtectedPath(root) {
		return fmt.Errorf("ALLOWED_BIND_ROOT %q overlaps a directory holding container runtime sockets or kernel interfaces; use a dedicated data directory", root)
	}
	resolved, err := p.resolve(root)
	if err != nil {
		return fmt.Errorf("ALLOWED_BIND_ROOT %q must be mounted into the panel at the same path so bind sources can be verified: %w", root, err)
	}
	if resolved == "/" || overlapsProtectedPath(resolved) {
		return fmt.Errorf("ALLOWED_BIND_ROOT %q resolves to %q, which overlaps protected host directories", root, resolved)
	}
	return nil
}

// overlapsProtectedPath reports whether p equals, contains or lies inside a protected path.
func overlapsProtectedPath(p string) bool {
	for _, protected := range protectedHostPaths {
		if within(p, protected) || within(protected, p) {
			return true
		}
	}
	return false
}

// within reports whether child equals parent or lies beneath it (both cleaned).
func within(child, parent string) bool {
	if parent == "/" {
		return strings.HasPrefix(child, "/")
	}
	return child == parent || strings.HasPrefix(child, parent+"/")
}

func (p Policy) resolve(name string) (string, error) {
	if p.evalSymlinks != nil {
		return p.evalSymlinks(name)
	}
	resolved, err := filepath.EvalSymlinks(name)
	if err != nil {
		return "", err
	}
	return filepath.ToSlash(resolved), nil
}

// ValidateVolumes checks every volume spec ("src:dst[:mode]" or "dst") against the policy.
func (p Policy) ValidateVolumes(volumes []string) error {
	_, err := p.ResolveVolumes(volumes)
	return err
}

// ResolveVolumes validates every volume spec and returns the specs to hand to
// Docker, with host bind sources replaced by their resolved, verified paths.
func (p Policy) ResolveVolumes(volumes []string) ([]string, error) {
	out := make([]string, 0, len(volumes))
	for _, v := range volumes {
		resolved, err := p.resolveVolume(v)
		if err != nil {
			return nil, err
		}
		out = append(out, resolved)
	}
	return out, nil
}

func (p Policy) resolveVolume(spec string) (string, error) {
	parts := strings.Split(spec, ":")
	if len(parts) < 1 || len(parts) > 3 || parts[0] == "" {
		return "", fmt.Errorf("invalid volume spec %q", spec)
	}
	if len(parts) == 1 {
		// Anonymous volume, only a container path
		if !strings.HasPrefix(parts[0], "/") {
			return "", fmt.Errorf("invalid volume spec %q", spec)
		}
		return spec, nil
	}

	src, dst := parts[0], parts[1]
	if !strings.HasPrefix(dst, "/") {
		return "", fmt.Errorf("invalid container path in volume spec %q", spec)
	}
	if !strings.HasPrefix(src, "/") {
		if !volumeNameRe.MatchString(src) {
			return "", fmt.Errorf("volume %q is neither a named volume nor an absolute host path", src)
		}
		return spec, nil
	}

	cleaned := path.Clean(src)
	if containsSocketName(cleaned) {
		return "", fmt.Errorf("mounting container runtime sockets is not allowed")
	}
	if p.BindRoot == "" {
		return "", fmt.Errorf("host bind mounts are disabled (set ALLOWED_BIND_ROOT to allow a base directory): %q", src)
	}
	if err := p.Validate(); err != nil {
		// Fail closed: a root that cannot keep sockets out disables host binds
		return "", fmt.Errorf("host bind mounts are disabled: %w", err)
	}
	root := path.Clean(p.BindRoot)
	if !within(cleaned, root) {
		return "", fmt.Errorf("host path %q is outside the allowed bind root %q", src, root)
	}

	// Resolve symlinks on the shared filesystem so a link planted beneath the
	// root (for example by a workload with a writable mount) cannot redirect
	// the bind outside of it. Docker receives the resolved path.
	resolvedRoot, err := p.resolve(root)
	if err != nil {
		return "", fmt.Errorf("allowed bind root %q is not accessible to the panel: %w", root, err)
	}
	resolved, err := p.resolveExisting(cleaned)
	if err != nil {
		return "", fmt.Errorf("cannot verify host path %q: %w", src, err)
	}
	if !within(resolved, resolvedRoot) {
		return "", fmt.Errorf("host path %q resolves outside the allowed bind root %q", src, root)
	}
	if containsSocketName(resolved) {
		return "", fmt.Errorf("mounting container runtime sockets is not allowed")
	}
	parts[0] = resolved
	return strings.Join(parts, ":"), nil
}

// resolveExisting resolves the longest existing prefix of name and appends the
// missing components, which Docker creates as plain directories.
func (p Policy) resolveExisting(name string) (string, error) {
	current := name
	var missing []string
	for {
		resolved, err := p.resolve(current)
		if err == nil {
			return path.Join(append([]string{resolved}, missing...)...), nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return "", err
		}
		parent := path.Dir(current)
		if parent == current {
			return "", err
		}
		missing = append([]string{path.Base(current)}, missing...)
		current = parent
	}
}

// containsSocketName rejects paths naming a runtime control socket directly.
func containsSocketName(p string) bool {
	for _, component := range strings.Split(p, "/") {
		if strings.HasSuffix(strings.ToLower(component), ".sock") {
			return true
		}
	}
	return false
}

// ValidatePorts bounds the number of port specifications and the mappings they
// expand to, and rejects duplicate bindings, before anything is allocated.
func ValidatePorts(ports []string) error {
	if len(ports) > maxPortSpecs {
		return fmt.Errorf("too many port specifications (max %d)", maxPortSpecs)
	}
	total := 0
	seen := make(map[string]struct{}, len(ports))
	for _, spec := range ports {
		spec = strings.TrimSpace(spec)
		if _, dup := seen[spec]; dup {
			return fmt.Errorf("duplicate port specification %q", spec)
		}
		seen[spec] = struct{}{}
		count, err := portSpecMappings(spec)
		if err != nil {
			return err
		}
		total += count
		if total > maxPortMappings {
			return fmt.Errorf("port specifications expand to more than %d mappings", maxPortMappings)
		}
	}
	// The budget is small now, so the real parser is cheap; use it to catch
	// overlapping ranges that produce duplicate bindings.
	_, bindings, err := nat.ParsePortSpecs(ports)
	if err != nil {
		return fmt.Errorf("parsing ports: %w", err)
	}
	for port, list := range bindings {
		unique := make(map[nat.PortBinding]struct{}, len(list))
		for _, b := range list {
			if _, dup := unique[b]; dup {
				return fmt.Errorf("duplicate binding for port %s", port)
			}
			unique[b] = struct{}{}
		}
	}
	return nil
}

// portSpecMappings returns how many container ports "[ip:][hostPort:]containerPort[/proto]" covers.
func portSpecMappings(spec string) (int, error) {
	rest, _, _ := strings.Cut(spec, "/")
	if strings.HasPrefix(rest, "[") {
		// Bracketed IPv6 host address
		end := strings.Index(rest, "]:")
		if end < 0 {
			return 0, fmt.Errorf("invalid port specification %q", spec)
		}
		rest = rest[end+2:]
	}
	parts := strings.Split(rest, ":")
	if len(parts) > 3 {
		return 0, fmt.Errorf("invalid port specification %q", spec)
	}
	containerPart := parts[len(parts)-1]
	if containerPart == "" {
		return 0, fmt.Errorf("invalid port specification %q", spec)
	}
	start, end, err := nat.ParsePortRangeToInt(containerPart)
	if err != nil {
		return 0, fmt.Errorf("invalid port specification %q", spec)
	}
	if len(parts) >= 2 && parts[len(parts)-2] != "" {
		if _, _, err := nat.ParsePortRangeToInt(parts[len(parts)-2]); err != nil {
			return 0, fmt.Errorf("invalid port specification %q", spec)
		}
	}
	return end - start + 1, nil
}

// CanonicalImage parses ref exactly as Docker does, checks the resulting
// registry and repository against the allowed prefixes and returns the
// normalized reference that must be used for pulling and creating.
func (p Policy) CanonicalImage(ref string) (string, error) {
	named, err := reference.ParseNormalizedNamed(strings.TrimSpace(ref))
	if err != nil {
		return "", fmt.Errorf("invalid image reference %q: %w", ref, err)
	}
	canonical := named.String()
	if len(p.AllowedImagePrefixes) == 0 {
		return canonical, nil
	}
	// Registry host names are case-insensitive; repository paths are lowercase by grammar
	compare := strings.ToLower(canonical)
	for _, prefix := range p.AllowedImagePrefixes {
		if prefix != "" && strings.HasPrefix(compare, normalizeImageRef(prefix)) {
			return canonical, nil
		}
	}
	return "", fmt.Errorf("image %q is not from an allowed registry", ref)
}

// ValidateImage checks the image reference against the allowed prefixes.
func (p Policy) ValidateImage(ref string) error {
	_, err := p.CanonicalImage(ref)
	return err
}

// normalizeImageRef expands short prefixes like "library/" to "docker.io/library/".
func normalizeImageRef(ref string) string {
	ref = strings.ToLower(strings.TrimSpace(ref))
	first, rest, hasSlash := strings.Cut(ref, "/")
	if hasSlash && (strings.ContainsAny(first, ".:") || first == "localhost") {
		return ref
	}
	if !hasSlash {
		return "docker.io/library/" + ref
	}
	return "docker.io/" + first + "/" + rest
}
