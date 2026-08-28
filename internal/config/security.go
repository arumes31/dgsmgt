package config

import (
	"errors"
	"fmt"
	"net/netip"
	"net/url"
	"strings"
	"unicode/utf8"
)

const (
	minJWTSecretBytes = 32
	minAdminPassword  = 16
)

var insecureValues = map[string]struct{}{
	"admin":                    {},
	"changeme":                 {},
	"change_me":                {},
	"change_me_in_production":  {},
	"default_secret_change_me": {},
	"replace_me_with_a_long_random_string_in_production": {},
}

// JWTSecret returns a deployment-specific signing key or an error. The server
// must never silently substitute a source-known key.
func JWTSecret(getenv func(string) string) (string, error) {
	secret := strings.TrimSpace(getenv("JWT_SECRET"))
	if secret == "" {
		return "", errors.New("JWT_SECRET is required")
	}
	if len(secret) < minJWTSecretBytes {
		return "", fmt.Errorf("JWT_SECRET must contain at least %d bytes", minJWTSecretBytes)
	}
	if _, insecure := insecureValues[strings.ToLower(secret)]; insecure {
		return "", errors.New("JWT_SECRET uses a known insecure value")
	}
	return secret, nil
}

// BootstrapCredentials returns the one-time credentials used only when the
// user table is empty. Existing installations do not need to retain them.
func BootstrapCredentials(getenv func(string) string) (string, string, error) {
	username := strings.TrimSpace(getenv("ADMIN_USER"))
	password := getenv("ADMIN_PASSWORD")

	if username == "" {
		return "", "", errors.New("ADMIN_USER is required for initial setup")
	}
	if err := ValidatePassword(password); err != nil {
		return "", "", fmt.Errorf("invalid ADMIN_PASSWORD: %w", err)
	}
	if strings.EqualFold(username, password) {
		return "", "", errors.New("ADMIN_PASSWORD must differ from ADMIN_USER")
	}

	return username, password, nil
}

// ValidatePassword applies the same minimum policy to bootstrap, creation, and
// password-change flows.
func ValidatePassword(password string) error {
	if strings.TrimSpace(password) == "" {
		return errors.New("password is required")
	}
	if utf8.RuneCountInString(password) < minAdminPassword {
		return fmt.Errorf("password must contain at least %d characters", minAdminPassword)
	}
	if _, insecure := insecureValues[strings.ToLower(password)]; insecure {
		return errors.New("password uses a known insecure value")
	}
	return nil
}

// CommaSeparated returns trimmed, non-empty configuration values.
func CommaSeparated(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

// AllowedOrigins validates optional cross-origin browser access. An empty list
// keeps the UI same-origin only.
func AllowedOrigins(getenv func(string) string) ([]string, error) {
	origins := CommaSeparated(getenv("ALLOWED_ORIGINS"))
	for _, origin := range origins {
		if origin == "*" {
			return nil, errors.New("ALLOWED_ORIGINS must not contain a wildcard")
		}
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
			return nil, fmt.Errorf("invalid allowed origin %q", origin)
		}
		if parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.Fragment != "" {
			return nil, fmt.Errorf("allowed origin %q must contain only scheme and host", origin)
		}
	}
	return origins, nil
}

// TrustedProxyCIDRs requires explicit network boundaries before forwarded
// client-address headers can influence rate limiting or audit logs.
func TrustedProxyCIDRs(getenv func(string) string, enabled bool) ([]netip.Prefix, error) {
	values := CommaSeparated(getenv("TRUSTED_PROXY_CIDRS"))
	if !enabled {
		return nil, nil
	}
	if len(values) == 0 {
		return nil, errors.New("TRUSTED_PROXY_CIDRS is required when TRUST_PROXY=true")
	}

	prefixes := make([]netip.Prefix, 0, len(values))
	for _, value := range values {
		prefix, err := netip.ParsePrefix(value)
		if err != nil {
			return nil, fmt.Errorf("invalid trusted proxy CIDR %q: %w", value, err)
		}
		prefixes = append(prefixes, prefix.Masked())
	}
	return prefixes, nil
}
