# DGSMgt (Docker Game Server Management)

A modern, lightweight game server management panel built with Go and plain HTML/JavaScript.

## Features (v2)

### Server management
- **One-click game templates**: Minecraft (itzg), Valheim, ARK, Rust, Palworld, Terraria, Factorio, Satisfactory, Project Zomboid, 7 Days to Die, V Rising, Enshrouded, CS2/CS:GO/TF2/L4D2/GMod/ARMA3/DayZ and the whole **LinuxGSM** family (130+ games), plus community template URLs.
- **Automatic port allocation** from a configurable range (`SERVER_GAME_PORTRANGE=25000-30000`), with `{PORT0}`-style env substitution and port-conflict detection.
- Full server lifecycle: create, **edit/recreate**, redeploy (pull latest with live progress), clone, rename, adopt existing containers, orphan detection.
- Actions: start / stop / restart / **kill / pause / unpause**, bulk actions with `depends_on` ordering, stacks (shared network multi-container groups).
- Per-server stop timeout & stop signal, restart policy, **CPU/memory limits**, network selection, folders & icons.
- **Trash with restore** — deleting keeps volumes and config; permanent purge is superadmin-only.
- Health checks (Docker healthcheck surfaced), crash detection with **auto-restart + backoff** and crash-loop protection, OOM-kill badges.

### Console & logs
- **Interactive console** (attach/stdin or **Source RCON**, incl. multi-packet responses) with command history, saved snippets and permission gating.
- **Zero-config RCON**: Minecraft, Palworld and Rust templates deploy with the RCON port published and a generated password wired in — console commands work immediately.
- Live logs with stdout/stderr distinction, ANSI colors, regex filter & highlighting, tail selection, timestamps, pause-autoscroll, fullscreen, download, optional **log persistence** to disk.
- **Log alerts**: regex patterns that trigger notifications.

### Files & backups
- **File manager** per server (permission-gated): browse, edit (5 MB editor), upload, download (tar), extract archives, mkdir/move/delete — works through the Docker API, no host path access needed.
- **Backups**: manual + cron-scheduled, retention, restore, download; targets **local / S3 / SFTP**.

### Monitoring
- Parsed live metrics (CPU%, memory, network) over WebSocket, **historical charts** (samples stored in DB), availability %, disk usage per volume.

### Auth & security
- Short-lived access tokens + **rotating refresh sessions**, logout & device list with revoke.
- **TOTP two-factor auth** (optional), **Discord OAuth login/linking**, invitation links, email password reset.
- Brute-force lockout with admin unlock view, reverse-proxy/**Cloudflare-aware client IPs** (`TRUST_PROXY`), last-admin protection, forced password change, disabled accounts.
- Superadmin (root) tier for permanent deletes, panel settings and node management.

### RBAC
- 8 granular per-server permissions (start/stop/restart/logs/**commands/config/files/backups**), presets (Viewer/Operator/Owner), **groups**, bulk assignment, expiring access, effective **permission matrix** view.

### Notifications
- **Discord webhook**, generic webhooks, Telegram, ntfy, Gotify, SMTP email, **browser Web Push**, in-app notification center with per-user/per-server preferences.

### Platform
- **PostgreSQL** database (SQLite removed from the runtime), **multi-node** Docker hosts (TCP/TLS), audit log v2 (filters, pagination, CSV export, IP/UA, diffs, retention), diagnostics, update notifier, request IDs, panic recovery, graceful shutdown.
- Frontend: glassmorphism UI with dark/light theme, EN/DE i18n, command palette (Ctrl+K), keyboard shortcuts, toasts, optimistic actions, PWA + push.
- CI: lint, gosec, tests, **multi-arch images (amd64 + arm64)** pushed to GHCR.

## Tech Stack

- **Backend**: Go 1.25+, gorilla/mux, GORM + PostgreSQL, Docker Engine API
- **Frontend**: Plain HTML5/CSS3/ES6+ (no build step, no CDNs)
- **Security**: JWT + refresh sessions, bcrypt, TOTP, gosec, golangci-lint

## Getting Started

### Docker Compose (recommended)

```bash
git clone https://github.com/arumes31/dgsmgt && cd dgsmgt
JWT_SECRET=$(openssl rand -hex 32) DB_PASSWORD=$(openssl rand -hex 16) ADMIN_PASSWORD=pick_one \
  docker compose up -d --build
```

Compose fails fast if `JWT_SECRET`, `DB_PASSWORD` or `ADMIN_PASSWORD` are missing — there are no insecure defaults. Open `http://localhost:8080` and log in as `admin` with your `ADMIN_PASSWORD` (a forced password change applies to weak defaults; if the server ever starts without an admin password, it generates a one-time password and prints it in the logs).

The bundled compose file starts PostgreSQL and the panel and mounts the Docker socket.

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

## Configuration

| Variable | Default | Description |
|---|---|---|
| `DATABASE_URL` | localhost DSN | **PostgreSQL** DSN (`host=... user=... password=... dbname=...`) |
| `JWT_SECRET` | – | Secret for JWT signing (set it!) |
| `ADMIN_USER` / `ADMIN_PASSWORD` | admin/admin | Initial superadmin |
| `TRUST_PROXY` | false | Honor `CF-Connecting-IP` / `X-Forwarded-For` behind a proxy/Cloudflare |
| `ACCESS_TOKEN_TTL` / `REFRESH_TOKEN_TTL` | 15m / 720h | Session lifetimes |
| `SERVER_GAME_PORTRANGE` | 25000-30000 | Host port range for template deployments |
| `SERVER_DATA_PATH` | ./serverdata | Host dir for auto-created game volumes |
| `RCON_HOST` | host.docker.internal | Address the panel dials for auto-configured RCON (set to your host IP when running without the compose host-gateway alias) |
| `BACKUP_PATH` | ./backups | Local backup storage |
| `DISCORD_CLIENT_ID/SECRET/REDIRECT_URL` | – | Discord OAuth login (`DISCORD_AUTO_CREATE=true` to auto-provision) |
| `SMTP_HOST/PORT/USER/PASS/FROM` | – | Email (password reset + notifications) |
| `S3_ENDPOINT/ACCESS_KEY/SECRET_KEY/BUCKET` | – | S3 backup target |
| `SFTP_HOST/PORT/USER/PASSWORD/PATH` | – | SFTP backup target |
| `SFTP_HOST_KEY` | – | SFTP server public key (`ssh-keyscan -t ed25519 host`); required unless `SFTP_INSECURE_SKIP_VERIFY=true` |
| `INSECURE_DEV_MODE` | false | Allow missing JWT_SECRET / default admin password (local dev only!) |
| `AUDIT_RETENTION_DAYS` / `METRIC_RETENTION_DAYS` | 90 / 14 | Data retention |
| `BASE_URL` | http://localhost:8080 | Public URL (emails, invites, OAuth) |

### Discord login setup
1. Create an application at https://discord.com/developers → OAuth2.
2. Add redirect: `https://your-panel/api/oauth/discord/callback`.
3. Set `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, `DISCORD_REDIRECT_URL`.
4. Users link Discord in their profile, then can sign in with Discord (or set `DISCORD_AUTO_CREATE=true`).

### Security note
Mounting `/var/run/docker.sock` grants the panel root-equivalent access to the host. For hardened setups put a [docker-socket-proxy](https://github.com/Tecnativa/docker-socket-proxy) in between and point `DOCKER_HOST` at it, or use a dedicated VM/node.

## Development

```bash
docker run -d -p 5432:5432 -e POSTGRES_USER=dgsmgt -e POSTGRES_PASSWORD=dgsmgt -e POSTGRES_DB=dgsmgt postgres:17-alpine
DATABASE_URL="host=localhost user=dgsmgt password=dgsmgt dbname=dgsmgt sslmode=disable" go run ./cmd/server
```

Tests: `go test ./...` (DB tests run against in-memory SQLite via a dialector override; the runtime is Postgres-only).

## License

MIT
