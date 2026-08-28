package config

import (
	"strings"
	"testing"
)

func env(values map[string]string) func(string) string {
	return func(key string) string {
		return values[key]
	}
}

func TestJWTSecret(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "missing", wantErr: true},
		{name: "short", value: "too-short", wantErr: true},
		{name: "known default", value: "default_secret_change_me", wantErr: true},
		{name: "strong", value: strings.Repeat("a9", 24), wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := JWTSecret(env(map[string]string{"JWT_SECRET": tt.value}))
			if tt.wantErr && err == nil {
				t.Fatal("JWTSecret() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("JWTSecret() error = %v", err)
			}
			if !tt.wantErr && got != tt.value {
				t.Fatalf("JWTSecret() = %q, want %q", got, tt.value)
			}
		})
	}
}

func TestBootstrapCredentials(t *testing.T) {
	tests := []struct {
		name     string
		username string
		password string
		wantErr  bool
	}{
		{name: "missing username", password: "a-strong-password-123", wantErr: true},
		{name: "missing password", username: "operator", wantErr: true},
		{name: "known default", username: "operator", password: "admin", wantErr: true},
		{name: "short password", username: "operator", password: "short", wantErr: true},
		{name: "password matches username", username: "sixteen-characters", password: "sixteen-characters", wantErr: true},
		{name: "strong", username: "operator", password: "a-strong-password-123", wantErr: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			username, password, err := BootstrapCredentials(env(map[string]string{
				"ADMIN_USER":     tt.username,
				"ADMIN_PASSWORD": tt.password,
			}))
			if tt.wantErr && err == nil {
				t.Fatal("BootstrapCredentials() error = nil, want error")
			}
			if !tt.wantErr && err != nil {
				t.Fatalf("BootstrapCredentials() error = %v", err)
			}
			if !tt.wantErr && (username != tt.username || password != tt.password) {
				t.Fatalf("BootstrapCredentials() = %q, %q", username, password)
			}
		})
	}
}

func TestValidatePassword(t *testing.T) {
	for _, password := range []string{"", "short", "admin"} {
		if err := ValidatePassword(password); err == nil {
			t.Fatalf("ValidatePassword(%q) error = nil", password)
		}
	}
	if err := ValidatePassword("a-strong-password-123"); err != nil {
		t.Fatalf("ValidatePassword() error = %v", err)
	}
}

func TestAllowedOrigins(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
	}{
		{name: "same origin default"},
		{name: "explicit origins", value: "https://example.com,http://localhost:8080"},
		{name: "wildcard", value: "*", wantErr: true},
		{name: "path", value: "https://example.com/app", wantErr: true},
		{name: "non HTTP scheme", value: "file://example.com", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := AllowedOrigins(env(map[string]string{"ALLOWED_ORIGINS": tt.value}))
			if (err != nil) != tt.wantErr {
				t.Fatalf("AllowedOrigins() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestTrustedProxyCIDRs(t *testing.T) {
	if _, err := TrustedProxyCIDRs(env(nil), true); err == nil {
		t.Fatal("TrustedProxyCIDRs() accepted enabled proxy trust without CIDRs")
	}
	if _, err := TrustedProxyCIDRs(env(map[string]string{"TRUSTED_PROXY_CIDRS": "not-a-cidr"}), true); err == nil {
		t.Fatal("TrustedProxyCIDRs() accepted an invalid CIDR")
	}
	prefixes, err := TrustedProxyCIDRs(env(map[string]string{"TRUSTED_PROXY_CIDRS": "10.0.0.9/8"}), true)
	if err != nil {
		t.Fatalf("TrustedProxyCIDRs() error = %v", err)
	}
	if got := prefixes[0].String(); got != "10.0.0.0/8" {
		t.Fatalf("TrustedProxyCIDRs() = %q, want masked prefix", got)
	}
}
