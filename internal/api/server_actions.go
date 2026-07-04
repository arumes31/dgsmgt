package api

import (
	"context"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"fmt"
	"net/http"

	"github.com/gorilla/mux"
	"go.uber.org/zap"
)

// ---- Actions ----------------------------------------------------------------

func permForAction(p models.Perms, action string) bool {
	switch action {
	case "start":
		return p.CanStart
	case "stop", "kill":
		return p.CanStop
	case "restart":
		return p.CanRestart
	case "pause", "unpause":
		return p.CanStop
	}
	return false
}

func (a *API) runAction(ctx context.Context, svc *docker.Service, server *models.Server, action string) error {
	switch action {
	case "start":
		return svc.Start(ctx, server.ContainerID)
	case "stop":
		return svc.Stop(ctx, server.ContainerID, server.StopTimeout, server.StopSignal)
	case "restart":
		return svc.Restart(ctx, server.ContainerID, server.StopTimeout, server.StopSignal)
	case "kill":
		return svc.Kill(ctx, server.ContainerID)
	case "pause":
		return svc.Pause(ctx, server.ContainerID)
	case "unpause":
		return svc.Unpause(ctx, server.ContainerID)
	}
	return fmt.Errorf("invalid action")
}

func (a *API) ActionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	action := vars["action"]

	claims := claimsFrom(r)
	perms, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	if !permForAction(perms, action) {
		a.audit(r, claims, action, auditOpts{Server: server, Details: "Permission denied", Success: false})
		utils.Forbidden(w, "Permission denied for this action")
		return
	}

	svc, ok := a.svcFor(w, server)
	if !ok {
		a.audit(r, claims, action, auditOpts{Server: server, Details: "Node offline", Success: false})
		return
	}
	if err := a.runAction(r.Context(), svc, server, action); err != nil {
		a.logger.Error("Docker action failed", zap.String("id", id), zap.String("action", action), zap.Error(err))
		a.audit(r, claims, action, auditOpts{Server: server, Details: err.Error(), Success: false})
		a.internalError(w, r, err, "Action failed")
		return
	}

	a.audit(r, claims, action, auditOpts{Server: server, Details: "Triggered container action", Success: true})
	utils.Success(w, map[string]string{"status": "ok"})
}

// svcFor resolves the Docker service for a server's node. Unlike a silent
// local fallback, an unreachable node is reported to the caller so actions
// can never run against the wrong Docker host.
func (a *API) svcFor(w http.ResponseWriter, server *models.Server) (*docker.Service, bool) {
	svc, err := a.dockerFor(server)
	if err != nil {
		a.logger.Warn("node unreachable", zap.Uint("node", server.NodeID), zap.Error(err))
		utils.JSON(w, http.StatusServiceUnavailable, nil, "Node offline: cannot reach this server's Docker host", nil)
		return nil, false
	}
	return svc, true
}

// BulkActionHandler runs one action on many servers; "start" honors
// depends_on ordering.
func (a *API) BulkActionHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ServerIDs []uint `json:"server_ids" validate:"required,min=1"`
		Action    string `json:"action" validate:"required,oneof=start stop restart kill pause unpause"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	claims := claimsFrom(r)

	type result struct {
		ServerID uint   `json:"server_id"`
		Name     string `json:"name"`
		OK       bool   `json:"ok"`
		Error    string `json:"error,omitempty"`
	}

	// Collect allowed servers
	var targets []*models.Server
	results := []result{}
	for _, sid := range input.ServerIDs {
		perms, server, status := a.getAccessByServerID(claims, sid)
		if status != http.StatusOK || !permForAction(perms, input.Action) {
			results = append(results, result{ServerID: sid, OK: false, Error: "access denied"})
			continue
		}
		targets = append(targets, server)
	}

	// Dependency ordering for start: repeatedly start servers whose
	// dependency is already handled.
	if input.Action == "start" {
		targets = orderByDependency(targets)
	}

	for _, server := range targets {
		svc, err := a.dockerFor(server)
		if err == nil {
			err = a.runAction(r.Context(), svc, server, input.Action)
		} else {
			err = fmt.Errorf("node offline")
		}
		res := result{ServerID: server.ID, Name: server.Name, OK: err == nil}
		if err != nil {
			res.Error = err.Error()
		}
		a.audit(r, claims, "bulk_"+input.Action, auditOpts{Server: server, Details: "Bulk action", Success: err == nil})
		results = append(results, res)
	}
	utils.Success(w, results)
}

func orderByDependency(servers []*models.Server) []*models.Server {
	byName := map[string]bool{}
	ordered := []*models.Server{}
	remaining := append([]*models.Server{}, servers...)
	for pass := 0; pass < len(servers)+1 && len(remaining) > 0; pass++ {
		next := []*models.Server{}
		for _, s := range remaining {
			if s.DependsOn == "" || byName[s.DependsOn] {
				ordered = append(ordered, s)
				byName[s.Name] = true
			} else {
				next = append(next, s)
			}
		}
		if len(next) == len(remaining) { // cycle or missing dep: give up ordering
			ordered = append(ordered, next...)
			break
		}
		remaining = next
	}
	return ordered
}
