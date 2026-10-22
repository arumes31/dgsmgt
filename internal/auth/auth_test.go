package auth

import (
	"dgsmgt/internal/models"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestHashPassword(t *testing.T) {
	password := "password123"
	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}
	if hash == "" {
		t.Fatal("Hash is empty")
	}
}

func TestGenerateToken(t *testing.T) {
	user := &models.User{
		Username: "testuser",
		IsAdmin:  true,
	}
	secret := "secret"
	token, err := GenerateToken(user, secret)
	if err != nil {
		t.Fatalf("Failed to generate token: %v", err)
	}
	if token == "" {
		t.Fatal("Token is empty")
	}

	claims, err := VerifyToken(token, secret)
	if err != nil {
		t.Fatalf("Failed to verify token: %v", err)
	}
	if claims.Username != user.Username {
		t.Errorf("Expected username %s, got %s", user.Username, claims.Username)
	}
}

func TestValidateSecret(t *testing.T) {
	for secret, ok := range map[string]bool{
		"":                                 false,
		"default_secret_change_me":         false,
		"change_me_in_production":          false,
		"short":                            false,
		"0123456789abcdef0123456789abcdef": true,
	} {
		if err := ValidateSecret(secret); (err == nil) != ok {
			t.Errorf("ValidateSecret(%q) = %v, want ok=%v", secret, err, ok)
		}
	}
}

func TestLoadOrCreateSecret(t *testing.T) {
	path := t.TempDir() + "/jwt_secret"
	s1, err := LoadOrCreateSecret(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateSecret(s1); err != nil {
		t.Fatalf("generated secret is invalid: %v", err)
	}
	s2, err := LoadOrCreateSecret(path)
	if err != nil || s1 != s2 {
		t.Fatalf("secret must be persisted: %q vs %q (%v)", s1, s2, err)
	}
}

func TestVerifyTokenRejectsOtherAlgorithms(t *testing.T) {
	claims := &Claims{
		UserID:           1,
		IsAdmin:          true,
		RegisteredClaims: jwt.RegisteredClaims{ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour))},
	}

	none, err := jwt.NewWithClaims(jwt.SigningMethodNone, claims).SignedString(jwt.UnsafeAllowNoneSignatureType)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyToken(none, "secret"); err == nil {
		t.Fatal("unsigned token must be rejected")
	}

	hs512, err := jwt.NewWithClaims(jwt.SigningMethodHS512, claims).SignedString([]byte("secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := VerifyToken(hs512, "secret"); err == nil {
		t.Fatal("tokens signed with other algorithms must be rejected")
	}
}

func TestWeakPasswords(t *testing.T) {
	for _, p := range []string{"admin", "password123", "short"} {
		if !IsWeakPassword(p) {
			t.Errorf("%q should be weak", p)
		}
	}
	hash, _ := HashPassword("admin")
	if !IsDefaultPassword(hash) {
		t.Error("admin hash should be detected as default")
	}
}
