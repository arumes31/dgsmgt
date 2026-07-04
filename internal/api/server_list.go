package api

import (
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"net/http"

	"github.com/gorilla/mux"
)

// ---- Listing ---------------------------------------------------------------

type serverView struct {
	models.Server
	Status string            `json:"status"`
	Uptime string            `json:"uptime"`
	Health docker.HealthInfo `json:"health"`
	Ports  []string          `json:"ports"`
	models.Perms
}

// ListMyServersHandler returns the servers visible to the caller with
// effective permissions (direct + group) and live docker status.
func (a *API) ListMyServersHandler(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r)

	var servers []models.Server
	if claims.IsAdmin {
		if err := a.db.Find(&servers).Error; err != nil {
			a.internalError(w, r, err, "Failed to list servers")
			return
		}
	} else {
		// direct assignments + group grants
		if err := a.db.Distinct("servers.*").
			Joins("LEFT JOIN user_servers ON user_servers.server_id = servers.id AND user_servers.user_id = ?", claims.UserID).
			Joins("LEFT JOIN group_servers ON group_servers.server_id = servers.id").
			Joins("LEFT JOIN user_groups ON user_groups.group_id = group_servers.group_id AND user_groups.user_id = ?", claims.UserID).
			Where("user_servers.user_id IS NOT NULL OR user_groups.user_id IS NOT NULL").
			Find(&servers).Error; err != nil {
			a.internalError(w, r, err, "Failed to list servers")
			return
		}
	}

	// One docker list per node used.
	statusByNode := map[uint]map[string]docker.ContainerInfo{}
	for _, s := range servers {
		if _, done := statusByNode[s.NodeID]; done {
			continue
		}
		statusByNode[s.NodeID] = map[string]docker.ContainerInfo{}
		if svc, err := a.nodes.For(s.NodeID); err == nil {
			if containers, err := svc.List(r.Context()); err == nil {
				for _, c := range containers {
					statusByNode[s.NodeID][c.ID] = c
				}
			}
		}
	}

	result := []serverView{}
	for _, s := range servers {
		perms, _, status := a.resolvePerms(claims, &s)
		if status != http.StatusOK {
			continue
		}
		sv := serverView{Server: s, Perms: perms, Status: "offline", Health: docker.HealthInfo{Status: "none"}}
		if info, ok := statusByNode[s.NodeID][s.ContainerID]; ok {
			sv.Status = info.State
			sv.Uptime = info.Uptime
			sv.Ports = info.Ports
		}
		// docker health needs inspect; fetch only for running servers
		if sv.Status == "running" && s.HealthCheckType == "docker" {
			if svc, err := a.nodes.For(s.NodeID); err == nil {
				if info, err := svc.GetStatus(r.Context(), s.ContainerID); err == nil {
					sv.Health = info.Health
					sv.Uptime = info.Uptime
				}
			}
		}
		result = append(result, sv)
	}

	utils.Success(w, result)
}

func (a *API) StatusHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	svc, err := a.dockerFor(server)
	if err != nil {
		utils.BadRequest(w, "Node offline")
		return
	}
	info, err := svc.GetStatus(r.Context(), server.ContainerID)
	if err != nil {
		utils.JSON(w, http.StatusNotFound, map[string]string{"status": "not_found"}, "container not found", nil)
		return
	}
	utils.Success(w, info)
}
