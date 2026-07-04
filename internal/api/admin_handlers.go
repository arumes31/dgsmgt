package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/csv"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
	"gorm.io/gorm"
)

// ---- Users -------------------------------------------------------------------

func (a *API) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	if err := a.db.Find(&users).Error; err != nil {
		a.internalError(w, r, err, "Failed to list users")
		return
	}
	utils.Success(w, users)
}

// UserDetailHandler returns a user with their assignments, groups and recent activity.
func (a *API) UserDetailHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var user models.User
	if err := a.db.First(&user, id).Error; err != nil {
		utils.NotFound(w, "User not found")
		return
	}
	var assignments []models.UserServer
	a.db.Where("user_id = ?", user.ID).Find(&assignments)
	var groups []models.Group
	a.db.Joins("JOIN user_groups ON user_groups.group_id = groups.id").
		Where("user_groups.user_id = ?", user.ID).Find(&groups)
	var recent []models.AuditLog
	a.db.Where("user_id = ?", user.ID).Order("created_at desc").Limit(20).Find(&recent)
	var sessions []models.Session
	a.db.Where("user_id = ? AND revoked = ? AND expires_at > ?", user.ID, false, time.Now()).Find(&sessions)

	utils.Success(w, map[string]interface{}{
		"user": user, "assignments": assignments, "groups": groups,
		"recent_activity": recent, "active_sessions": len(sessions),
	})
}

func (a *API) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username           string `json:"username" validate:"required,min=3,max=32"`
		Password           string `json:"password" validate:"required,min=8"`
		Email              string `json:"email" validate:"omitempty,email"`
		IsAdmin            bool   `json:"is_admin"`
		MustChangePassword bool   `json:"must_change_password"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	hashedPass, err := auth.HashPassword(input.Password)
	if err != nil {
		a.internalError(w, r, err, "Error hashing password")
		return
	}

	user := models.User{
		Username:           input.Username,
		Email:              input.Email,
		PasswordHash:       hashedPass,
		IsAdmin:            input.IsAdmin,
		MustChangePassword: input.MustChangePassword,
	}

	if err := a.db.Create(&user).Error; err != nil {
		utils.BadRequest(w, "Username already exists")
		return
	}

	claims := claimsFrom(r)
	a.audit(r, claims, "create_user", auditOpts{Details: fmt.Sprintf("Created user: %s", user.Username), Success: true})
	utils.Created(w, user)
}

func (a *API) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var user models.User
	if err := a.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(w, "User not found")
		} else {
			a.internalError(w, r, err, "Failed to load user")
		}
		return
	}

	var input struct {
		Username           string `json:"username" validate:"required,min=3,max=32"`
		Password           string `json:"password" validate:"omitempty,min=8"`
		Email              string `json:"email" validate:"omitempty,email"`
		IsAdmin            bool   `json:"is_admin"`
		Disabled           bool   `json:"disabled"`
		MustChangePassword bool   `json:"must_change_password"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	// Last-admin protection: cannot demote or disable the final admin.
	if user.IsAdmin && (!input.IsAdmin || input.Disabled) {
		var adminCount int64
		a.db.Model(&models.User{}).Where("is_admin = ? AND disabled = ? AND id != ?", true, false, user.ID).Count(&adminCount)
		if adminCount == 0 {
			utils.BadRequest(w, "Cannot demote or disable the last admin")
			return
		}
	}
	// Root user can only be edited by root.
	if user.IsRoot && !claims.IsRoot {
		utils.Forbidden(w, "Only the superadmin can edit this user")
		return
	}

	before := user
	user.Username = input.Username
	user.Email = input.Email
	user.IsAdmin = input.IsAdmin
	user.Disabled = input.Disabled
	user.MustChangePassword = input.MustChangePassword
	if input.Password != "" {
		hashedPass, err := auth.HashPassword(input.Password)
		if err != nil {
			a.internalError(w, r, err, "Error hashing password")
			return
		}
		user.PasswordHash = hashedPass
	}

	if err := a.db.Save(&user).Error; err != nil {
		a.internalError(w, r, err, "Failed to update user")
		return
	}
	if input.Password != "" || input.Disabled {
		_ = auth.RevokeAllSessions(a.db, user.ID)
	}

	a.audit(r, claims, "update_user", auditOpts{
		Details: fmt.Sprintf("Updated user: %s", user.Username), Success: true,
		Diff: map[string]interface{}{
			"before": map[string]interface{}{"username": before.Username, "is_admin": before.IsAdmin, "disabled": before.Disabled},
			"after":  map[string]interface{}{"username": user.Username, "is_admin": user.IsAdmin, "disabled": user.Disabled},
		},
	})
	utils.Success(w, user)
}

func (a *API) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var user models.User
	if err := a.db.First(&user, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(w, "User not found")
		} else {
			a.internalError(w, r, err, "Failed to load user")
		}
		return
	}

	if user.ID == claims.UserID {
		utils.BadRequest(w, "You cannot delete your own account")
		return
	}
	if user.IsRoot {
		utils.Forbidden(w, "The superadmin account cannot be deleted")
		return
	}
	if user.IsAdmin {
		var adminCount int64
		a.db.Model(&models.User{}).Where("is_admin = ? AND disabled = ? AND id != ?", true, false, user.ID).Count(&adminCount)
		if adminCount == 0 {
			utils.BadRequest(w, "Cannot delete the last admin")
			return
		}
	}

	// Cascade: assignments, group memberships, sessions, prefs, subscriptions.
	a.db.Where("user_id = ?", user.ID).Delete(&models.UserServer{})
	a.db.Where("user_id = ?", user.ID).Delete(&models.UserGroup{})
	a.db.Unscoped().Where("user_id = ?", user.ID).Delete(&models.Session{})
	a.db.Where("user_id = ?", user.ID).Delete(&models.NotificationPref{})
	a.db.Unscoped().Where("user_id = ?", user.ID).Delete(&models.PushSubscription{})
	a.db.Unscoped().Where("user_id = ?", user.ID).Delete(&models.Notification{})

	if err := a.db.Delete(&user).Error; err != nil {
		a.internalError(w, r, err, "Failed to delete user")
		return
	}

	a.audit(r, claims, "delete_user", auditOpts{Details: fmt.Sprintf("Deleted user: %s", user.Username), Success: true})
	w.WriteHeader(http.StatusNoContent)
}

// ImportUsersHandler bulk-creates users from CSV: username,password,email,is_admin
func (a *API) ImportUsersHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)
	if err := r.ParseMultipartForm(8 << 20); err != nil {
		utils.BadRequest(w, "Invalid upload")
		return
	}
	file, _, err := r.FormFile("csv")
	if err != nil {
		utils.BadRequest(w, "csv field required")
		return
	}
	defer func() { _ = file.Close() }()

	reader := csv.NewReader(file)
	created, failed := []string{}, []string{}
	for {
		rec, err := reader.Read()
		if err != nil {
			break
		}
		if len(rec) < 2 || strings.EqualFold(rec[0], "username") {
			continue
		}
		username, password := strings.TrimSpace(rec[0]), strings.TrimSpace(rec[1])
		email := ""
		isAdmin := false
		if len(rec) > 2 {
			email = strings.TrimSpace(rec[2])
		}
		if len(rec) > 3 {
			isAdmin = strings.EqualFold(strings.TrimSpace(rec[3]), "true")
		}
		if len(username) < 3 || len(password) < 8 {
			failed = append(failed, username)
			continue
		}
		hash, err := auth.HashPassword(password)
		if err != nil {
			failed = append(failed, username)
			continue
		}
		u := models.User{Username: username, Email: email, PasswordHash: hash,
			IsAdmin: isAdmin, MustChangePassword: true}
		if err := a.db.Create(&u).Error; err != nil {
			failed = append(failed, username)
		} else {
			created = append(created, username)
		}
	}
	a.audit(r, claims, "import_users", auditOpts{
		Details: fmt.Sprintf("Imported %d users (%d failed)", len(created), len(failed)), Success: true})
	utils.Success(w, map[string]interface{}{"created": created, "failed": failed})
}
