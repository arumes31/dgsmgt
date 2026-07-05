<div align="center">

# 🎮 DGSMgt

### Docker Game Server Management

**A modern, lightweight game server panel — one-click deploys, live consoles, backups and monitoring for 170+ games.**

[![CI](https://github.com/arumes31/dgsmgt/actions/workflows/ci.yml/badge.svg)](https://github.com/arumes31/dgsmgt/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?logo=go&logoColor=white)](go.mod)
[![PostgreSQL](https://img.shields.io/badge/PostgreSQL-17-4169E1?logo=postgresql&logoColor=white)](#-configuration)
[![Docker](https://img.shields.io/badge/Docker-multi--arch-2496ED?logo=docker&logoColor=white)](#-getting-started)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

*Glassmorphism UI · EN/DE · PWA · amd64 + arm64*

</div>

---

## 📖 Table of contents

- [Highlights](#-highlights)
- [Feature tour](#-feature-tour)
- [Game templates](#-game-templates)
- [Getting started](#-getting-started)
- [Configuration](#-configuration)
- [Permissions (RBAC)](#-permissions-rbac)
- [Notifications](#-notifications)
- [Discord login setup](#-discord-login-setup)
- [Architecture](#-architecture)
- [Security notes](#-security-notes)
- [Development & testing](#-development--testing)
- [License](#-license)

---

## ✨ Highlights

| | |
|---|---|
| 🚀 **One-click deploys** | 42 curated templates + the whole LinuxGSM family (130+ games), automatic port allocation, env editing at deploy time |
| 🎛 **Zero-config RCON** | Minecraft, Palworld and Rust deploy with the RCON port published and a generated password wired in — console commands work immediately |
| 🖥 **Live console & logs** | Attach/stdin or Source RCON (multi-packet responses), ANSI colors, regex filters, log alerts, persistence |
| 💾 **Backups** | Manual + scheduled, retention pruning, restore & download — local, **S3** or **SFTP** (host-key pinned) |
| 📈 **Monitoring** | Live CPU/RAM/network over WebSocket, historical charts, availability %, disk usage, crash auto-restart with backoff |
| 🔐 **Serious auth** | Refresh sessions, TOTP 2FA, Discord OAuth, invitations, brute-force lockout, instant token revocation |
| 🧩 **Multi-node** | Manage remote Docker hosts over TCP/TLS from one panel |
| 🗄 **PostgreSQL-only** | Production-grade storage; in-memory SQLite is used only inside the test suite |

---

## 🧭 Feature tour

<details>
<summary><b>🖥 Server management</b></summary>

- Full lifecycle: create, **edit/recreate** (staged swap with automatic rollback), redeploy with live pull progress, clone, rename, adopt existing containers, orphan detection
- Actions: start / stop / restart / **kill / pause / unpause**, bulk actions with `depends_on` ordering, **stacks** (shared-network groups started in dependency order)
- Per-server stop timeout & signal, restart policy, **CPU/memory limits**, network selection, custom command/launch flags, folders & icons
- **Trash with restore** — deleting keeps volumes and config; permanent purge is superadmin-only
- Health checks (Docker healthcheck surfaced), crash detection with **auto-restart + backoff** and crash-loop protection, OOM-kill badges

</details>

<details>
<summary><b>⌨️ Console & logs</b></summary>

- **Interactive console** over WebSocket: container stdin (attach) or **Source RCON** — protocol client handles fragmented multi-packet responses and Rust's console-broadcast quirk
- Command history, saved **snippets**, permission gating per server
- Live logs with stdout/stderr distinction, ANSI colors, regex filter & highlight, tail selection, timestamps, pause-autoscroll, fullscreen, download, optional **log persistence** to disk (size-capped rotation)
- **Log alerts**: regex patterns that trigger notifications

</details>

<details>
<summary><b>📁 Files & backups</b></summary>

- **File manager** per server (permission-gated): browse, edit (5 MB editor), upload, download (tar), extract archives, mkdir/move/delete — works through the Docker API, no host path access needed
- **Backups**: manual + cron-scheduled, retention (also prunes the remote S3/SFTP objects), restore, download; targets **local / S3 / SFTP**

</details>

<details>
<summary><b>📊 Monitoring</b></summary>

- Parsed live metrics (CPU %, memory, network) streamed over WebSocket
- **Historical charts** (samples stored in DB, configurable interval & retention), availability %, disk usage per volume
- Docker event watcher: crash/OOM detection, image update notifier

</details>

<details>
<summary><b>🔐 Auth & security</b></summary>

- Short-lived access tokens (15 min) + **rotating refresh sessions**, device list with per-session revoke
- **Instant revocation**: password change / disable / "sign out everywhere" bumps a token version checked per request (cached, fails open on DB blips)
- **TOTP two-factor auth** (optional), **Discord OAuth login/linking**, invitation links, email password reset
- Brute-force lockout with admin unlock view, reverse-proxy/**Cloudflare-aware client IPs** (`TRUST_PROXY`), last-admin protection, forced password change, disabled accounts
- Superadmin (root) tier for permanent deletes, panel settings and node management
- No insecure defaults: compose **fails fast** without `JWT_SECRET` / `DB_PASSWORD` / `ADMIN_PASSWORD`; a missing admin password generates a one-time random one

</details>

<details>
<summary><b>🧱 Platform</b></summary>

- **PostgreSQL** database, **multi-node** Docker hosts (TCP/TLS), audit log v2 (filters, pagination, CSV export, IP/UA, diffs, retention), diagnostics, update notifier, request IDs, panic recovery, graceful shutdown
- Frontend: no build step, no CDNs — glassmorphism UI with dark/light theme, EN/DE i18n, command palette (Ctrl+K), keyboard shortcuts, toasts, PWA + **Web Push**
- CI: lint, gosec, tests, **multi-arch images (amd64 + arm64)** pushed to GHCR; max 500 lines per source file enforced

</details>

---

## 🕹 Game templates

Every template deploys with automatic host-port allocation from `SERVER_GAME_PORTRANGE`, volumes under `SERVER_DATA_PATH/<name>/`, editable env defaults, and `{PORT0}`/`{RCONPW}` substitution. All templates below were **end-to-end tested** (deploy → stable run → monitoring → console → cleanup).

<details>
<summary><b>🏕 Survival & Sandbox (17)</b></summary>

| Game | Image | RCON |
|---|---|---|
| Minecraft (Java) | `itzg/minecraft-server` | ✅ zero-config |
| Minecraft (Bedrock) | `itzg/minecraft-bedrock-server` | – |
| Valheim | `lloesche/valheim-server` | – |
| ARK: Survival Evolved | LinuxGSM `:ark` | manual |
| ARK: Survival Ascended | `mschnitzer/asa-linux-server` | – |
| Rust | `didstopia/rust-server` | ✅ zero-config |
| Palworld | `thijsvanloef/palworld-server-docker` | ✅ zero-config |
| Terraria (TShock) | `ryshe/terraria` | REST (TShock) |
| Factorio | `factoriotools/factorio` | ✅ password via file manager |
| Satisfactory | `wolveix/satisfactory-server` | – |
| Project Zomboid | LinuxGSM `:pz` | manual |
| 7 Days to Die | `vinanrra/7dtd-server` | telnet |
| Don't Starve Together | `jamesits/dst-server` | – (needs Klei cluster token) |
| V Rising | `trueosiris/vrising` | manual |
| Enshrouded | `mornedhels/enshrouded-server` | – |
| Starbound | LinuxGSM `:sb` | manual |
| Space Engineers | `devidian/spaceengineers` | – |

</details>

<details>
<summary><b>🔫 FPS & Action (16)</b></summary>

| Game | Image |
|---|---|
| Counter-Strike 2 | LinuxGSM `:cs2` |
| CS:GO | LinuxGSM `:csgo` |
| CS: Source | LinuxGSM `:css` |
| CS 1.6 | LinuxGSM `:cs` |
| Team Fortress 2 | LinuxGSM `:tf2` |
| Left 4 Dead 2 | LinuxGSM `:l4d2` |
| Garry's Mod | LinuxGSM `:gmod` |
| ARMA 3 | LinuxGSM `:arma3` |
| DayZ | LinuxGSM `:dayz` |
| Insurgency | LinuxGSM `:ins` |
| Insurgency: Sandstorm | LinuxGSM `:inss` |
| Mordhau | LinuxGSM `:mh` |
| Quake Live | LinuxGSM `:ql` |
| Quake 3 Arena | LinuxGSM `:q3` |
| Unreal Tournament 99 | LinuxGSM `:ut99` |
| Unreal Tournament | LinuxGSM `:ut` |

*Source-engine games get RCON after first install via the game's `server.cfg`.*

</details>

<details>
<summary><b>🏎 Strategy, Simulation & Open Source (8) + Generic</b></summary>

| Game | Image | Note |
|---|---|---|
| Assetto Corsa | LinuxGSM `:ac` | |
| Assetto Corsa Competizione | `grimsi/accserver` (Wine) | bring your own server files (Kunos license) |
| OpenTTD | `bateau/openttd` | tcp+udp share one host port |
| Mindustry | `oldshensheep/mindustry-server` | send `host` in console to start a map |
| Battle for Wesnoth | LinuxGSM `:wmc` | |
| Teeworlds | LinuxGSM `:tw` | |
| DDraceNetwork | `ich777/ddnetserver` | |
| Veloren | GitLab registry `server-cli:nightly` | |
| **Custom LinuxGSM** | `gameservermanagers/gameserver:<code>` | any of 130+ LinuxGSM titles |

</details>

> [!TIP]
> Admins can also add **community template catalogs** (JSON URL) under panel settings, and any container can be created from scratch with the custom wizard (ports, env, volumes, command override, network, limits).

---

## 🚀 Getting started

### Docker Compose (recommended)

```bash
git clone https://github.com/arumes31/dgsmgt && cd dgsmgt
JWT_SECRET=$(openssl rand -hex 32) \
DB_PASSWORD=$(openssl rand -hex 16) \
ADMIN_PASSWORD=pick_a_strong_one \
docker compose up -d --build
```

Open `http://localhost:8080` and log in as `admin` with your `ADMIN_PASSWORD`.

> [!IMPORTANT]
> There are **no insecure defaults**: compose fails fast if `JWT_SECRET`, `DB_PASSWORD` or `ADMIN_PASSWORD` are missing. If the server ever starts without an admin password, it generates a one-time password and prints it once in the logs (with a forced change on first login).

The bundled compose file starts PostgreSQL and the panel, mounts the Docker socket, and adds the `host.docker.internal` host-gateway alias used by zero-config RCON.

### Pre-built image (GHCR)

```yaml
services:
  postgres:
    image: postgres:17-alpine
    environment: [POSTGRES_USER=dgsmgt, POSTGRES_PASSWORD=change_me, POSTGRES_DB=dgsmgt]
    volumes: [pg_data:/var/lib/postgresql/data]
  dgsmgt:
    image: ghcr.io/arumes31/dgsmgt:latest
    ports: ["8080:8080"]
    extra_hosts: ["host.docker.internal:host-gateway"]
    volumes:
      - backups:/app/backups
      - ./serverdata:/app/serverdata
      - /var/run/docker.sock:/var/run/docker.sock
    environment:
      - DATABASE_URL=host=postgres user=dgsmgt password=change_me dbname=dgsmgt sslmode=disable
      - JWT_SECRET=generate_a_long_random_secret
      - ADMIN_PASSWORD=change_me_too
      - BASE_URL=https://panel.example.com   # used for OAuth redirects, invite + reset links
      - SERVER_GAME_PORTRANGE=25000-30000
volumes: { pg_data: {}, backups: {} }
```

> [!NOTE]
> `SERVER_DATA_PATH` must be the **same absolute path** on the host and inside the panel container — game-server volume binds are resolved by the host Docker daemon.

---

## ⚙️ Configuration

All configuration is environment-based.

### Core

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | localhost DSN | **PostgreSQL** DSN (`host=… user=… password=… dbname=…`) |
| `PORT` | `8080` | HTTP listen port |
| `JWT_SECRET` | – | **Required.** Secret for JWT signing |
| `ADMIN_USER` / `ADMIN_PASSWORD` | `admin` / – | Initial superadmin (weak/missing password ⇒ generated one-time password) |
| `BASE_URL` | `http://localhost:8080` | Public URL (emails, invites, OAuth redirects, push) |
| `TRUST_PROXY` | `false` | Honor `CF-Connecting-IP` / `X-Forwarded-For` behind a proxy/Cloudflare |
| `DEBUG` | `false` | Verbose development logging |
| `INSECURE_DEV_MODE` | `false` | Allow missing `JWT_SECRET` / default admin password (**local dev only!**) |

### Game servers

| Variable | Default | Description |
|---|---|---|
| `SERVER_GAME_PORTRANGE` | `25000-30000` | Host port range for template deployments |
| `SERVER_DATA_PATH` | `./serverdata` | Host dir for auto-created game volumes (same path in container!) |
| `RCON_HOST` | `host.docker.internal` | Address the panel dials for auto-configured RCON (set your host IP when running without the compose host-gateway alias) |

### Sessions & auth

| Variable | Default | Description |
|---|---|---|
| `ACCESS_TOKEN_TTL` | `15m` | Access-token lifetime |
| `REFRESH_TOKEN_TTL` | `720h` | Refresh-session lifetime |
| `DISCORD_CLIENT_ID` / `DISCORD_CLIENT_SECRET` / `DISCORD_REDIRECT_URL` | – | Discord OAuth login ([setup](#-discord-login-setup)) |
| `DISCORD_AUTO_CREATE` | `false` | Auto-provision accounts on first Discord login |

### Backups & storage

| Variable | Default | Description |
|---|---|---|
| `BACKUP_PATH` | `./backups` | Local backup storage |
| `LOG_PATH` | `./serverlogs` | Persisted-log storage (50 MB cap per server, rotated) |
| `S3_ENDPOINT` / `S3_ACCESS_KEY` / `S3_SECRET_KEY` / `S3_BUCKET` | – | S3 backup target |
| `S3_USE_SSL` | `true` | Use TLS for the S3 endpoint |
| `SFTP_HOST` / `SFTP_PORT` / `SFTP_USER` / `SFTP_PASSWORD` | – / `22` | SFTP backup target |
| `SFTP_PATH` | `dgsmgt-backups` | Remote directory |
| `SFTP_HOST_KEY` | – | Server public key (`ssh-keyscan -t ed25519 host`); **required** unless skip-verify |
| `SFTP_INSECURE_SKIP_VERIFY` | `false` | Explicitly skip SFTP host-key verification (not recommended) |

### Email (password reset + notifications)

| Variable | Default | Description |
|---|---|---|
| `SMTP_HOST` / `SMTP_PORT` | – / `587` | SMTP relay |
| `SMTP_USER` / `SMTP_PASS` | – | SMTP credentials |
| `SMTP_FROM` | `dgsmgt@localhost` | Sender address |

### Retention & metrics

| Variable | Default | Description |
|---|---|---|
| `AUDIT_RETENTION_DAYS` | `90` | Audit-log retention |
| `METRIC_RETENTION_DAYS` | `14` | Metric-sample retention |
| `METRIC_INTERVAL` | `30s` | Sampling interval for history/availability |

---

## 🛂 Permissions (RBAC)

Eight granular per-server permissions, resolved from **direct assignments merged with group grants** (optionally time-limited), with presets (Viewer / Operator / Owner), bulk assignment and an effective **permission matrix** view:

| Permission | Default | Gates |
|---|---|---|
| `can_start` | ✅ | Start action |
| `can_stop` | ✅ | Stop / kill / pause / unpause |
| `can_restart` | ✅ | Restart action |
| `can_view_logs` | ✅ | Logs, console output, metrics |
| `can_send_commands` | ❌ | Console input, snippets |
| `can_edit_config` | ❌ | Server settings edit/recreate + redeploy |
| `can_access_files` | ❌ | File manager |
| `can_manage_backups` | ❌ | Backups (create/restore/delete) |

Admins manage everything; the **root** (superadmin) tier additionally controls permanent deletes, panel settings and nodes.

---

## 🔔 Notifications

Per-user, per-server preferences with an in-app notification center. Channels:

**Discord webhook** · generic webhooks · **Telegram** · **ntfy** · **Gotify** · SMTP email · **browser Web Push**

Events include crashes/OOM kills, auto-restarts, log-alert matches, backups, and image updates.

---

## 🤝 Discord login setup

1. Create an application at [discord.com/developers](https://discord.com/developers) → OAuth2
2. Add redirect: `https://your-panel/api/oauth/discord/callback`
3. Set `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, `DISCORD_REDIRECT_URL`
4. Users link Discord in their profile, then sign in with Discord (or set `DISCORD_AUTO_CREATE=true`)

---

## 🏗 Architecture

**Backend**: Go 1.25+, gorilla/mux, GORM + PostgreSQL, Docker Engine API · **Frontend**: plain HTML/CSS/ES6+ (no build step, no CDNs) · **Security tooling**: gosec, golangci-lint

```
cmd/server/          entry point, routing, middleware wiring
cmd/seed/            demo-data seeder (requires DATABASE_URL)
internal/api/        HTTP handlers (one group per file), game template catalog
internal/auth/       JWT, sessions, TOTP, token-version revocation
internal/backup/     local/S3/SFTP backup engine
internal/config/     env configuration
internal/db/         Postgres init (SQLite dialector hook for tests only)
internal/docker/     Docker Engine client: lifecycle, exec, stats, files, nodes
internal/middleware/ auth, RBAC tiers, rate limit, request IDs, recovery
internal/monitor/    event watcher, crash auto-restart, metric sampling, log tailers
internal/notify/     notification fan-out (all channels)
internal/rcon/       Source RCON protocol client
internal/scheduler/  cron actions & scheduled backups
static/              frontend (markup-only HTML shells + per-section JS modules)
```

Multi-node: node 0 is the local socket; additional Docker hosts connect over `tcp://` with optional mutual TLS (PEM upload in the UI).

---

## 🛡 Security notes

> [!WARNING]
> Mounting `/var/run/docker.sock` grants the panel **root-equivalent access to the host**. For hardened setups put a [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) in between and point `DOCKER_HOST` at it, or use a dedicated VM/node.

- Raw database/Docker errors are never returned to clients — responses carry a request ID that matches the server log
- Every state-changing action is written to the **audit log** (actor, IP, user agent, diff)
- SFTP backups require **host-key pinning** by default
- Passwords: bcrypt; refresh tokens & reset tokens stored hashed

---

## 🧑‍💻 Development & testing

```bash
# local Postgres
docker run -d -p 5432:5432 -e POSTGRES_USER=dgsmgt -e POSTGRES_PASSWORD=dgsmgt \
  -e POSTGRES_DB=dgsmgt postgres:17-alpine

DATABASE_URL="host=localhost user=dgsmgt password=dgsmgt dbname=dgsmgt sslmode=disable" \
  go run ./cmd/server
```

- **Tests**: `go test ./...` — DB tests run against in-memory SQLite via a dialector override; the runtime is Postgres-only
- **Conventions**: max **500 lines per source file** (`scripts/check-file-size.sh`, enforced in CI); handlers audit every state change; per-server permission checks via the shared access resolver
- The full template catalog is **E2E-tested**: every template is deployed against a live daemon, watched for stable running, checked for metrics/logs/console, then torn down (including RCON round-trips for Minecraft, Factorio, Palworld and Rust)

---

## 📄 License

[MIT](LICENSE)
