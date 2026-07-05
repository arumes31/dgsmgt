package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// rconHostFor returns the address the panel uses to reach a published RCON
// port: the Docker host for local deployments (host-gateway alias, see
// docker-compose.yml), or the remote node's hostname.
func (a *API) rconHostFor(nodeID uint) string {
	if nodeID != 0 {
		var n models.Node
		if err := a.db.First(&n, nodeID).Error; err == nil {
			if u, err := url.Parse(n.Address); err == nil && u.Hostname() != "" {
				return u.Hostname()
			}
			// Scheme-less "host:port" addresses parse as opaque URLs above.
			if host, _, err := net.SplitHostPort(n.Address); err == nil && host != "" {
				return host
			}
		}
	}
	return "host.docker.internal"
}

// ListTemplatesHandler returns built-in templates plus any community
// templates fetched from the admin-configured template_url setting.
func (a *API) ListTemplatesHandler(w http.ResponseWriter, r *http.Request) {
	templates := append([]GameTemplate{}, builtinTemplates...)

	var s models.Setting
	if err := a.db.First(&s, "key = ?", "template_url").Error; err == nil && s.Value != "" {
		client := http.Client{Timeout: 8 * time.Second}
		if resp, err := client.Get(s.Value); err == nil {
			var remote []GameTemplate
			if json.NewDecoder(resp.Body).Decode(&remote) == nil {
				for i := range remote {
					remote[i].Category = "Community: " + remote[i].Category
				}
				templates = append(templates, remote...)
			}
			_ = resp.Body.Close()
		}
	}
	utils.Success(w, templates)
}

// portRange parses SERVER_GAME_PORTRANGE.
func (a *API) portRange() (int, int) {
	parts := strings.SplitN(a.cfg.GamePortRange, "-", 2)
	lo, hi := 25000, 30000
	if len(parts) == 2 {
		if v, err := strconv.Atoi(strings.TrimSpace(parts[0])); err == nil {
			lo = v
		}
		if v, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
			hi = v
		}
	}
	if hi < lo {
		lo, hi = hi, lo
	}
	return lo, hi
}

// usedHostPorts collects host ports bound by containers on a node plus ports
// recorded in the DB (including stopped/trashed servers).
func (a *API) usedHostPorts(r *http.Request, nodeID uint) map[int]bool {
	used := map[int]bool{}
	if svc, err := a.nodes.For(nodeID); err == nil {
		if containers, err := svc.List(r.Context()); err == nil {
			for _, c := range containers {
				for _, p := range c.Ports {
					if idx := strings.Index(p, "->"); idx > 0 {
						hp := p[:idx]
						if colon := strings.LastIndex(hp, ":"); colon >= 0 {
							hp = hp[colon+1:]
						}
						if v, err := strconv.Atoi(hp); err == nil {
							used[v] = true
						}
					}
				}
			}
		}
	}
	var servers []models.Server
	a.db.Unscoped().Where("node_id = ?", nodeID).Find(&servers)
	for _, s := range servers {
		var cc containerConfig
		if json.Unmarshal([]byte(s.ConfigJSON), &cc) == nil {
			for _, spec := range cc.Ports {
				parts := strings.Split(strings.Split(spec, "/")[0], ":")
				hostPort := ""
				switch len(parts) {
				case 2:
					hostPort = parts[0]
				case 3:
					hostPort = parts[1]
				}
				if v, err := strconv.Atoi(hostPort); err == nil {
					used[v] = true
				}
			}
		}
	}
	return used
}

// allocatePorts picks n free host ports from the configured range.
func (a *API) allocatePorts(r *http.Request, nodeID uint, n int) ([]int, error) {
	lo, hi := a.portRange()
	used := a.usedHostPorts(r, nodeID)
	out := []int{}
	for p := lo; p <= hi && len(out) < n; p++ {
		if !used[p] {
			out = append(out, p)
		}
	}
	if len(out) < n {
		return nil, fmt.Errorf("not enough free ports in range %d-%d", lo, hi)
	}
	return out, nil
}

// AllocatePortsHandler previews free ports: ?count=2&node_id=0
func (a *API) AllocatePortsHandler(w http.ResponseWriter, r *http.Request) {
	count, _ := strconv.Atoi(r.URL.Query().Get("count"))
	if count < 1 || count > 20 {
		count = 1
	}
	nodeID, _ := strconv.Atoi(r.URL.Query().Get("node_id"))
	ports, err := a.allocatePorts(r, uint(nodeID), count)
	if err != nil {
		utils.BadRequest(w, err.Error())
		return
	}
	utils.Success(w, map[string]interface{}{"ports": ports, "range": a.cfg.GamePortRange})
}

// DeployTemplateHandler creates a server from a template with automatic port
// allocation and volume paths.
func (a *API) DeployTemplateHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		TemplateID string   `json:"template_id" validate:"required"`
		Name       string   `json:"name" validate:"required,min=3,max=64"`
		NodeID     uint     `json:"node_id"`
		Env        []string `json:"env"`   // extra/override env
		Image      string   `json:"image"` // override (e.g. custom linuxgsm tag)
		Start      bool     `json:"start"`
	}
	if !a.decodeAndValidate(w, r, &input) {
		return
	}
	claims := claimsFrom(r)

	var tpl *GameTemplate
	for i := range builtinTemplates {
		if builtinTemplates[i].ID == input.TemplateID {
			t := builtinTemplates[i]
			tpl = &t
			break
		}
	}
	if tpl == nil {
		utils.NotFound(w, "Template not found")
		return
	}

	image := tpl.Image
	if input.Image != "" {
		image = input.Image
	}
	if strings.Contains(image, "{GAME}") {
		utils.BadRequest(w, "This template requires an explicit image (replace {GAME})")
		return
	}

	// Port allocation (one extra slot when the template exposes RCON)
	allPorts := tpl.Ports
	if tpl.RconPort != "" {
		allPorts = append(append([]string{}, tpl.Ports...), tpl.RconPort)
	}
	ports := []string{}
	hostPorts, err := a.allocatePorts(r, input.NodeID, len(allPorts))
	if err != nil {
		utils.BadRequest(w, err.Error())
		return
	}
	assigned := map[string]int{} // container port -> host port
	for i, cp := range allPorts {
		parts := strings.SplitN(cp, "/", 2)
		proto := "tcp"
		if len(parts) == 2 {
			proto = parts[1]
		}
		hp := hostPorts[i]
		if tpl.FixedPorts {
			// Game requires host port == container port (protocol constraint).
			if v, err := strconv.Atoi(parts[0]); err == nil {
				hp = v
			}
		} else if prev, ok := assigned[parts[0]]; ok {
			// Same game port over tcp+udp must share one host port: clients
			// expect both protocols on the port the server advertises.
			hp = prev
		}
		assigned[parts[0]] = hp
		ports = append(ports, fmt.Sprintf("%d:%s/%s", hp, parts[0], proto))
		hostPorts[i] = hp
	}

	// RCON preset: publish the port, generate a password and preconfigure
	// the console so commands work out of the box.
	rconHostPort := 0
	rconPassword := ""
	if tpl.RconPort != "" {
		rconHostPort = hostPorts[len(hostPorts)-1]
		tok, err := auth.RandomToken()
		if err != nil {
			a.internalError(w, r, err, "Failed to generate RCON password")
			return
		}
		rconPassword = tok[:16]
		hostPorts = hostPorts[:len(hostPorts)-1] // game ports only in the response
	}

	// Env with {PORTn} / {RCONPW} substitution. User-supplied entries
	// override template defaults of the same name (dedup keeps one entry
	// per variable so config_json stays clean across recreates).
	merged := append(append([]string{}, tpl.Env...), input.Env...)
	seen := map[string]int{}
	env := []string{}
	for _, e := range merged {
		name := strings.SplitN(e, "=", 2)[0]
		if idx, ok := seen[name]; ok {
			env[idx] = e
			continue
		}
		seen[name] = len(env)
		env = append(env, e)
	}
	for i, hp := range hostPorts {
		token := fmt.Sprintf("{PORT%d}", i)
		for j := range env {
			env[j] = strings.ReplaceAll(env[j], token, strconv.Itoa(hp))
		}
	}
	if rconPassword != "" {
		for j := range env {
			env[j] = strings.ReplaceAll(env[j], "{RCONPW}", rconPassword)
		}
	}

	// Volumes: host side auto-created under DataPath
	volumes := []string{}
	safeName := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, input.Name)
	for i, cpath := range tpl.Volumes {
		host := filepath.ToSlash(filepath.Join(a.cfg.DataPath, safeName, fmt.Sprintf("vol%d", i)))
		volumes = append(volumes, host+":"+cpath)
	}

	in := serverInput{
		Name: input.Name, Image: image, Ports: ports, Env: env, Volumes: volumes, Cmd: tpl.Cmd,
		NodeID: input.NodeID, Icon: tpl.Icon, Folder: tpl.Category,
		StopTimeout: tpl.StopTimeout, StopSignal: tpl.StopSignal,
		RestartPolicy: "unless-stopped", AutoRestart: true,
		HealthCheckType: "docker",
	}
	if rconHostPort != 0 {
		in.ConsoleMode = "rcon"
		in.RconHost = a.rconHostFor(input.NodeID)
		in.RconPort = rconHostPort
		in.RconPassword = rconPassword
	}
	server := models.Server{}
	applyInput(&server, &in)

	svc, err := a.nodes.For(input.NodeID)
	if err != nil {
		utils.BadRequest(w, "Node offline")
		return
	}
	containerID, err := svc.Create(r.Context(), a.createOptsFor(&server))
	if err != nil {
		a.audit(r, claims, "deploy_template", auditOpts{Details: err.Error(), Success: false})
		a.internalError(w, r, err, "Docker create failed while deploying the template")
		return
	}
	server.ContainerID = containerID
	if err := a.db.Create(&server).Error; err != nil {
		_ = svc.Delete(r.Context(), containerID, true)
		a.internalError(w, r, err, "Failed to save server")
		return
	}
	if input.Start {
		_ = svc.Start(r.Context(), containerID)
	}

	a.scheduler.Reload()
	a.audit(r, claims, "deploy_template", auditOpts{Server: &server,
		Details: fmt.Sprintf("Deployed template %s (ports %v)", tpl.Name, hostPorts), Success: true})
	utils.Created(w, map[string]interface{}{"server": server, "allocated_ports": hostPorts})
}
