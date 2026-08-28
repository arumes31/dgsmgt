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

func TestVerifyTokenRejectsUnexpectedSigningMethod(t *testing.T) {
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    tokenIssuer,
			Audience:  jwt.ClaimStrings{tokenAudience},
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS384, claims)
	signed, err := token.SignedString([]byte("a-secret-that-is-long-enough-for-the-test"))
	if err != nil {
		t.Fatalf("signing token: %v", err)
	}

	if _, err := VerifyToken(signed, "a-secret-that-is-long-enough-for-the-test"); err == nil {
		t.Fatal("VerifyToken() accepted HS384 token")
	}
}

func TestVerifyTokenRejectsWrongIssuerOrAudience(t *testing.T) {
	tests := []struct {
		name     string
		issuer   string
		audience string
	}{
		{name: "wrong issuer", issuer: "attacker", audience: tokenAudience},
		{name: "wrong audience", issuer: tokenIssuer, audience: "other-service"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims := &Claims{
				RegisteredClaims: jwt.RegisteredClaims{
					Issuer:    tt.issuer,
					Audience:  jwt.ClaimStrings{tt.audience},
					ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
				},
			}
			token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
			signed, err := token.SignedString([]byte("a-secret-that-is-long-enough-for-the-test"))
			if err != nil {
				t.Fatalf("signing token: %v", err)
			}

			if _, err := VerifyToken(signed, "a-secret-that-is-long-enough-for-the-test"); err == nil {
				t.Fatal("VerifyToken() accepted invalid registered claims")
			}
		})
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
