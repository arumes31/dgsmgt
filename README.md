# DGSMgt (Docker Game Server Management)

A modern, lightweight game server management panel built with Go and plain HTML/JavaScript.

## Features

- **Lightweight UI/UX**: Clean, glassmorphism-inspired design using pure CSS and Vanilla JavaScript.
- **No Dependencies**: No complex frontend build steps, no node_modules, no external CDNs.
- **Docker Integration**: Direct integration with Docker Engine API for container management.
- **RBAC (Role-Based Access Control)**:
  - **Admins**: Full control over users, servers, and assignments.
  - **Users**: Access only to assigned servers with specific permissions (Start, Stop, Restart, View Logs).
- **Real-time Monitoring**: Live log streaming via WebSockets.
- **Secure**: JWT-based authentication with bcrypt password hashing.
- **CI/CD**: Fully automated pipeline with GitHub Actions:
  - **Linting**: Golangci-lint for code quality.
  - **Security**: Gosec for automated security scanning.
  - **Testing**: Automated unit tests on every push.
  - **Auto-deployment**: Automated Docker image builds pushed to GitHub Container Registry (GHCR).

## Tech Stack

- **Backend**: Go 1.26+
- **Frontend**: Plain HTML5, CSS3, Vanilla JavaScript (ES6+)
- **Security**: Gosec, golangci-lint, JWT
- **Database**: SQLite (via GORM)
- **Container Engine**: Docker Engine API
- **Deployment**: GitHub Actions, GHCR, Docker

## Getting Started

### Prerequisites

- Go 1.26+
- Docker Engine (running and accessible via local socket)

### Development Setup

1. **Clone the repository**
2. **Run the server**:
   ```bash
   go run cmd/server/main.go
   ```
   On first start an admin user is created. Set `ADMIN_USER` / `ADMIN_PASSWORD` (min. 8 characters) beforehand, or leave
   `ADMIN_PASSWORD` empty to get a random password that is printed once in the log and must be changed at first login.
   Demo data for development can be created with `go run ./cmd/seed -dev` (writes a random demo-user password to `seed_credentials.txt` next to the database).
3. **Access the portal**:
   Open `http://localhost:8080` in your browser.

### Build

1. **Build Backend**:
   ```bash
   go build -o dgsmgt ./cmd/server/main.go
   ```

## Deployment

### Using Docker Compose (GHCR)

You can run the latest pre-built image from the GitHub Container Registry:

1. Create a `docker-compose.yml` file:
   ```yaml
   version: '3.8'
   services:
     dgsmgt:
       image: ghcr.io/arumes31/dgsmgt:latest
       container_name: dgsmgt
       restart: always
       ports:
         - "127.0.0.1:8080:8080"
       volumes:
         - db_data:/app/data
         - /var/run/docker.sock:/var/run/docker.sock
       environment:
         - DATABASE_URL=/app/data/dgsmgt.db
         - ADMIN_USER=admin
         # optional: JWT_SECRET (>= 32 random chars) and ADMIN_PASSWORD, see Configuration
   volumes:
     db_data:
   ```
2. Run the command:
   ```bash
   docker-compose up -d
   ```

## Configuration

Environment variables:
- `DATABASE_URL`: Path to SQLite DB (default: `dgsmgt.db`)
- `JWT_SECRET`: Secret for JWT signing, at least 32 bytes. Known placeholder values are rejected and the server refuses
  to start. If unset, a random secret is generated and stored in `jwt_secret` next to the database (or `JWT_SECRET_FILE`).
- `ADMIN_USER`: Initial admin username (default: `admin`)
- `ADMIN_PASSWORD`: Initial admin password (min. 8 characters, only used when the database has no users). If unset, a
  random password is generated, printed once in the log, and must be changed at first login.
- `TRUST_PROXY`: Set to `true` if behind a reverse proxy
- `TRUSTED_PROXIES`: Comma-separated CIDRs of proxies whose `X-Forwarded-For` / `CF-Connecting-IP` headers are trusted
  when `TRUST_PROXY=true` (default: loopback and private ranges; add the Cloudflare ranges when using Cloudflare)
- `ALLOWED_ORIGINS`: Comma-separated extra origins allowed for CORS and WebSockets (default: same origin only)
- `ALLOWED_BIND_ROOT`: Host directory under which bind mounts may be used for created containers. Empty (default)
  allows named volumes only; the Docker socket and `/` can never be mounted.
- `ALLOWED_IMAGE_PREFIXES`: Comma-separated image prefixes allowed for created containers, e.g.
  `docker.io/library/,ghcr.io/myorg/` (default: any image)
- `BIND_ADDR` (docker compose only): host address of the published port (default: `127.0.0.1`)

## Security Notes

- Panel admins can create containers through the Docker socket, so treat admin accounts as highly privileged.
- The panel speaks plain HTTP. Run it behind a reverse proxy that terminates TLS (and sets HSTS), e.g. Caddy, Traefik
  or nginx; the compose files only publish the port on `127.0.0.1` by default.
- Admins that still use a default password from earlier releases are forced to change it at their next login.

## License

MIT
