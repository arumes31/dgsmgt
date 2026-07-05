package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"net/http"
)

// ---- Profile -------------------------------------------------------------------

func (a *API) MeHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil {
		utils.Unauthorized(w, "User not found")
		return
	}
	utils.Success(w, map[string]interface{}{
		"user_id":              user.ID,
		"username":             user.Username,
		"email":                user.Email,
		"is_admin":             user.IsAdmin,
		"is_root":              user.IsRoot,
		"totp_enabled":         user.TOTPEnabled,
		"discord_id":           user.DiscordID,
		"discord_username":     user.DiscordUsername,
		"must_change_password": user.MustChangePassword,
		"timezone":             user.Timezone,
		"last_login_at":        user.LastLoginAt,
	})
}

func (a *API) UpdateProfileHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Email    string `json:"email" validate:"omitempty,email"`
		Timezone string `json:"timezone" validate:"omitempty,max=64"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	// Struct-based Updates only writes non-zero fields, so an omitted email or
	// timezone is left unchanged rather than blanked. Clobbering email would
	// silently break password-reset-by-email for that account.
	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Updates(models.User{Email: input.Email, Timezone: input.Timezone}).Error; err != nil {
		a.internalError(w, r, err, "Failed to update profile")
		return
	}
	a.audit(r, claims, "update_profile", auditOpts{Details: "Updated profile", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// ChangePasswordHandler requires the current password (session-hijack guard)
// and revokes all other sessions afterwards.
func (a *API) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)

	var input struct {
		CurrentPassword string `json:"current_password" validate:"required"`
		Password        string `json:"password" validate:"required,min=8"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil {
		utils.Unauthorized(w, "User not found")
		return
	}
	if _, err := auth.Authenticate(a.db, user.Username, input.CurrentPassword); err != nil {
		a.audit(r, claims, "change_password", auditOpts{Details: "Wrong current password", Success: false})
		utils.Forbidden(w, "Current password is incorrect")
		return
	}

	hashedPass, err := auth.HashPassword(input.Password)
	if err != nil {
		a.internalError(w, r, err, "Error hashing password")
		return
	}

	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Updates(map[string]interface{}{
			"password_hash":        hashedPass,
			"must_change_password": false,
		}).Error; err != nil {
		a.internalError(w, r, err, "Failed to update password")
		return
	}
	_ = auth.RevokeAllSessions(a.db, claims.UserID)

	a.audit(r, claims, "change_password", auditOpts{Details: "User changed their own password", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- TOTP 2FA -------------------------------------------------------------------

// TOTPSetupHandler generates a secret and provisioning URL (QR).
func (a *API) TOTPSetupHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	secret, url, err := auth.GenerateTOTPSecret(claims.Username)
	if err != nil {
		a.internalError(w, r, err, "Failed to generate TOTP secret")
		return
	}
	// Stored but not enabled until confirmed with a valid code.
	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Updates(map[string]interface{}{"totp_secret": secret, "totp_enabled": false}).Error; err != nil {
		a.internalError(w, r, err, "Failed to store TOTP secret")
		return
	}
	utils.Success(w, map[string]string{"secret": secret, "url": url})
}

func (a *API) TOTPEnableHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Code string `json:"code" validate:"required,len=6"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil || user.TOTPSecret == "" {
		utils.BadRequest(w, "Run TOTP setup first")
		return
	}
	if !auth.ValidateTOTP(input.Code, user.TOTPSecret) {
		utils.BadRequest(w, "Invalid code")
		return
	}
	a.db.Model(&user).Update("totp_enabled", true)
	a.audit(r, claims, "totp_enabled", auditOpts{Details: "Two-factor authentication enabled", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) TOTPDisableHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Password string `json:"password" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil {
		utils.Unauthorized(w, "User not found")
		return
	}
	if _, err := auth.Authenticate(a.db, user.Username, input.Password); err != nil {
		utils.Forbidden(w, "Password is incorrect")
		return
	}
	a.db.Model(&user).Updates(map[string]interface{}{"totp_enabled": false, "totp_secret": ""})
	a.audit(r, claims, "totp_disabled", auditOpts{Details: "Two-factor authentication disabled", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}
