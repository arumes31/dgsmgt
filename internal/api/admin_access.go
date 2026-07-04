package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"fmt"
	"net/http"
	"time"

	"github.com/gorilla/mux"
)

// ---- Assignments -----------------------------------------------------------------

// presetPerms maps permission preset names to permission bundles.
func presetPerms(preset string) models.Perms {
	switch preset {
	case "viewer":
		return models.Perms{CanViewLogs: true}
	case "operator":
		return models.Perms{CanStart: true, CanStop: true, CanRestart: true,
			CanViewLogs: true, CanSendCommands: true}
	case "owner":
		return models.AllPerms()
	}
	return models.Perms{}
}

func (a *API) AssignServerHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID    uint       `json:"user_id" validate:"required"`
		ServerID  uint       `json:"server_id" validate:"required"`
		Preset    string     `json:"preset" validate:"omitempty,oneof=viewer operator owner"`
		ExpiresAt *time.Time `json:"expires_at"`
		models.Perms
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	// Validate both sides exist (no dangling rows).
	var count int64
	a.db.Model(&models.User{}).Where("id = ?", input.UserID).Count(&count)
	if count == 0 {
		utils.BadRequest(w, "User does not exist")
		return
	}
	a.db.Model(&models.Server{}).Where("id = ?", input.ServerID).Count(&count)
	if count == 0 {
		utils.BadRequest(w, "Server does not exist")
		return
	}

	perms := input.Perms
	if input.Preset != "" {
		perms = presetPerms(input.Preset)
	}

	userServer := models.UserServer{
		UserID: input.UserID, ServerID: input.ServerID,
		CanStart: perms.CanStart, CanStop: perms.CanStop, CanRestart: perms.CanRestart,
		CanViewLogs: perms.CanViewLogs, CanSendCommands: perms.CanSendCommands,
		CanEditConfig: perms.CanEditConfig, CanAccessFiles: perms.CanAccessFiles,
		CanManageBackups: perms.CanManageBackups,
		ExpiresAt:        input.ExpiresAt,
	}

	if err := a.db.Save(&userServer).Error; err != nil {
		a.internalError(w, r, err, "Failed to save assignment")
		return
	}

	claims := claimsFrom(r)
	a.audit(r, claims, "assign_server",
		auditOpts{Details: fmt.Sprintf("Assigned server %d to user %d", input.ServerID, input.UserID), Success: true})
	utils.Success(w, userServer)
}

// BulkAssignHandler assigns many users to many servers with one permission set.
func (a *API) BulkAssignHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserIDs   []uint `json:"user_ids" validate:"required,min=1"`
		ServerIDs []uint `json:"server_ids" validate:"required,min=1"`
		Preset    string `json:"preset" validate:"omitempty,oneof=viewer operator owner"`
		models.Perms
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	perms := input.Perms
	if input.Preset != "" {
		perms = presetPerms(input.Preset)
	}
	count := 0
	for _, uid := range input.UserIDs {
		for _, sid := range input.ServerIDs {
			us := models.UserServer{
				UserID: uid, ServerID: sid,
				CanStart: perms.CanStart, CanStop: perms.CanStop, CanRestart: perms.CanRestart,
				CanViewLogs: perms.CanViewLogs, CanSendCommands: perms.CanSendCommands,
				CanEditConfig: perms.CanEditConfig, CanAccessFiles: perms.CanAccessFiles,
				CanManageBackups: perms.CanManageBackups,
			}
			if a.db.Save(&us).Error == nil {
				count++
			}
		}
	}
	a.audit(r, claimsFrom(r), "bulk_assign",
		auditOpts{Details: fmt.Sprintf("Created %d assignments", count), Success: true})
	utils.Success(w, map[string]int{"created": count})
}

func (a *API) DeleteAssignmentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["userId"]
	serverID := vars["serverId"]

	if err := a.db.Where("user_id = ? AND server_id = ?", userID, serverID).Delete(&models.UserServer{}).Error; err != nil {
		a.internalError(w, r, err, "Failed to delete assignment")
		return
	}

	claims := claimsFrom(r)
	a.audit(r, claims, "delete_assignment",
		auditOpts{Details: fmt.Sprintf("Removed assignment of server %s from user %s", serverID, userID), Success: true})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListAssignmentsHandler(w http.ResponseWriter, r *http.Request) {
	var assignments []models.UserServer
	if err := a.db.Find(&assignments).Error; err != nil {
		a.internalError(w, r, err, "Failed to list assignments")
		return
	}
	utils.Success(w, assignments)
}

// PermissionMatrixHandler answers "who can touch what": all users with their
// effective permissions per server.
func (a *API) PermissionMatrixHandler(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	var servers []models.Server
	a.db.Where("disabled = ?", false).Find(&users)
	a.db.Find(&servers)

	type cell struct {
		UserID   uint         `json:"user_id"`
		ServerID uint         `json:"server_id"`
		Perms    models.Perms `json:"perms"`
	}
	matrix := []cell{}
	for _, u := range users {
		claims := &auth.Claims{UserID: u.ID, Username: u.Username, IsAdmin: u.IsAdmin}
		for i := range servers {
			perms, _, status := a.resolvePerms(claims, &servers[i])
			if status == http.StatusOK {
				matrix = append(matrix, cell{UserID: u.ID, ServerID: servers[i].ID, Perms: perms})
			}
		}
	}
	utils.Success(w, matrix)
}
