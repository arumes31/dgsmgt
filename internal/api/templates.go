package api

import (
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// GameTemplate describes a one-click deployable game server. Container ports
// are given as "port/proto"; host ports are auto-allocated from the range in
// SERVER_GAME_PORTRANGE (default 25000-30000). "{PORT0}", "{PORT1}", ... in
// env values are replaced with the allocated host ports so games that must
// know their public port stay consistent. Volumes are container paths; the
// host side is created under SERVER_DATA_PATH/<server-name>/.
type GameTemplate struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Category    string   `json:"category"`
	Image       string   `json:"image"`
	Ports       []string `json:"ports"`       // container ports "25565/tcp"
	FixedPorts  bool     `json:"fixed_ports"` // host port must equal container port (game protocol requirement)
	Env         []string `json:"env"`
	Volumes     []string `json:"volumes"` // container paths
	StopTimeout int      `json:"stop_timeout"`
	StopSignal  string   `json:"stop_signal"`
	Icon        string   `json:"icon"`
	Notes       string   `json:"notes"`
}

// builtinTemplates is the curated catalog: the most-used community image per
// game, plus the LinuxGSM family (gameservermanagers/gameserver:<code>)
// covering 130+ titles.
var builtinTemplates = []GameTemplate{
	// ---- Survival, Crafting & Sandbox ----
	{ID: "minecraft-java", Name: "Minecraft (Java)", Category: "Survival & Sandbox",
		Image: "itzg/minecraft-server:latest", Ports: []string{"25565/tcp"},
		Env: []string{"EULA=TRUE", "MEMORY=4G", "TYPE=VANILLA"}, Volumes: []string{"/data"},
		StopTimeout: 60, Icon: "minecraft",
		Notes: "Gold standard image. Set TYPE=PAPER/FORGE/FABRIC and VERSION for modded servers."},
	{ID: "minecraft-bedrock", Name: "Minecraft (Bedrock)", Category: "Survival & Sandbox",
		Image: "itzg/minecraft-bedrock-server:latest", Ports: []string{"19132/udp"},
		Env: []string{"EULA=TRUE"}, Volumes: []string{"/data"}, StopTimeout: 60, Icon: "minecraft"},
	{ID: "valheim", Name: "Valheim", Category: "Survival & Sandbox",
		Image: "lloesche/valheim-server:latest", Ports: []string{"2456/udp", "2457/udp"}, FixedPorts: true,
		Env:     []string{"SERVER_NAME=DGSMgt Valheim", "WORLD_NAME=Dedicated", "SERVER_PASS=secret", "SERVER_PUBLIC=false", "BACKUPS=true"},
		Volumes: []string{"/config", "/opt/valheim"}, StopTimeout: 120, StopSignal: "SIGINT", Icon: "valheim",
		Notes: "Supports automatic world backups and crossplay (add CROSSPLAY=true)."},
	{ID: "ark-se", Name: "ARK: Survival Evolved (LinuxGSM)", Category: "Survival & Sandbox",
		Image: "gameservermanagers/gameserver:ark", Ports: []string{"7777/udp", "7778/udp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "ark"},
	{ID: "ark-sa", Name: "ARK: Survival Ascended", Category: "Survival & Sandbox",
		Image: "mschnitzer/asa-linux-server:latest", Ports: []string{"7777/udp"}, FixedPorts: true,
		Env: []string{"ASA_START_PARAMS=TheIsland_WP?listen?Port=7777"}, Volumes: []string{"/home/gameserver/server-files"},
		StopTimeout: 120, Icon: "ark"},
	{ID: "rust", Name: "Rust", Category: "Survival & Sandbox",
		Image: "didstopia/rust-server:latest", Ports: []string{"28015/udp", "28016/tcp"}, FixedPorts: true,
		Env:     []string{"RUST_SERVER_NAME=DGSMgt Rust", "RUST_SERVER_WORLDSIZE=3500", "RUST_RCON_PASSWORD=changeme"},
		Volumes: []string{"/steamcmd/rust"}, StopTimeout: 120, Icon: "rust",
		Notes: "RCON on 28016/tcp — set console mode RCON with the password from RUST_RCON_PASSWORD."},
	{ID: "palworld", Name: "Palworld", Category: "Survival & Sandbox",
		Image: "thijsvanloef/palworld-server-docker:latest", Ports: []string{"8211/udp"},
		Env: []string{"PORT={PORT0}", "PLAYERS=16", "COMMUNITY=false"}, Volumes: []string{"/palworld"},
		StopTimeout: 60, Icon: "palworld"},
	{ID: "terraria-tshock", Name: "Terraria (TShock)", Category: "Survival & Sandbox",
		Image: "ryshe/terraria:latest", Ports: []string{"7777/tcp"},
		Env: []string{"WORLD_FILENAME=world.wld"}, Volumes: []string{"/root/.local/share/Terraria/Worlds"},
		StopTimeout: 60, Icon: "terraria"},
	{ID: "factorio", Name: "Factorio", Category: "Survival & Sandbox",
		Image: "factoriotools/factorio:stable", Ports: []string{"34197/udp", "27015/tcp"},
		Volumes: []string{"/factorio"}, StopTimeout: 60, Icon: "factorio"},
	{ID: "satisfactory", Name: "Satisfactory", Category: "Survival & Sandbox",
		Image: "wolveix/satisfactory-server:latest", Ports: []string{"7777/udp", "7777/tcp"}, FixedPorts: true,
		Env: []string{"MAXPLAYERS=4"}, Volumes: []string{"/config"}, StopTimeout: 120, Icon: "satisfactory"},
	{ID: "project-zomboid", Name: "Project Zomboid", Category: "Survival & Sandbox",
		Image: "gameservermanagers/gameserver:pz", Ports: []string{"16261/udp", "16262/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "zomboid"},
	{ID: "7dtd", Name: "7 Days to Die", Category: "Survival & Sandbox",
		Image: "vinanrra/7dtd-server:latest", Ports: []string{"26900/udp", "26900/tcp", "26901/udp", "26902/udp"}, FixedPorts: true,
		Env: []string{"START_MODE=1", "VERSION=stable"}, Volumes: []string{"/home/sdtdserver/serverfiles", "/home/sdtdserver/.local/share/7DaysToDie"},
		StopTimeout: 120, Icon: "7dtd"},
	{ID: "dst", Name: "Don't Starve Together", Category: "Survival & Sandbox",
		Image: "jamesits/dst-server:latest", Ports: []string{"10999/udp"},
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "dst"},
	{ID: "vrising", Name: "V Rising", Category: "Survival & Sandbox",
		Image: "trueosiris/vrising:latest", Ports: []string{"9876/udp", "9877/udp"}, FixedPorts: true,
		Env: []string{"TZ=Europe/Vienna", "SERVERNAME=DGSMgt VRising"}, Volumes: []string{"/mnt/vrising/server", "/mnt/vrising/persistentdata"},
		StopTimeout: 120, Icon: "vrising"},
	{ID: "enshrouded", Name: "Enshrouded", Category: "Survival & Sandbox",
		Image: "mornedhels/enshrouded-server:latest", Ports: []string{"15636/udp", "15637/udp"}, FixedPorts: true,
		Env: []string{"SERVER_NAME=DGSMgt Enshrouded"}, Volumes: []string{"/opt/enshrouded"},
		StopTimeout: 120, Icon: "enshrouded"},
	{ID: "starbound", Name: "Starbound (LinuxGSM)", Category: "Survival & Sandbox",
		Image: "gameservermanagers/gameserver:sb", Ports: []string{"21025/tcp"},
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "starbound"},
	{ID: "space-engineers", Name: "Space Engineers", Category: "Survival & Sandbox",
		Image: "devidian/spaceengineers:latest", Ports: []string{"27016/udp"}, FixedPorts: true,
		Volumes: []string{"/appdata/space-engineers"}, StopTimeout: 120, Icon: "spaceengineers"},

	// ---- FPS & Action (mostly LinuxGSM / SteamCMD) ----
	{ID: "cs2", Name: "Counter-Strike 2 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:cs2", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "cs2"},
	{ID: "csgo", Name: "CS:GO (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:csgo", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "csgo"},
	{ID: "css", Name: "CS: Source (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:css", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "css"},
	{ID: "cs16", Name: "CS 1.6 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:cs", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "cs16"},
	{ID: "tf2", Name: "Team Fortress 2 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:tf2", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "tf2"},
	{ID: "l4d2", Name: "Left 4 Dead 2 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:l4d2", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "l4d2"},
	{ID: "gmod", Name: "Garry's Mod (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:gmod", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "gmod"},
	{ID: "arma3", Name: "ARMA 3 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:arma3", Ports: []string{"2302/udp", "2303/udp", "2304/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "arma3"},
	{ID: "dayz", Name: "DayZ (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:dayz", Ports: []string{"2302/udp", "27016/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "dayz"},
	{ID: "insurgency", Name: "Insurgency (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:ins", Ports: []string{"27015/tcp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "insurgency"},
	{ID: "sandstorm", Name: "Insurgency: Sandstorm (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:inss", Ports: []string{"27102/udp", "27131/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "sandstorm"},
	{ID: "mordhau", Name: "Mordhau (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:mh", Ports: []string{"7777/udp", "15000/udp", "27015/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "mordhau"},
	{ID: "quakelive", Name: "Quake Live (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:ql", Ports: []string{"27960/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "quake"},
	{ID: "quake3", Name: "Quake 3 Arena (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:q3", Ports: []string{"27960/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "quake"},
	{ID: "ut99", Name: "Unreal Tournament 99 (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:ut99", Ports: []string{"7777/udp", "7778/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "ut"},
	{ID: "ut", Name: "Unreal Tournament (LinuxGSM)", Category: "FPS & Action",
		Image: "gameservermanagers/gameserver:ut", Ports: []string{"7777/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "ut"},

	// ---- Strategy, Simulation & Open Source ----
	{ID: "assetto-corsa", Name: "Assetto Corsa (LinuxGSM)", Category: "Strategy & Simulation",
		Image: "gameservermanagers/gameserver:ac", Ports: []string{"9600/udp", "9600/tcp", "8081/tcp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "assetto"},
	{ID: "acc", Name: "Assetto Corsa Competizione (LinuxGSM)", Category: "Strategy & Simulation",
		Image: "gameservermanagers/gameserver:acc", Ports: []string{"9231/udp", "9232/udp"}, FixedPorts: true,
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "assetto"},
	{ID: "openttd", Name: "OpenTTD", Category: "Strategy & Simulation",
		Image: "bateau/openttd:latest", Ports: []string{"3979/tcp", "3979/udp"},
		Volumes: []string{"/config"}, StopTimeout: 30, Icon: "openttd"},
	{ID: "mindustry", Name: "Mindustry (LinuxGSM)", Category: "Strategy & Simulation",
		Image: "gameservermanagers/gameserver:mind", Ports: []string{"6567/tcp", "6567/udp"},
		Volumes: []string{"/data"}, StopTimeout: 30, Icon: "mindustry"},
	{ID: "wesnoth", Name: "The Battle for Wesnoth (LinuxGSM)", Category: "Strategy & Simulation",
		Image: "gameservermanagers/gameserver:wmc", Ports: []string{"15000/tcp"},
		Volumes: []string{"/data"}, StopTimeout: 30, Icon: "wesnoth"},
	{ID: "teeworlds", Name: "Teeworlds (LinuxGSM)", Category: "Strategy & Simulation",
		Image: "gameservermanagers/gameserver:tw", Ports: []string{"8303/udp"},
		Volumes: []string{"/data"}, StopTimeout: 30, Icon: "teeworlds"},
	{ID: "ddnet", Name: "DDraceNetwork", Category: "Strategy & Simulation",
		Image: "ich777/ddnetserver:latest", Ports: []string{"8303/udp"},
		Volumes: []string{"/serverdata/serverfiles"}, StopTimeout: 30, Icon: "teeworlds"},
	{ID: "veloren", Name: "Veloren", Category: "Strategy & Simulation",
		Image: "registry.gitlab.com/veloren/veloren/server-cli:nightly", Ports: []string{"14004/tcp"},
		Volumes: []string{"/opt/userdata"}, StopTimeout: 60, Icon: "veloren"},

	// ---- Generic ----
	{ID: "linuxgsm-custom", Name: "Custom LinuxGSM (130+ games)", Category: "Generic",
		Image: "gameservermanagers/gameserver:{GAME}", Ports: []string{},
		Volumes: []string{"/data"}, StopTimeout: 120, Icon: "linuxgsm",
		Notes: "Replace {GAME} in the image tag with any LinuxGSM shortname (e.g. vhserver, rustserver codes: vh, rust, pmc...). See linuxgsm.com/servers for the full list; add the game's ports manually."},
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

	// Port allocation
	ports := []string{}
	hostPorts, err := a.allocatePorts(r, input.NodeID, len(tpl.Ports))
	if err != nil {
		utils.BadRequest(w, err.Error())
		return
	}
	for i, cp := range tpl.Ports {
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
		}
		ports = append(ports, fmt.Sprintf("%d:%s/%s", hp, parts[0], proto))
		hostPorts[i] = hp
	}

	// Env with {PORTn} substitution
	env := append([]string{}, tpl.Env...)
	env = append(env, input.Env...)
	for i, hp := range hostPorts {
		token := fmt.Sprintf("{PORT%d}", i)
		for j := range env {
			env[j] = strings.ReplaceAll(env[j], token, strconv.Itoa(hp))
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
		Name: input.Name, Image: image, Ports: ports, Env: env, Volumes: volumes,
		NodeID: input.NodeID, Icon: tpl.Icon, Folder: tpl.Category,
		StopTimeout: tpl.StopTimeout, StopSignal: tpl.StopSignal,
		RestartPolicy: "unless-stopped", AutoRestart: true,
		HealthCheckType: "docker",
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
