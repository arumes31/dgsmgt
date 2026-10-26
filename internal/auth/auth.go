package auth

import (
	"crypto/rand"
	"dgsmgt/internal/db"
	"dgsmgt/internal/models"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

// MinSecretLength is the minimum accepted length (in bytes) of the JWT signing secret.
const MinSecretLength = 32

// knownPlaceholderSecrets are example values that shipped with earlier releases
// and must never be used to sign tokens.
var knownPlaceholderSecrets = map[string]bool{
	"default_secret_change_me":                           true,
	"change_me_in_production":                            true,
	"replace_me_with_a_long_random_string_in_production": true,
	"your_secure_secret":                                 true,
	"secret":                                             true,
	"changeme":                                           true,
}

// knownDefaultPasswords are passwords that shipped as defaults in earlier releases.
var knownDefaultPasswords = []string{"admin", "password123"}

type Claims struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	IsAdmin      bool   `json:"is_admin"`
	TokenVersion uint   `json:"ver"`
	// MustChangePassword is loaded from the database on each request, never trusted from the token
	MustChangePassword bool `json:"must_change_password"`
	jwt.RegisteredClaims
}

// ValidateSecret rejects empty, placeholder and too short JWT secrets.
func ValidateSecret(secret string) error {
	if secret == "" {
		return errors.New("JWT secret is empty")
	}
	if knownPlaceholderSecrets[strings.ToLower(secret)] {
		return errors.New("JWT secret is a publicly known placeholder value")
	}
	if len(secret) < MinSecretLength {
		return fmt.Errorf("JWT secret must be at least %d bytes long", MinSecretLength)
	}
	return nil
}

// LoadOrCreateSecret reads the secret stored at path, or generates a new random
// secret and stores it there (mode 0600) if the file does not exist yet.
func LoadOrCreateSecret(path string) (string, error) {
	data, err := os.ReadFile(path) // #nosec G304 -- path is derived from operator configuration
	if err == nil {
		secret := strings.TrimSpace(string(data))
		if err := ValidateSecret(secret); err != nil {
			return "", fmt.Errorf("stored secret in %s is invalid: %w", path, err)
		}
		return secret, nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}

	secret, err := RandomString(48)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(secret+"\n"), 0o600); err != nil {
		return "", err
	}
	return secret, nil
}

// RandomString returns a URL-safe random string built from n random bytes.
func RandomString(n int) (string, error) {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func GenerateToken(user *models.User, secret string) (string, error) {
	expirationTime := time.Now().Add(24 * time.Hour)
	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		IsAdmin:      user.IsAdmin,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expirationTime),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func VerifyToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithExpirationRequired())

	if err != nil {
		return nil, err
	}

	if !token.Valid {
		return nil, errors.New("invalid token")
	}

	return claims, nil
}

var (
	dummyHashOnce sync.Once
	dummyHash     []byte
)

// dummyPasswordHash returns a bcrypt hash used to equalize login timing for unknown users.
func dummyPasswordHash() []byte {
	dummyHashOnce.Do(func() {
		dummyHash, _ = bcrypt.GenerateFromPassword([]byte("dgsmgt-timing-dummy"), bcrypt.DefaultCost)
	})
	return dummyHash
}

func Authenticate(username, password string) (*models.User, error) {
	var user models.User
	if err := db.DB.Where("username = ?", username).First(&user).Error; err != nil {
		// Run a comparison anyway so unknown users cannot be detected by response time
		_ = bcrypt.CompareHashAndPassword(dummyPasswordHash(), []byte(password))
		return nil, errors.New("invalid username or password")
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, errors.New("invalid username or password")
	}

	return &user, nil
}

func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// IsDefaultPassword reports whether hash matches one of the passwords that
// shipped as defaults in earlier releases.
func IsDefaultPassword(hash string) bool {
	for _, p := range knownDefaultPasswords {
		if bcrypt.CompareHashAndPassword([]byte(hash), []byte(p)) == nil {
			return true
		}
	}
	return false
}

// IsWeakPassword reports whether a password is too short or a known default.
func IsWeakPassword(password string) bool {
	if len(password) < 8 {
		return true
	}
	for _, p := range knownDefaultPasswords {
		if password == p {
			return true
		}
	}
	return false
}
