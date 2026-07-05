package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

// serverInput is the shared payload for create/update.
// ---- Admin: CRUD ---------------------------------------------------------------

func (a *API) ListServersHandler(w http.ResponseWriter, r *http.Request) {
	var servers []models.Server
	if err := a.db.Find(&servers).Error; err != nil {
		a.internalError(w, r, err, "Failed to list servers")
		return
	}
	utils.Success(w, servers)
}

// PortConflicts returns host ports in the request that are already bound.
func (a *API) PortCheckHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Ports  []string `json:"ports"`
		NodeID uint     `json:"node_id"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	svc, err := a.nodes.For(input.NodeID)
	if err != nil {
		utils.BadRequest(w, "Node offline")
		return
	}
	containers, err := svc.List(r.Context())
	if err != nil {
		a.internalError(w, r, err, "Failed to list containers")
		return
	}
	used := map[string]string{} // hostPort -> container name
	for _, c := range containers {
		for _, p := range c.Ports {
			// formats: "0.0.0.0:25565->25565/tcp"
			if idx := strings.Index(p, "->"); idx > 0 {
				hp := p[:idx]
				if colon := strings.LastIndex(hp, ":"); colon >= 0 {
					hp = hp[colon+1:]
				}
				name := c.ID[:12]
				if len(c.Names) > 0 {
					name = strings.TrimPrefix(c.Names[0], "/")
				}
				used[hp] = name
			}
		}
	}
	conflicts := map[string]string{}
	for _, spec := range input.Ports {
		// specs like "25565:25565" or "0.0.0.0:25565:25565/udp"
		parts := strings.Split(strings.Split(spec, "/")[0], ":")
		hostPort := ""
		switch len(parts) {
		case 2:
			hostPort = parts[0]
		case 3:
			hostPort = parts[1]
		}
		if hostPort != "" {
			if name, ok := used[hostPort]; ok {
				conflicts[hostPort] = name
			}
		}
	}
	utils.Success(w, map[string]interface{}{"conflicts": conflicts})
}

func (a *API) CreateServerHandler(w http.ResponseWriter, r *http.Request) {
	var input serverInput
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	claims := claimsFrom(r)

	svc, err := a.nodes.For(input.NodeID)
	if err != nil {
		utils.BadRequest(w, "Node offline or unknown")
		return
	}

	server := models.Server{}
	applyInput(&server, &input)

	containerID, err := svc.Create(r.Context(), a.createOptsFor(&server))
	if err != nil {
		a.audit(r, claims, "create_server", auditOpts{Details: err.Error(), Success: false})
		a.internalError(w, r, err, "Docker create failed — check image, ports and volumes")
		return
	}
	server.ContainerID = containerID

	if err := a.db.Create(&server).Error; err != nil {
		_ = svc.Delete(r.Context(), containerID, true)
		a.internalError(w, r, err, "Failed to save server")
		return
	}

	a.scheduler.Reload()
	a.audit(r, claims, "create_server", auditOpts{Server: &server, Details: "Created new server and container", Success: true})
	utils.Created(w, server)
}

// UpdateServerHandler edits a server; container-affecting changes trigger a
// recreate (volumes are host binds, data survives).
func (a *API) UpdateServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var server models.Server
	if err := a.db.First(&server, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(w, "Server not found")
		} else {
			a.internalError(w, r, err, "Failed to load server")
		}
		return
	}

	// Non-admins may edit only with CanEditConfig.
	if !claims.IsAdmin {
		perms, _, status := a.resolvePerms(claims, &server)
		if status != http.StatusOK || !perms.CanEditConfig {
			utils.Forbidden(w, "Permission denied: cannot edit configuration")
			return
		}
	}

	var input serverInput
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	before := server
	oldOpts := a.createOptsFor(&server)
	applyInput(&server, &input)
	newOpts := a.createOptsFor(&server)

	needsRecreate := before.Image != server.Image ||
		before.NetworkMode != server.NetworkMode ||
		before.RestartPolicy != server.RestartPolicy ||
		before.CPULimit != server.CPULimit ||
		before.MemoryLimitMB != server.MemoryLimitMB ||
		before.StopSignal != server.StopSignal ||
		before.StopTimeout != server.StopTimeout ||
		fmt.Sprint(oldOpts.Ports) != fmt.Sprint(newOpts.Ports) ||
		fmt.Sprint(oldOpts.Env) != fmt.Sprint(newOpts.Env) ||
		fmt.Sprint(oldOpts.Volumes) != fmt.Sprint(newOpts.Volumes) ||
		fmt.Sprint(oldOpts.Cmd) != fmt.Sprint(newOpts.Cmd)

	svc, ok := a.svcFor(w, &server)
	if !ok {
		return
	}

	if before.Name != server.Name && !needsRecreate && server.ContainerID != "" {
		if err := svc.Rename(r.Context(), server.ContainerID, server.Name); err != nil {
			a.internalError(w, r, err, "Rename failed")
			return
		}
	}

	if needsRecreate && server.ContainerID != "" {
		newID, err := svc.Recreate(r.Context(), server.ContainerID, newOpts, false, nil)
		if err != nil {
			a.audit(r, claims, "update_server", auditOpts{Server: &server, Details: err.Error(), Success: false})
			a.internalError(w, r, err, "Recreate failed — the previous container was preserved")
			return
		}
		server.ContainerID = newID
	}

	if err := a.db.Save(&server).Error; err != nil {
		a.internalError(w, r, err, "Failed to save server")
		return
	}

	a.scheduler.Reload()
	a.audit(r, claims, "update_server", auditOpts{
		Server: &server, Details: "Updated server configuration", Success: true,
		Diff: map[string]interface{}{"before": before, "after": server},
	})
	utils.Success(w, server)
}

// RedeployHandler pulls the latest image and recreates the container,
// streaming pull progress over the websocket.
func (a *API) RedeployHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var server models.Server
	if err := a.db.First(&server, id).Error; err != nil {
		utils.NotFound(w, "Server not found")
		return
	}
	if !claims.IsAdmin {
		perms, _, status := a.resolvePerms(claims, &server)
		if status != http.StatusOK || !perms.CanEditConfig {
			utils.Forbidden(w, "Permission denied")
			return
		}
	}

	svc, ok := a.svcFor(w, &server)
	if !ok {
		return
	}

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer func() { _ = conn.Close() }()
	send := func(line string) { _ = conn.WriteMessage(websocket.TextMessage, []byte(line)) }

	newID, err := svc.Recreate(r.Context(), server.ContainerID, a.createOptsFor(&server), true, send)
	if err != nil {
		send(`{"error":"` + strings.ReplaceAll(err.Error(), `"`, `'`) + `"}`)
		a.audit(r, claims, "redeploy", auditOpts{Server: &server, Details: err.Error(), Success: false})
		return
	}
	server.ContainerID = newID
	if err := a.db.Save(&server).Error; err != nil {
		a.logger.Error("redeploy: failed to persist new container id", zap.Error(err))
		a.audit(r, claims, "redeploy", auditOpts{Server: &server, Details: "Recreated but DB save failed: " + err.Error(), Success: false})
		send(`{"error":"container recreated but saving the new container id failed — check server logs"}`)
		return
	}
	a.audit(r, claims, "redeploy", auditOpts{Server: &server, Details: "Pulled latest image and recreated container", Success: true})
	send(`{"done":true}`)
}

// DeleteServerHandler soft-deletes: the container is removed but volume data
// and the DB record stay (Trash). Root can purge permanently.
func (a *API) DeleteServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var server models.Server
	if err := a.db.First(&server, id).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			utils.NotFound(w, "Server not found")
		} else {
			a.internalError(w, r, err, "Failed to load server")
		}
		return
	}

	if server.ContainerID != "" {
		svc, ok := a.svcFor(w, &server)
		if !ok {
			return
		}
		if err := svc.Delete(r.Context(), server.ContainerID, true); err != nil {
			a.logger.Warn("Failed to delete container during server deletion",
				zap.String("id", server.ContainerID), zap.Error(err))
		}
	}

	if err := a.db.Delete(&server).Error; err != nil { // gorm soft delete
		a.internalError(w, r, err, "Failed to delete server")
		return
	}

	a.scheduler.Reload()
	a.audit(r, claims, "delete_server", auditOpts{Server: &server, Details: "Moved server to trash (volumes retained)", Success: true})
	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListTrashHandler(w http.ResponseWriter, r *http.Request) {
	var servers []models.Server
	if err := a.db.Unscoped().Where("deleted_at IS NOT NULL").Find(&servers).Error; err != nil {
		a.internalError(w, r, err, "Failed to list trash")
		return
	}
	utils.Success(w, servers)
}

// RestoreServerHandler recreates the container from stored config.
func (a *API) RestoreServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var server models.Server
	if err := a.db.Unscoped().First(&server, id).Error; err != nil {
		utils.NotFound(w, "Server not found")
		return
	}

	svc, ok := a.svcFor(w, &server)
	if !ok {
		return
	}
	containerID, err := svc.Create(r.Context(), a.createOptsFor(&server))
	if err != nil {
		a.internalError(w, r, err, "Failed to recreate container from stored config")
		return
	}

	server.ContainerID = containerID
	server.DeletedAt = gorm.DeletedAt{}
	if err := a.db.Unscoped().Save(&server).Error; err != nil {
		a.internalError(w, r, err, "Failed to restore server")
		return
	}
	a.scheduler.Reload()
	a.audit(r, claims, "restore_server", auditOpts{Server: &server, Details: "Restored server from trash", Success: true})
	utils.Success(w, server)
}

// PurgeServerHandler permanently deletes a trashed server (root only,
// enforced by route middleware).
func (a *API) PurgeServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)

	var server models.Server
	if err := a.db.Unscoped().First(&server, id).Error; err != nil {
		utils.NotFound(w, "Server not found")
		return
	}
	if !server.DeletedAt.Valid {
		utils.BadRequest(w, "Server must be in trash before purging")
		return
	}

	// Purge atomically so a partial failure can't leave orphaned rows.
	if err := a.db.Transaction(func(tx *gorm.DB) error {
		for _, model := range []interface{}{
			&models.UserServer{}, &models.GroupServer{}, &models.MetricSample{},
			&models.CommandSnippet{}, &models.LogAlert{},
		} {
			if err := tx.Where("server_id = ?", server.ID).Delete(model).Error; err != nil {
				return err
			}
		}
		return tx.Unscoped().Delete(&server).Error
	}); err != nil {
		a.internalError(w, r, err, "Failed to purge server")
		return
	}
	a.audit(r, claims, "purge_server", auditOpts{Server: &server, Details: "Permanently deleted server", Success: true})
	w.WriteHeader(http.StatusNoContent)
}

// CloneServerHandler duplicates a server's config under a new name (ports
// must be adjusted by the caller; container is created stopped).
func (a *API) CloneServerHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	var input struct {
		Name  string   `json:"name" validate:"required,min=3,max=64"`
		Ports []string `json:"ports"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}

	var src models.Server
	if err := a.db.First(&src, id).Error; err != nil {
		utils.NotFound(w, "Server not found")
		return
	}

	clone := src
	clone.Model = gorm.Model{}
	clone.Name = input.Name
	clone.ContainerID = ""
	var cc containerConfig
	_ = json.Unmarshal([]byte(src.ConfigJSON), &cc)
	if len(input.Ports) > 0 {
		cc.Ports = input.Ports
	}
	b, _ := json.Marshal(cc)
	clone.ConfigJSON = string(b)

	svc, ok := a.svcFor(w, &clone)
	if !ok {
		return
	}
	containerID, err := svc.Create(r.Context(), a.createOptsFor(&clone))
	if err != nil {
		a.internalError(w, r, err, "Docker create failed for the clone")
		return
	}
	clone.ContainerID = containerID
	if err := a.db.Create(&clone).Error; err != nil {
		_ = svc.Delete(r.Context(), containerID, true)
		a.internalError(w, r, err, "Failed to save clone")
		return
	}
	a.audit(r, claims, "clone_server", auditOpts{Server: &clone, Details: "Cloned from " + src.Name, Success: true})
	utils.Created(w, clone)
}
