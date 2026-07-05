package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"dgsmgt/internal/notify"
	"dgsmgt/internal/utils"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

const maxLoginAttempts = 5
const lockoutWindow = 15 * time.Minute

func (a *API) attemptKey(r *http.Request, username string) string {
	return middleware.ClientIP(r) + ":" + username
}

func (a *API) isLockedOut(key string) bool {
	if val, ok := a.loginAttempts.Load(key); ok {
		count, last := val.(*loginAttempt).snapshot()
		return count >= maxLoginAttempts && time.Since(last) < lockoutWindow
	}
	return false
}

func (a *API) recordFailure(key string) {
	actual, _ := a.loginAttempts.LoadOrStore(key, &loginAttempt{})
	attempt := actual.(*loginAttempt)
	attempt.mu.Lock()
	attempt.Count++
	attempt.LastError = time.Now()
	attempt.mu.Unlock()
}

// issueTokens creates the access + refresh token pair for a user.
func (a *API) issueTokens(r *http.Request, user *models.User) (map[string]interface{}, error) {
	access, err := auth.GenerateToken(user, a.jwtSecret, a.cfg.AccessTokenTTL)
	if err != nil {
		return nil, err
	}
	refresh, err := auth.CreateSession(a.db, user, r.UserAgent(), middleware.ClientIP(r), a.cfg.RefreshTokenTTL)
	if err != nil {
		return nil, err
	}
	now := time.Now()
	a.db.Model(&models.User{}).Where("id = ?", user.ID).Update("last_login_at", &now)
	return map[string]interface{}{
		"token":                access,
		"refresh_token":        refresh,
		"expires_in":           int(a.cfg.AccessTokenTTL.Seconds()),
		"must_change_password": user.MustChangePassword,
	}, nil
}

func (a *API) LoginHandler(w http.ResponseWriter, r *http.Request) {
	var creds struct {
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &creds) {
		return
	}

	key := a.attemptKey(r, creds.Username)
	if a.isLockedOut(key) {
		a.logger.Warn("Brute force protection triggered", zap.String("key", key))
		a.audit(r, &auth.Claims{Username: creds.Username}, "login_blocked",
			auditOpts{Details: "Login blocked by brute force protection", Success: false})
		utils.Forbidden(w, "Too many login attempts. Try again in 15 minutes.")
		return
	}

	user, err := auth.Authenticate(a.db, creds.Username, creds.Password)
	if err != nil {
		if errors.Is(err, auth.ErrInvalidCredentials) {
			a.recordFailure(key)
			a.audit(r, &auth.Claims{Username: creds.Username}, "login_failed",
				auditOpts{Details: "Invalid credentials", Success: false})
			utils.Unauthorized(w, "Invalid credentials")
			return
		}
		if errors.Is(err, auth.ErrAccountDisabled) {
			a.audit(r, &auth.Claims{Username: creds.Username}, "login_failed",
				auditOpts{Details: "Account disabled", Success: false})
			utils.Forbidden(w, "Account is disabled")
			return
		}
		a.internalError(w, r, err, "Login failed")
		return
	}

	// Two-factor: return a pending token, client must call /login/totp.
	// Do NOT clear the brute-force counter here — LoginTOTPHandler shares this
	// key, so resetting it on password success would let a password-knowing
	// attacker loop /login -> /login/totp and brute-force the 6-digit code
	// without ever tripping the lockout.
	if user.TOTPEnabled {
		pending, err := auth.GeneratePendingToken(user, a.jwtSecret)
		if err != nil {
			a.internalError(w, r, err, "Failed to generate token")
			return
		}
		utils.Success(w, map[string]interface{}{"totp_required": true, "pending_token": pending})
		return
	}

	a.loginAttempts.Delete(key)

	tokens, err := a.issueTokens(r, user)
	if err != nil {
		a.internalError(w, r, err, "Failed to generate token")
		return
	}
	claims := &auth.Claims{UserID: user.ID, Username: user.Username}
	a.audit(r, claims, "login", auditOpts{Details: "User logged in", Success: true})
	a.notifier.Dispatch(notify.Event{
		Type: notify.EventLogin, Title: fmt.Sprintf("Login: %s", user.Username),
		Body: fmt.Sprintf("%s logged in from %s", user.Username, middleware.ClientIP(r)), Level: "info",
	})
	utils.Success(w, tokens)
}

// LoginTOTPHandler completes a 2FA login using the pending token + code.
func (a *API) LoginTOTPHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		PendingToken string `json:"pending_token" validate:"required"`
		Code         string `json:"code" validate:"required,len=6"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	claims, err := auth.VerifyToken(input.PendingToken, a.jwtSecret)
	if err != nil || !claims.Pending2FA {
		utils.Unauthorized(w, "Invalid or expired pending token")
		return
	}

	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil {
		utils.Unauthorized(w, "User not found")
		return
	}

	key := a.attemptKey(r, user.Username)
	if a.isLockedOut(key) {
		utils.Forbidden(w, "Too many attempts. Try again later.")
		return
	}
	if !auth.ValidateTOTP(input.Code, user.TOTPSecret) {
		a.recordFailure(key)
		a.audit(r, claims, "login_failed", auditOpts{Details: "Invalid TOTP code", Success: false})
		utils.Unauthorized(w, "Invalid code")
		return
	}
	a.loginAttempts.Delete(key)

	tokens, err := a.issueTokens(r, &user)
	if err != nil {
		a.internalError(w, r, err, "Failed to generate token")
		return
	}
	a.audit(r, claims, "login", auditOpts{Details: "User logged in (2FA)", Success: true})
	utils.Success(w, tokens)
}

// RefreshHandler rotates a refresh token and issues a new access token.
func (a *API) RefreshHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	user, newRefresh, err := auth.RotateSession(a.db, input.RefreshToken)
	if err != nil {
		utils.Unauthorized(w, "Invalid session")
		return
	}
	access, err := auth.GenerateToken(user, a.jwtSecret, a.cfg.AccessTokenTTL)
	if err != nil {
		a.internalError(w, r, err, "Failed to generate token")
		return
	}
	utils.Success(w, map[string]interface{}{
		"token":         access,
		"refresh_token": newRefresh,
		"expires_in":    int(a.cfg.AccessTokenTTL.Seconds()),
	})
}

// LogoutHandler revokes the presented refresh token.
func (a *API) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		RefreshToken string `json:"refresh_token"`
	}
	_ = a.decodeAndValidate(w, r, &input)
	if input.RefreshToken != "" {
		_ = auth.RevokeSession(a.db, input.RefreshToken)
	}
	if claims := claimsFrom(r); claims != nil {
		a.audit(r, claims, "logout", auditOpts{Details: "User logged out", Success: true})
	}
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- Sessions (device list) ---------------------------------------------------

func (a *API) ListSessionsHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var sessions []models.Session
	if err := a.db.Where("user_id = ? AND revoked = ? AND expires_at > ?",
		claims.UserID, false, time.Now()).Order("last_used_at desc").Find(&sessions).Error; err != nil {
		a.internalError(w, r, err, "Failed to list sessions")
		return
	}
	utils.Success(w, sessions)
}

func (a *API) RevokeSessionHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	id := mux.Vars(r)["id"]
	res := a.db.Model(&models.Session{}).
		Where("id = ? AND user_id = ?", id, claims.UserID).Update("revoked", true)
	if res.Error != nil {
		a.internalError(w, r, res.Error, "Failed to revoke session")
		return
	}
	a.audit(r, claims, "revoke_session", auditOpts{Details: "Revoked session " + id, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) RevokeAllSessionsHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if err := auth.RevokeAllSessions(a.db, claims.UserID); err != nil {
		a.internalError(w, r, err, "Failed to revoke sessions")
		return
	}
	a.audit(r, claims, "revoke_all_sessions", auditOpts{Details: "Revoked all sessions", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- Lockout admin view -------------------------------------------------------------

func (a *API) ListLockoutsHandler(w http.ResponseWriter, r *http.Request) {
	type entry struct {
		Key       string    `json:"key"`
		Count     int       `json:"count"`
		LastError time.Time `json:"last_error"`
		Locked    bool      `json:"locked"`
	}
	out := []entry{}
	a.loginAttempts.Range(func(k, v interface{}) bool {
		count, last := v.(*loginAttempt).snapshot()
		out = append(out, entry{
			Key: k.(string), Count: count, LastError: last,
			Locked: count >= maxLoginAttempts && time.Since(last) < lockoutWindow,
		})
		return true
	})
	utils.Success(w, out)
}

func (a *API) UnlockHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Key string `json:"key" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	a.loginAttempts.Delete(input.Key)
	a.audit(r, claimsFrom(r), "unlock_account", auditOpts{Details: "Unlocked " + input.Key, Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}
