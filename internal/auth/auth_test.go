package auth

import (
	"dgsmgt/internal/models"
	"testing"
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
