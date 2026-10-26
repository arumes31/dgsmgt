package docker

import (
	"fmt"
	"path"
	"regexp"
	"strings"
)

// Policy restricts what an admin can request when creating a container.
// Admin access to the panel must not automatically mean root on the Docker host.
type Policy struct {
	// BindRoot is the only host directory (and its subdirectories) that may be
	// bind mounted. Empty disables host bind mounts; named volumes still work.
	BindRoot string
	// AllowedImagePrefixes limits images to these normalized prefixes
	// (e.g. "docker.io/library/", "ghcr.io/myorg/"). Empty allows any image.
	AllowedImagePrefixes []string
}

var volumeNameRe = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]*$`)

// ValidateVolumes checks every volume spec ("src:dst[:mode]" or "dst") against the policy.
func (p Policy) ValidateVolumes(volumes []string) error {
	for _, v := range volumes {
		if err := p.validateVolume(v); err != nil {
			return err
		}
	}
	return nil
}

func (p Policy) validateVolume(spec string) error {
	parts := strings.Split(spec, ":")
	if len(parts) < 1 || len(parts) > 3 || parts[0] == "" {
		return fmt.Errorf("invalid volume spec %q", spec)
	}
	if len(parts) == 1 {
		// Anonymous volume, only a container path
		if !strings.HasPrefix(parts[0], "/") {
			return fmt.Errorf("invalid volume spec %q", spec)
		}
		return nil
	}

	src, dst := parts[0], parts[1]
	if !strings.HasPrefix(dst, "/") {
		return fmt.Errorf("invalid container path in volume spec %q", spec)
	}
	if !strings.HasPrefix(src, "/") {
		if !volumeNameRe.MatchString(src) {
			return fmt.Errorf("volume %q is neither a named volume nor an absolute host path", src)
		}
		return nil
	}

	cleaned := path.Clean(src)
	if strings.Contains(cleaned, "docker.sock") {
		return fmt.Errorf("mounting the Docker socket is not allowed")
	}
	root := p.bindRoot()
	if root == "" {
		return fmt.Errorf("host bind mounts are disabled (set ALLOWED_BIND_ROOT to allow a base directory): %q", src)
	}
	if cleaned != root && !strings.HasPrefix(cleaned, root+"/") {
		return fmt.Errorf("host path %q is outside the allowed bind root %q", src, root)
	}
	return nil
}

func (p Policy) bindRoot() string {
	if p.BindRoot == "" || !strings.HasPrefix(p.BindRoot, "/") {
		return ""
	}
	root := path.Clean(p.BindRoot)
	if root == "/" {
		// Allowing "/" would allow everything
		return ""
	}
	return root
}

// ValidateImage checks the image reference against the allowed prefixes.
func (p Policy) ValidateImage(ref string) error {
	if len(p.AllowedImagePrefixes) == 0 {
		return nil
	}
	normalized := normalizeImageRef(ref)
	for _, prefix := range p.AllowedImagePrefixes {
		if prefix != "" && strings.HasPrefix(normalized, normalizeImageRef(prefix)) {
			return nil
		}
	}
	return fmt.Errorf("image %q is not from an allowed registry", ref)
}

// normalizeImageRef expands short references like "nginx" to "docker.io/library/nginx".
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
