package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"dgsmgt/internal/models"
	"encoding/hex"
	"errors"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
	"gorm.io/gorm"
)

var ErrInvalidCredentials = errors.New("invalid username or password")
var ErrAccountDisabled = errors.New("account is disabled")

type Claims struct {
	UserID       uint   `json:"user_id"`
	Username     string `json:"username"`
	IsAdmin      bool   `json:"is_admin"`
	IsRoot       bool   `json:"is_root"`
	TokenVersion int    `json:"tv"`
	Pending2FA   bool   `json:"pending_2fa,omitempty"` // password ok, awaiting TOTP
	jwt.RegisteredClaims
}

// GenerateToken issues a short-lived access token.
var GenerateToken = func(user *models.User, secret string, ttl time.Duration) (string, error) {
	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		IsAdmin:      user.IsAdmin,
		IsRoot:       user.IsRoot,
		TokenVersion: user.TokenVersion,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(ttl)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

// GeneratePendingToken issues a 5-minute token that is only valid to complete
// the TOTP step of login.
func GeneratePendingToken(user *models.User, secret string) (string, error) {
	claims := &Claims{
		UserID:       user.ID,
		Username:     user.Username,
		TokenVersion: user.TokenVersion,
		Pending2FA:   true,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(5 * time.Minute)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(secret))
}

func VerifyToken(tokenString, secret string) (*Claims, error) {
	claims := &Claims{}
	_, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
		return []byte(secret), nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

func Authenticate(database *gorm.DB, username, password string) (*models.User, error) {
	var user models.User
	if err := database.Where("username = ?", username).First(&user).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidCredentials
		}
		return nil, err
	}

	if err := bcrypt.CompareHashAndPassword([]byte(user.PasswordHash), []byte(password)); err != nil {
		return nil, ErrInvalidCredentials
	}

	if user.Disabled {
		return nil, ErrAccountDisabled
	}

	return &user, nil
}

var HashPassword = func(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	return string(bytes), err
}

// RandomToken returns a 32-byte hex random token.
func RandomToken() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// HashToken hashes an opaque token (refresh token, reset token) for storage.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// CreateSession creates a refresh session and returns the opaque refresh token.
func CreateSession(database *gorm.DB, user *models.User, ua, ip string, ttl time.Duration) (string, error) {
	token, err := RandomToken()
	if err != nil {
		return "", err
	}
	sess := models.Session{
		UserID:           user.ID,
		RefreshTokenHash: HashToken(token),
		UserAgent:        ua,
		IP:               ip,
		ExpiresAt:        time.Now().Add(ttl),
		LastUsedAt:       time.Now(),
	}
	if err := database.Create(&sess).Error; err != nil {
		return "", err
	}
	return token, nil
}

// RotateSession validates a refresh token, rotates it and returns user + new token.
func RotateSession(database *gorm.DB, refreshToken string) (*models.User, string, error) {
	var sess models.Session
	if err := database.Where("refresh_token_hash = ? AND revoked = ?", HashToken(refreshToken), false).
		First(&sess).Error; err != nil {
		return nil, "", errors.New("invalid session")
	}
	if time.Now().After(sess.ExpiresAt) {
		return nil, "", errors.New("session expired")
	}

	var user models.User
	if err := database.First(&user, sess.UserID).Error; err != nil {
		return nil, "", errors.New("user not found")
	}
	if user.Disabled {
		return nil, "", ErrAccountDisabled
	}

	newToken, err := RandomToken()
	if err != nil {
		return nil, "", err
	}
	sess.RefreshTokenHash = HashToken(newToken)
	sess.LastUsedAt = time.Now()
	if err := database.Save(&sess).Error; err != nil {
		return nil, "", err
	}
	return &user, newToken, nil
}

// RevokeSession revokes the session matching a refresh token.
func RevokeSession(database *gorm.DB, refreshToken string) error {
	return database.Model(&models.Session{}).
		Where("refresh_token_hash = ?", HashToken(refreshToken)).
		Update("revoked", true).Error
}

// RevokeAllSessions revokes every session of a user and bumps their token
// version so outstanding access tokens die too.
func RevokeAllSessions(database *gorm.DB, userID uint) error {
	if err := database.Model(&models.Session{}).Where("user_id = ?", userID).
		Update("revoked", true).Error; err != nil {
		return err
	}
	err := database.Model(&models.User{}).Where("id = ?", userID).
		UpdateColumn("token_version", gorm.Expr("token_version + 1")).Error
	if err == nil {
		invalidateTokenVersion(userID)
	}
	return err
}

// tokenVersionTTL bounds staleness for out-of-band token_version changes;
// in-process revocations invalidate the cache immediately.
const tokenVersionTTL = 30 * time.Second

type tokenVersionEntry struct {
	version int
	at      time.Time
}

var tokenVersions = struct {
	mu sync.Mutex
	m  map[uint]tokenVersionEntry
}{m: map[uint]tokenVersionEntry{}}

// ValidateTokenVersion reports whether the claims carry the user's current
// token version. Versions are cached briefly so this per-request check does
// not add a DB round-trip to every authenticated call. A missing user is
// invalid; a DB failure is returned so the caller can fail open instead of
// mass-401ing during a transient outage.
func ValidateTokenVersion(database *gorm.DB, c *Claims) (bool, error) {
	now := time.Now()
	tokenVersions.mu.Lock()
	e, ok := tokenVersions.m[c.UserID]
	tokenVersions.mu.Unlock()
	if !ok || now.Sub(e.at) > tokenVersionTTL {
		var u models.User
		if err := database.Select("token_version").Where("id = ?", c.UserID).
			Take(&u).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		e = tokenVersionEntry{version: u.TokenVersion, at: now}
		tokenVersions.mu.Lock()
		tokenVersions.m[c.UserID] = e
		tokenVersions.mu.Unlock()
	}
	return e.version == c.TokenVersion, nil
}

func invalidateTokenVersion(userID uint) {
	tokenVersions.mu.Lock()
	delete(tokenVersions.m, userID)
	tokenVersions.mu.Unlock()
}
