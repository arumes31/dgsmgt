package api

import (
	"dgsmgt/internal/docker"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gorilla/mux"
)

// ---- Orphans / adoption ------------------------------------------------------

// ListOrphansHandler shows host containers not managed by the panel and DB
// servers whose containers vanished.
func (a *API) ListOrphansHandler(w http.ResponseWriter, r *http.Request) {
	nodeID := uint(0)
	svc, err := a.nodes.For(nodeID)
	if err != nil {
		utils.BadRequest(w, "Node offline")
		return
	}
	containers, err := svc.List(r.Context())
	if err != nil {
		a.internalError(w, r, err, "Failed to list containers")
		return
	}

	var servers []models.Server
	a.db.Find(&servers)
	known := map[string]bool{}
	for _, s := range servers {
		known[s.ContainerID] = true
	}

	unmanaged := []docker.ContainerInfo{}
	for _, c := range containers {
		if !known[c.ID] {
			unmanaged = append(unmanaged, c)
		}
	}

	missing := []models.Server{}
	byID := map[string]bool{}
	for _, c := range containers {
		byID[c.ID] = true
	}
	for _, s := range servers {
		if s.ContainerID == "" || !byID[s.ContainerID] {
			missing = append(missing, s)
		}
	}

	utils.Success(w, map[string]interface{}{
		"unmanaged_containers": unmanaged,
		"missing_containers":   missing,
	})
}

// AdoptContainerHandler imports an existing container as a managed server.
func (a *API) AdoptContainerHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		ContainerID string `json:"container_id" validate:"required"`
		Name        string `json:"name" validate:"required,min=3,max=64"`
		NodeID      uint   `json:"node_id"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	claims := claimsFrom(r)

	svc, err := a.nodes.For(input.NodeID)
	if err != nil {
		utils.BadRequest(w, "Node offline")
		return
	}
	inspect, err := svc.Inspect(r.Context(), input.ContainerID)
	if err != nil {
		utils.NotFound(w, "Container not found")
		return
	}

	cc := containerConfig{Env: inspect.Config.Env}
	for _, m := range inspect.Mounts {
		cc.Volumes = append(cc.Volumes, fmt.Sprintf("%s:%s", m.Source, m.Destination))
	}
	if inspect.HostConfig != nil {
		for port, bindings := range inspect.HostConfig.PortBindings {
			for _, b := range bindings {
				cc.Ports = append(cc.Ports, fmt.Sprintf("%s:%s", b.HostPort, port.Port()))
			}
		}
	}
	b, _ := json.Marshal(cc)

	server := models.Server{
		Name:        input.Name,
		ContainerID: inspect.ID,
		Image:       inspect.Config.Image,
		ConfigJSON:  string(b),
		NodeID:      input.NodeID,
	}
	if err := a.db.Create(&server).Error; err != nil {
		a.internalError(w, r, err, "Failed to adopt container")
		return
	}
	a.audit(r, claims, "adopt_container", auditOpts{Server: &server, Details: "Adopted existing container", Success: true})
	utils.Created(w, server)
}

// ---- Utility endpoints ---------------------------------------------------------

func (a *API) NetworksHandler(w http.ResponseWriter, r *http.Request) {
	nets, err := a.nodes.Local().Networks(r.Context())
	if err != nil {
		a.internalError(w, r, err, "Failed to list networks")
		return
	}
	utils.Success(w, nets)
}

// ImageTagsHandler lists tags from Docker Hub for the create form.
func (a *API) ImageTagsHandler(w http.ResponseWriter, r *http.Request) {
	img := r.URL.Query().Get("image")
	if img == "" || strings.Contains(img, "..") {
		utils.BadRequest(w, "image parameter required")
		return
	}
	repo := strings.Split(img, ":")[0]
	if !strings.Contains(repo, "/") {
		repo = "library/" + repo
	}
	if strings.Count(repo, "/") > 1 {
		utils.Success(w, []string{}) // non-hub registry: no tag listing
		return
	}

	client := http.Client{Timeout: 8 * time.Second}
	resp, err := client.Get(fmt.Sprintf("https://registry.hub.docker.com/v2/repositories/%s/tags?page_size=50", repo))
	if err != nil {
		utils.Success(w, []string{})
		return
	}
	defer func() { _ = resp.Body.Close() }()
	var body struct {
		Results []struct {
			Name string `json:"name"`
		} `json:"results"`
	}
	tags := []string{}
	if json.NewDecoder(resp.Body).Decode(&body) == nil {
		for _, t := range body.Results {
			tags = append(tags, t.Name)
		}
	}
	utils.Success(w, tags)
}

func (a *API) DiskUsageHandler(w http.ResponseWriter, r *http.Request) {
	id := mux.Vars(r)["id"]
	claims := claimsFrom(r)
	_, server, status := a.getAccess(claims, id)
	if status != http.StatusOK {
		writeAccessStatus(w, status)
		return
	}
	svc := mustSvc(a, server)
	sizeRw, sizeRootFs, err := svc.DiskUsage(r.Context(), server.ContainerID)
	if err != nil {
		a.internalError(w, r, err, "Failed to read disk usage")
		return
	}

	volumes := map[string]int64{}
	if inspect, err := svc.Inspect(r.Context(), server.ContainerID); err == nil {
		for _, m := range inspect.Mounts {
			if kb, err := svc.DirSizeKB(r.Context(), server.ContainerID, m.Destination); err == nil {
				volumes[m.Destination] = kb * 1024
			}
		}
	}

	utils.Success(w, map[string]interface{}{
		"size_rw":      sizeRw,
		"size_root_fs": sizeRootFs,
		"volumes":      volumes,
	})
}
