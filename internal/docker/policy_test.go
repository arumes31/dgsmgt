package docker

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	v1 "github.com/opencontainers/image-spec/specs-go/v1"
)

// fakeFS resolves the given paths as if they existed on the host; any other
// path reports fs.ErrNotExist.
func fakeFS(existing map[string]string) func(string) (string, error) {
	return func(name string) (string, error) {
		if target, ok := existing[name]; ok {
			return target, nil
		}
		return "", os.ErrNotExist
	}
}

func TestPolicyValidateVolumes(t *testing.T) {
	games := fakeFS(map[string]string{"/srv/games": "/srv/games", "/srv/games/a": "/srv/games/a"})
	tests := []struct {
		name    string
		policy  Policy
		volume  string
		wantErr bool
	}{
		{"named volume", Policy{}, "gamedata:/data", false},
		{"named volume read-only", Policy{}, "gamedata:/data:ro", false},
		{"anonymous volume", Policy{}, "/data", false},
		{"host bind disabled by default", Policy{}, "/srv/games/a:/data", true},
		{"host root", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/:/host", true},
		{"docker socket", Policy{BindRoot: "/var/run"}, "/var/run/docker.sock:/var/run/docker.sock", true},
		{"socket ancestor via /var/run root", Policy{BindRoot: "/var/run", evalSymlinks: fakeFS(map[string]string{"/var/run": "/run"})}, "/var/run:/data", true},
		{"socket ancestor via /var root", Policy{BindRoot: "/var", evalSymlinks: fakeFS(map[string]string{"/var": "/var"})}, "/var:/data", true},
		{"root inside docker data", Policy{BindRoot: "/var/lib/docker/volumes", evalSymlinks: fakeFS(map[string]string{"/var/lib/docker/volumes": "/var/lib/docker/volumes"})}, "/var/lib/docker/volumes/x:/data", true},
		{"root aliasing protected dir", Policy{BindRoot: "/srv/run", evalSymlinks: fakeFS(map[string]string{"/srv/run": "/run", "/srv/run/x": "/run/x"})}, "/srv/run/x:/data", true},
		{"socket file name", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/srv/games/containerd.sock:/s", true},
		{"inside bind root", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/srv/games/a:/data", false},
		{"missing subdirectory inside bind root", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/srv/games/new/world:/data", false},
		{"bind root itself", Policy{BindRoot: "/srv/games/", evalSymlinks: games}, "/srv/games:/data", false},
		{"bind root not visible to panel", Policy{BindRoot: "/srv/games", evalSymlinks: fakeFS(nil)}, "/srv/games/a:/data", true},
		{"symlink escaping bind root", Policy{BindRoot: "/srv/games", evalSymlinks: fakeFS(map[string]string{"/srv/games": "/srv/games", "/srv/games/evil": "/etc"})}, "/srv/games/evil:/data", true},
		{"symlink to socket", Policy{BindRoot: "/srv/games", evalSymlinks: fakeFS(map[string]string{"/srv/games": "/srv/games", "/srv/games/s": "/srv/games/x/docker.sock"})}, "/srv/games/s:/data", true},
		{"traversal out of bind root", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/srv/games/../../etc:/data", true},
		{"sibling prefix", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "/srv/games2:/data", true},
		{"slash bind root is ignored", Policy{BindRoot: "/"}, "/etc:/data", true},
		{"relative host path", Policy{BindRoot: "/srv/games", evalSymlinks: games}, "./data:/data", true},
		{"relative container path", Policy{}, "gamedata:data", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.policy.ValidateVolumes([]string{tt.volume})
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateVolumes(%q) error = %v, wantErr %v", tt.volume, err, tt.wantErr)
			}
		})
	}
}

func TestPolicyResolvesRealSymlinks(t *testing.T) {
	base, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	base = filepath.ToSlash(base)
	if !strings.HasPrefix(base, "/") {
		t.Skip("POSIX paths required")
	}
	root := base + "/games"
	outside := base + "/outside"
	for _, dir := range []string{root + "/world", outside} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Symlink(outside, root+"/escape"); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	if err := os.Symlink(root+"/world", root+"/alias"); err != nil {
		t.Fatal(err)
	}
	p := Policy{BindRoot: root}
	if err := p.Validate(); err != nil {
		t.Fatalf("Validate() = %v", err)
	}
	if _, err := p.ResolveVolumes([]string{root + "/escape:/data"}); err == nil {
		t.Error("escaping symlink must be rejected")
	}
	if _, err := p.ResolveVolumes([]string{root + "/escape/sub:/data"}); err == nil {
		t.Error("missing path below an escaping symlink must be rejected")
	}
	if err := os.Symlink(base+"/does-not-exist-yet", root+"/dangling"); err != nil {
		t.Fatal(err)
	}
	if _, err := p.ResolveVolumes([]string{root + "/dangling:/data"}); err == nil {
		t.Error("dangling symlink must be rejected")
	}
	if _, err := p.ResolveVolumes([]string{root + "/dangling/sub:/data"}); err == nil {
		t.Error("path below a dangling symlink must be rejected")
	}
	got, err := p.ResolveVolumes([]string{root + "/alias:/data:ro"})
	if err != nil {
		t.Fatal(err)
	}
	if want := root + "/world:/data:ro"; got[0] != want {
		t.Errorf("ResolveVolumes() = %q, want %q", got[0], want)
	}
}

func TestPolicyValidateConfig(t *testing.T) {
	for root, ok := range map[string]bool{
		"":                true,
		"/srv/games":      true,
		"relative":        false,
		"/":               false,
		"/run":            false,
		"/var":            false,
		"/var/run/docker": false,
		"/dev/shm":        false,
	} {
		p := Policy{BindRoot: root, evalSymlinks: func(s string) (string, error) { return s, nil }}
		if err := p.Validate(); (err == nil) != ok {
			t.Errorf("Validate(%q) = %v, want ok=%v", root, err, ok)
		}
	}
}

func TestPolicyValidateImage(t *testing.T) {
	open := Policy{}
	if err := open.ValidateImage("anything/at:all"); err != nil {
		t.Errorf("empty allow-list must allow any image: %v", err)
	}
	if err := open.ValidateImage("Not A Reference"); err == nil {
		t.Error("invalid references must be rejected")
	}

	p := Policy{AllowedImagePrefixes: []string{"docker.io/library/", "ghcr.io/myorg/"}}
	for img, ok := range map[string]bool{
		"alpine":                     true,
		"library/alpine:3":           true,
		"docker.io/library/alpine":   true,
		"ghcr.io/myorg/game:latest":  true,
		"GHCR.io/myorg/game":         true,
		"evil/alpine":                false,
		"ghcr.io/other/game":         false,
		"registry.example.com/image": false,
		// Docker treats an uppercase first component as a registry host
		"Library/alpine":     false,
		"LIBRARY/alpine:3":   false,
		"ghcr.io/MyOrg/game": false,
	} {
		if err := p.ValidateImage(img); (err == nil) != ok {
			t.Errorf("ValidateImage(%q) = %v, want allowed=%v", img, err, ok)
		}
	}
}

func TestValidatePorts(t *testing.T) {
	tooMany := make([]string, maxPortSpecs+1)
	for i := range tooMany {
		tooMany[i] = fmt.Sprintf("%d", 10000+i)
	}
	tests := []struct {
		name    string
		ports   []string
		wantErr bool
	}{
		{"none", nil, false},
		{"simple", []string{"8080:80", "8777:8777/udp", "25565"}, false},
		{"ip", []string{"127.0.0.1:8080:80/tcp", "[::1]:8081:81"}, false},
		{"empty host port", []string{"127.0.0.1::80"}, false},
		{"small range", []string{"7000-7010:7000-7010/udp"}, false},
		{"large range", []string{"1-65535:1-65535"}, true},
		{"aggregate over budget", []string{"1000-1199:1000-1199", "2000-2099:2000-2099"}, true},
		{"duplicate spec", []string{"8080:80", "8080:80"}, true},
		{"overlapping ranges", []string{"7000-7010:7000-7010", "7005:7005"}, true},
		{"too many specs", tooMany, true},
		{"invalid", []string{"abc"}, true},
		{"invalid host", []string{"x:80"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := ValidatePorts(tt.ports); (err != nil) != tt.wantErr {
				t.Errorf("ValidatePorts(%v) = %v, wantErr %v", tt.ports, err, tt.wantErr)
			}
		})
	}
}

func TestCreateUsesValidatedImageAndRejectsBeforePull(t *testing.T) {
	var pulled, created string
	mock := &mockDockerClient{
		pullFunc: func(ctx context.Context, ref string, options image.PullOptions) (io.ReadCloser, error) {
			pulled = ref
			return io.NopCloser(strings.NewReader("")), nil
		},
		createFunc: func(ctx context.Context, config *container.Config, hostConfig *container.HostConfig, networkingConfig *network.NetworkingConfig, platform *v1.Platform, containerName string) (container.CreateResponse, error) {
			created = config.Image
			return container.CreateResponse{ID: "id"}, nil
		},
	}
	svc := NewServiceWithClient(mock)
	svc.SetPolicy(Policy{AllowedImagePrefixes: []string{"docker.io/library/"}})
	if _, err := svc.Create(context.Background(), "srv", "alpine:3", []string{"8080:80"}, nil, nil); err != nil {
		t.Fatal(err)
	}
	if pulled != "docker.io/library/alpine:3" || created != pulled {
		t.Errorf("pulled %q, created %q; want the canonical reference for both", pulled, created)
	}

	pulled = ""
	if _, err := svc.Create(context.Background(), "srv", "alpine", []string{"1-65535:1-65535"}, nil, nil); err == nil {
		t.Error("oversized port ranges must be rejected")
	}
	if pulled != "" {
		t.Error("the image must not be pulled before ports are validated")
	}
}
