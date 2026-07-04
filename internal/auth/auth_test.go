package auth

import (
	"dgsmgt/internal/models"
	"testing"
	"time"

	"github.com/glebarez/sqlite"
	"github.com/golang-jwt/jwt/v5"
	"gorm.io/gorm"
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
	user.ID = 1
	secret := "secret"
	token, err := GenerateToken(user, secret, time.Hour)
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

func TestVerifyTokenError(t *testing.T) {
	_, err := VerifyToken("invalid-token", "secret")
	if err == nil {
		t.Error("Expected error for invalid token")
	}

	// malformed token
	_, err = VerifyToken("a.b.c", "secret")
	if err == nil {
		t.Error("Expected error for malformed token")
	}
}

func TestVerifyTokenUnsigned(t *testing.T) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, &Claims{})
	// Sign with a different secret or method to trigger verification failure
	tokenString, _ := token.SignedString([]byte("wrong-secret"))
	_, err := VerifyToken(tokenString, "correct-secret")
	if err == nil {
		t.Error("Expected error for invalid signature")
	}
}

func TestAuthenticate(t *testing.T) {
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	_ = db.AutoMigrate(&models.User{})

	pass := "password123"
	hash, _ := HashPassword(pass)
	user := models.User{Username: "testuser", PasswordHash: hash}
	db.Create(&user)

	// Success
	res, err := Authenticate(db, "testuser", pass)
	if err != nil {
		t.Fatalf("Expected success, got error: %v", err)
	}
	if res.Username != "testuser" {
		t.Errorf("Expected testuser, got %s", res.Username)
	}

	// Wrong password
	_, err = Authenticate(db, "testuser", "wrong-password")
	if err == nil {
		t.Error("Expected error for wrong password")
	}

	// Non-existent user
	_, err = Authenticate(db, "nonexistent-user-123", pass)
	if err == nil || err.Error() != "invalid username or password" {
		t.Errorf("Expected invalid credentials error, got %v", err)
	}

	// Trigger DB error
	_ = db.Migrator().DropTable(&models.User{})
	_, err = Authenticate(db, "any", "any")
	if err == nil {
		t.Error("Expected DB error, got nil")
	}
}
