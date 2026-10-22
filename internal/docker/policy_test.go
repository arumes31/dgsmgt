package docker

import "testing"

func TestPolicyValidateVolumes(t *testing.T) {
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
		{"host root", Policy{BindRoot: "/srv/games"}, "/:/host", true},
		{"docker socket", Policy{BindRoot: "/var/run"}, "/var/run/docker.sock:/var/run/docker.sock", true},
		{"inside bind root", Policy{BindRoot: "/srv/games"}, "/srv/games/a:/data", false},
		{"bind root itself", Policy{BindRoot: "/srv/games/"}, "/srv/games:/data", false},
		{"traversal out of bind root", Policy{BindRoot: "/srv/games"}, "/srv/games/../../etc:/data", true},
		{"sibling prefix", Policy{BindRoot: "/srv/games"}, "/srv/games2:/data", true},
		{"slash bind root is ignored", Policy{BindRoot: "/"}, "/etc:/data", true},
		{"relative host path", Policy{BindRoot: "/srv/games"}, "./data:/data", true},
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

func TestPolicyValidateImage(t *testing.T) {
	open := Policy{}
	if err := open.ValidateImage("anything/at:all"); err != nil {
		t.Errorf("empty allow-list must allow any image: %v", err)
	}

	p := Policy{AllowedImagePrefixes: []string{"docker.io/library/", "ghcr.io/myorg/"}}
	for img, ok := range map[string]bool{
		"alpine":                     true,
		"library/alpine:3":           true,
		"docker.io/library/alpine":   true,
		"ghcr.io/myorg/game:latest":  true,
		"evil/alpine":                false,
		"ghcr.io/other/game":         false,
		"registry.example.com/image": false,
	} {
		if err := p.ValidateImage(img); (err == nil) != ok {
			t.Errorf("ValidateImage(%q) = %v, want allowed=%v", img, err, ok)
		}
	}
}
