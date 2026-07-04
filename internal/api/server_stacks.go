package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"net/http"
	"strings"

	"github.com/gorilla/mux"
)

// ---- Stacks (multi-container deployments) ---------------------------------------

func (a *API) ListStacksHandler(w http.ResponseWriter, r *http.Request) {
	var stacks []models.Stack
	if err := a.db.Find(&stacks).Error; err != nil {
		a.internalError(w, r, err, "Failed to list stacks")
		return
	}
	utils.Success(w, stacks)
}

func (a *API) CreateStackHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name string `json:"name" validate:"required,min=3,max=64"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	network := "dgsmgt-" + strings.ToLower(input.Name)
	if err := a.nodes.Local().EnsureNetwork(r.Context(), network); err != nil {
		a.internalError(w, r, err, "Failed to create the stack network")
		return
	}
	stack := models.Stack{Name: input.Name, Network: network}
	if err := a.db.Create(&stack).Error; err != nil {
		a.internalError(w, r, err, "Failed to create stack")
		return
	}
	a.audit(r, claimsFrom(r), "create_stack", auditOpts{Details: "Created stack " + input.Name, Success: true})
	utils.Created(w, stack)
}

func (a *API) DeleteStackHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	if err := a.db.Model(&models.Server{}).Where("stack_id = ?", id).Update("stack_id", 0).Error; err != nil {
		a.internalError(w, r, err, "Failed to detach servers from stack")
		return
	}
	// The stack's Docker network is intentionally kept: detached containers
	// may still be attached to it; remove it manually once unused.
	if err := a.db.Delete(&models.Stack{}, id).Error; err != nil {
		a.internalError(w, r, err, "Failed to delete stack")
		return
	}
	a.audit(r, claimsFrom(r), "delete_stack", auditOpts{Details: "Deleted stack " + id, Success: true})
	w.WriteHeader(http.StatusNoContent)
}

// StackActionHandler starts/stops all servers in a stack in dependency order.
func (a *API) StackActionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	action := vars["action"]
	if action != "start" && action != "stop" {
		utils.BadRequest(w, "Invalid action")
		return
	}
	claims := claimsFrom(r)

	var servers []models.Server
	if err := a.db.Where("stack_id = ?", id).Find(&servers).Error; err != nil {
		a.internalError(w, r, err, "Failed to load stack servers")
		return
	}
	ptrs := make([]*models.Server, len(servers))
	for i := range servers {
		ptrs[i] = &servers[i]
	}
	ptrs = orderByDependency(ptrs)
	if action == "stop" { // stop in reverse order
		for i, j := 0, len(ptrs)-1; i < j; i, j = i+1, j-1 {
			ptrs[i], ptrs[j] = ptrs[j], ptrs[i]
		}
	}

	results := []map[string]interface{}{}
	for _, s := range ptrs {
		svc, err := a.dockerFor(s)
		if err == nil {
			err = a.runAction(r.Context(), svc, s, action)
		}
		results = append(results, map[string]interface{}{"server": s.Name, "ok": err == nil})
		a.audit(r, claims, "stack_"+action, auditOpts{Server: s, Success: err == nil})
	}
	utils.Success(w, results)
}
