package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"errors"
	"fmt"
	"net/http"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// ---- Discord OAuth ---------------------------------------------------------------

func (a *API) discordConfigured() bool {
	return a.cfg.DiscordClientID != "" && a.cfg.DiscordClientSecret != "" && a.cfg.DiscordRedirectURL != ""
}

// DiscordLoginHandler redirects to Discord's consent screen.
func (a *API) DiscordLoginHandler(w http.ResponseWriter, r *http.Request) {
	if !a.discordConfigured() {
		utils.BadRequest(w, "Discord login is not configured")
		return
	}
	state, err := auth.RandomToken()
	if err != nil {
		a.internalError(w, r, err, "Failed to create state")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: "oauth_state", Value: state, Path: "/", HttpOnly: true,
		MaxAge: 600, SameSite: http.SameSiteLaxMode,
	})
	http.Redirect(w, r, auth.DiscordAuthURL(a.cfg.DiscordClientID, a.cfg.DiscordRedirectURL, state), http.StatusFound)
}

// DiscordCallbackHandler completes OAuth: logs in a linked user, or
// provisions one when DISCORD_AUTO_CREATE=true.
func (a *API) DiscordCallbackHandler(w http.ResponseWriter, r *http.Request) {
	if !a.discordConfigured() {
		utils.BadRequest(w, "Discord login is not configured")
		return
	}
	stateCookie, err := r.Cookie("oauth_state")
	if err != nil || stateCookie.Value == "" || stateCookie.Value != r.URL.Query().Get("state") {
		utils.Forbidden(w, "Invalid OAuth state")
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		utils.BadRequest(w, "Missing code")
		return
	}

	du, err := auth.DiscordExchangeCode(a.cfg.DiscordClientID, a.cfg.DiscordClientSecret, a.cfg.DiscordRedirectURL, code)
	if err != nil {
		a.internalError(w, r, err, "Discord authentication failed")
		return
	}

	var user models.User
	err = a.db.Where("discord_id = ?", du.ID).First(&user).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !a.cfg.DiscordAutoCreate {
			http.Redirect(w, r, "/login.html?error=discord_not_linked", http.StatusFound)
			return
		}
		randomPass, _ := auth.RandomToken()
		hash, _ := auth.HashPassword(randomPass)
		user = models.User{
			Username:        "discord_" + du.ID,
			Email:           du.Email,
			PasswordHash:    hash,
			DiscordID:       du.ID,
			DiscordUsername: du.Username,
		}
		if err := a.db.Create(&user).Error; err != nil {
			a.internalError(w, r, err, "Failed to create user")
			return
		}
	} else if err != nil {
		a.internalError(w, r, err, "Login failed")
		return
	}

	if user.Disabled {
		utils.Forbidden(w, "Account is disabled")
		return
	}

	tokens, err := a.issueTokens(r, &user)
	if err != nil {
		a.internalError(w, r, err, "Failed to generate token")
		return
	}
	a.audit(r, &auth.Claims{UserID: user.ID, Username: user.Username}, "login",
		auditOpts{Details: "Logged in via Discord", Success: true})

	// Hand tokens to the SPA via redirect fragment (not query, so they stay
	// out of server logs).
	http.Redirect(w, r, fmt.Sprintf("/login.html#token=%s&refresh=%s",
		tokens["token"], tokens["refresh_token"]), http.StatusFound)
}

// DiscordLinkHandler links the logged-in user's Discord account (returns URL).
func (a *API) DiscordLinkHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		Code string `json:"code" validate:"required"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	du, err := auth.DiscordExchangeCode(a.cfg.DiscordClientID, a.cfg.DiscordClientSecret, a.cfg.DiscordRedirectURL, input.Code)
	if err != nil {
		a.internalError(w, r, err, "Discord link failed")
		return
	}
	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Updates(map[string]interface{}{"discord_id": du.ID, "discord_username": du.Username}).Error; err != nil {
		a.internalError(w, r, err, "Failed to link Discord")
		return
	}
	a.audit(r, claims, "discord_linked", auditOpts{Details: "Linked Discord account " + du.Username, Success: true})
	utils.Success(w, map[string]string{"status": "ok", "discord_username": du.Username})
}

func (a *API) DiscordUnlinkHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Updates(map[string]interface{}{"discord_id": "", "discord_username": ""}).Error; err != nil {
		a.internalError(w, r, err, "Failed to unlink Discord")
		return
	}
	utils.Success(w, map[string]string{"status": "ok"})
}

// ---- Invitations ------------------------------------------------------------------

func (a *API) CreateInvitationHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	var input struct {
		IsAdmin   bool `json:"is_admin"`
		ExpiresIn int  `json:"expires_in_hours"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	if input.ExpiresIn <= 0 {
		input.ExpiresIn = 72
	}
	token, err := auth.RandomToken()
	if err != nil {
		a.internalError(w, r, err, "Failed to create invitation")
		return
	}
	inv := models.Invitation{
		Token:     token,
		IsAdmin:   input.IsAdmin,
		ExpiresAt: time.Now().Add(time.Duration(input.ExpiresIn) * time.Hour),
		CreatedBy: claims.Username,
	}
	if err := a.db.Create(&inv).Error; err != nil {
		a.internalError(w, r, err, "Failed to create invitation")
		return
	}
	a.audit(r, claims, "create_invitation", auditOpts{Details: "Created invitation link", Success: true})
	utils.Created(w, map[string]string{
		"token": token,
		"url":   a.cfg.BaseURL + "/login.html#invite=" + token,
	})
}

func (a *API) ListInvitationsHandler(w http.ResponseWriter, r *http.Request) {
	var invs []models.Invitation
	if err := a.db.Order("created_at desc").Limit(100).Find(&invs).Error; err != nil {
		a.internalError(w, r, err, "Failed to list invitations")
		return
	}
	utils.Success(w, invs)
}

// AcceptInvitationHandler is unauthenticated: creates the account.
func (a *API) AcceptInvitationHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token" validate:"required"`
		Username string `json:"username" validate:"required,min=3,max=32"`
		Password string `json:"password" validate:"required,min=8"`
		Email    string `json:"email" validate:"omitempty,email"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		a.internalError(w, r, err, "Error hashing password")
		return
	}

	// Claim + user creation run in one transaction with a row lock so an
	// invitation can never be redeemed twice concurrently.
	var user models.User
	errInvalid := errors.New("invalid")
	errExpired := errors.New("expired")
	errTaken := errors.New("taken")
	txErr := a.db.Transaction(func(tx *gorm.DB) error {
		var inv models.Invitation
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("token = ? AND used_by_id = 0", input.Token).First(&inv).Error; err != nil {
			return errInvalid
		}
		if time.Now().After(inv.ExpiresAt) {
			return errExpired
		}
		user = models.User{Username: input.Username, Email: input.Email, PasswordHash: hash, IsAdmin: inv.IsAdmin}
		if err := tx.Create(&user).Error; err != nil {
			return errTaken
		}
		now := time.Now()
		return tx.Model(&inv).Updates(map[string]interface{}{"used_by_id": user.ID, "used_at": &now}).Error
	})
	switch {
	case errors.Is(txErr, errInvalid):
		utils.Forbidden(w, "Invalid or used invitation")
		return
	case errors.Is(txErr, errExpired):
		utils.Forbidden(w, "Invitation expired")
		return
	case errors.Is(txErr, errTaken):
		utils.BadRequest(w, "Username already taken")
		return
	case txErr != nil:
		a.internalError(w, r, txErr, "Failed to accept invitation")
		return
	}
	a.audit(r, &auth.Claims{UserID: user.ID, Username: user.Username}, "accept_invitation",
		auditOpts{Details: "Account created via invitation", Success: true})
	utils.Created(w, map[string]string{"status": "ok"})
}

// ---- Password reset (email) --------------------------------------------------------

func (a *API) RequestPasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Email string `json:"email" validate:"required,email"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	// Always respond OK to avoid account enumeration.
	defer utils.Success(w, map[string]string{"status": "ok"})

	var user models.User
	if err := a.db.Where("email = ? AND disabled = ?", input.Email, false).First(&user).Error; err != nil {
		return
	}
	token, err := auth.RandomToken()
	if err != nil {
		return
	}
	a.db.Create(&models.PasswordReset{
		UserID:    user.ID,
		TokenHash: auth.HashToken(token),
		ExpiresAt: time.Now().Add(time.Hour),
	})
	link := a.cfg.BaseURL + "/login.html#reset=" + token
	go a.notifier.SendEmail(user.Email, "[DGSMgt] Password reset",
		"A password reset was requested for your account.\n\nReset link (valid 1 hour):\n"+link+
			"\n\nIf you didn't request this, ignore this email.")
}

func (a *API) ConfirmPasswordResetHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Token    string `json:"token" validate:"required"`
		Password string `json:"password" validate:"required,min=8"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	var pr models.PasswordReset
	if err := a.db.Where("token_hash = ? AND used = ?", auth.HashToken(input.Token), false).
		First(&pr).Error; err != nil || time.Now().After(pr.ExpiresAt) {
		utils.Forbidden(w, "Invalid or expired reset token")
		return
	}
	hash, err := auth.HashPassword(input.Password)
	if err != nil {
		a.internalError(w, r, err, "Error hashing password")
		return
	}
	a.db.Model(&models.User{}).Where("id = ?", pr.UserID).Update("password_hash", hash)
	a.db.Model(&pr).Update("used", true)
	_ = auth.RevokeAllSessions(a.db, pr.UserID)
	a.audit(r, &auth.Claims{UserID: pr.UserID}, "password_reset", auditOpts{Details: "Password reset via email", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}
