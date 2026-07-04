package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"fmt"
	"net/http"
	"strconv"

	"github.com/gorilla/mux"
)

// ---- Groups -----------------------------------------------------------------------

func (a *API) ListGroupsHandler(w http.ResponseWriter, r *http.Request) {
	var groups []models.Group
	if err := a.db.Find(&groups).Error; err != nil {
		a.internalError(w, r, err, "Failed to list groups")
		return
	}
	type groupView struct {
		models.Group
		Members []uint               `json:"members"`
		Servers []models.GroupServer `json:"servers"`
	}
	out := []groupView{}
	for _, g := range groups {
		gv := groupView{Group: g, Members: []uint{}, Servers: []models.GroupServer{}}
		var memberships []models.UserGroup
		a.db.Where("group_id = ?", g.ID).Find(&memberships)
		for _, m := range memberships {
			gv.Members = append(gv.Members, m.UserID)
		}
		a.db.Where("group_id = ?", g.ID).Find(&gv.Servers)
		out = append(out, gv)
	}
	utils.Success(w, out)
}

func (a *API) CreateGroupHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name" validate:"required,min=2,max=64"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	group := models.Group{Name: input.Name}
	if err := a.db.Create(&group).Error; err != nil {
		utils.BadRequest(w, "Group already exists")
		return
	}
	a.audit(r, claimsFrom(r), "create_group", auditOpts{Details: "Created group " + input.Name, Success: true})
	utils.Created(w, group)
}

func (a *API) DeleteGroupHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	a.db.Where("group_id = ?", id).Delete(&models.UserGroup{})
	a.db.Where("group_id = ?", id).Delete(&models.GroupServer{})
	if err := a.db.Delete(&models.Group{}, id).Error; err != nil {
		a.internalError(w, r, err, "Failed to delete group")
		return
	}
	a.audit(r, claimsFrom(r), "delete_group", auditOpts{Details: "Deleted group " + id, Success: true})
	w.WriteHeader(http.StatusNoContent)
}

// GroupMembersHandler sets the full member list of a group.
func (a *API) GroupMembersHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var input struct {
		UserIDs []uint `json:"user_ids"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	gid, err := strconv.Atoi(id)
	if err != nil || gid <= 0 {
		utils.BadRequest(w, "Invalid group id")
		return
	}
	a.db.Where("group_id = ?", gid).Delete(&models.UserGroup{})
	for _, uid := range input.UserIDs {
		a.db.Create(&models.UserGroup{UserID: uid, GroupID: uint(gid)})
	}
	a.audit(r, claimsFrom(r), "update_group_members",
		auditOpts{Details: fmt.Sprintf("Group %s now has %d members", id, len(input.UserIDs)), Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// GroupServersHandler grants a group access to a server with permissions.
func (a *API) GroupServersHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	var input struct {
		ServerID uint   `json:"server_id" validate:"required"`
		Preset   string `json:"preset" validate:"omitempty,oneof=viewer operator owner"`
		models.Perms
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	gid, err := strconv.Atoi(id)
	if err != nil || gid <= 0 {
		utils.BadRequest(w, "Invalid group id")
		return
	}

	perms := input.Perms
	if input.Preset != "" {
		perms = presetPerms(input.Preset)
	}
	gs := models.GroupServer{
		GroupID: uint(gid), ServerID: input.ServerID,
		CanStart: perms.CanStart, CanStop: perms.CanStop, CanRestart: perms.CanRestart,
		CanViewLogs: perms.CanViewLogs, CanSendCommands: perms.CanSendCommands,
		CanEditConfig: perms.CanEditConfig, CanAccessFiles: perms.CanAccessFiles,
		CanManageBackups: perms.CanManageBackups,
	}
	if err := a.db.Save(&gs).Error; err != nil {
		a.internalError(w, r, err, "Failed to save group grant")
		return
	}
	a.audit(r, claimsFrom(r), "group_grant",
		auditOpts{Details: fmt.Sprintf("Granted group %d access to server %d", gid, input.ServerID), Success: true})
	utils.Success(w, gs)
}

func (a *API) DeleteGroupServerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	if err := a.db.Where("group_id = ? AND server_id = ?", vars["id"], vars["serverId"]).
		Delete(&models.GroupServer{}).Error; err != nil {
		a.internalError(w, r, err, "Failed to remove group grant")
		return
	}
	a.audit(r, claimsFrom(r), "group_grant_removed",
		auditOpts{Details: fmt.Sprintf("Removed server %s grant from group %s", vars["serverId"], vars["id"]), Success: true})
	w.WriteHeader(http.StatusNoContent)
}
