package api

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
	Cmd         []string `json:"cmd"`       // image CMD override (launch flags)
	RconPort    string   `json:"rcon_port"` // container RCON port "25575/tcp": auto-published, password generated ({RCONPW} in Env), console preset to rcon
	Volumes     []string `json:"volumes"`   // container paths
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
		Env:      []string{"EULA=TRUE", "MEMORY=4G", "TYPE=VANILLA", "RCON_PASSWORD={RCONPW}"},
		RconPort: "25575/tcp", Volumes: []string{"/data"},
		StopTimeout: 60, Icon: "minecraft",
		Notes: "Gold standard image. RCON console preconfigured. Set TYPE=PAPER/FORGE/FABRIC and VERSION for modded servers."},
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
		Image: "didstopia/rust-server:latest", Ports: []string{"28015/udp"}, FixedPorts: true,
		Env: []string{"RUST_SERVER_NAME=DGSMgt Rust", "RUST_SERVER_WORLDSIZE=3500",
			"RUST_RCON_PASSWORD={RCONPW}", "RUST_RCON_WEB=0"},
		RconPort: "28016/tcp", Volumes: []string{"/steamcmd/rust"}, StopTimeout: 120, Icon: "rust",
		Notes: "RCON console preconfigured (legacy Source RCON on 28016). First boot downloads the game via SteamCMD — allow 10+ minutes."},
	{ID: "palworld", Name: "Palworld", Category: "Survival & Sandbox",
		Image: "thijsvanloef/palworld-server-docker:latest", Ports: []string{"8211/udp"},
		Env: []string{"PORT={PORT0}", "PLAYERS=16", "COMMUNITY=false",
			"RCON_ENABLED=true", "RCON_PORT=25575", "ADMIN_PASSWORD={RCONPW}"},
		RconPort: "25575/tcp", Volumes: []string{"/palworld"},
		StopTimeout: 60, Icon: "palworld",
		Notes: "RCON console preconfigured (try the 'Info' command)."},
	{ID: "terraria-tshock", Name: "Terraria (TShock)", Category: "Survival & Sandbox",
		Image: "ryshe/terraria:latest", Ports: []string{"7777/tcp"},
		Env: []string{"WORLD_FILENAME=world.wld"},
		// The image requires launch flags: auto-create a medium world on first boot.
		Cmd:         []string{"-world", "/root/.local/share/Terraria/Worlds/world.wld", "-autocreate", "2"},
		Volumes:     []string{"/root/.local/share/Terraria/Worlds"},
		StopTimeout: 60, Icon: "terraria",
		Notes: "Auto-creates a medium world on first boot. TShock uses SQLite: the data volume must be on a native Linux filesystem (Windows drive shares under Docker Desktop are not supported)."},
	{ID: "factorio", Name: "Factorio", Category: "Survival & Sandbox",
		Image: "factoriotools/factorio:stable", Ports: []string{"34197/udp", "27015/tcp"},
		Volumes: []string{"/factorio"}, StopTimeout: 60, Icon: "factorio",
		Notes: "RCON listens on the second port; the image generates the password into config/rconpw on first boot — read it via the file manager and set console mode to RCON."},
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
		Volumes: []string{"/data"}, StopTimeout: 60, Icon: "dst",
		Notes: "Requires a free Klei cluster token (accounts.klei.com): the server restarts until DoNotStarveTogether/Cluster_1/cluster_token.txt exists in the data volume — add it via the file manager."},
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
	{ID: "acc", Name: "Assetto Corsa Competizione (Wine)", Category: "Strategy & Simulation",
		Image: "grimsi/accserver:latest", Ports: []string{"9231/udp", "9232/tcp"}, FixedPorts: true,
		Volumes: []string{"/opt/server"}, StopTimeout: 60, Icon: "assetto",
		Notes: "The ACC dedicated server is Windows-only and Kunos does not allow redistribution: copy your own server files (accServer.exe + cfg/, from Steam) into the volume before starting."},
	{ID: "openttd", Name: "OpenTTD", Category: "Strategy & Simulation",
		Image: "bateau/openttd:latest", Ports: []string{"3979/tcp", "3979/udp"},
		Volumes: []string{"/config"}, StopTimeout: 30, Icon: "openttd"},
	{ID: "mindustry", Name: "Mindustry", Category: "Strategy & Simulation",
		Image: "oldshensheep/mindustry-server:latest", Ports: []string{"6567/tcp", "6567/udp"},
		Volumes: []string{"/opt/mindustry/config"}, StopTimeout: 30, Icon: "mindustry",
		Notes: "Send 'host' in the console to start a map (the server idles until then)."},
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
