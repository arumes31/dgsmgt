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

- **Backend**: Go 1.25+
- **Frontend**: Plain HTML5, CSS3, Vanilla JavaScript (ES6+)
- **Security**: Gosec, golangci-lint, JWT
- **Database**: SQLite (via GORM)
- **Container Engine**: Docker Engine API
- **Deployment**: GitHub Actions, GHCR, Docker

## Getting Started

### Prerequisites

- Go 1.25+
- Docker Engine (running and accessible via local socket)

### Development Setup

1. **Clone the repository**
2. **Run the server**:
   ```bash
   go run cmd/server/main.go
   ```
   Default admin credentials: `admin` / `admin` (change via environment variables `ADMIN_USER` and `ADMIN_PASSWORD`).
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
         - "8080:8080"
       volumes:
         - db_data:/app/data
         - /var/run/docker.sock:/var/run/docker.sock
       environment:
         - DATABASE_URL=/app/data/dgsmgt.db
         - JWT_SECRET=your_secure_secret
         - ADMIN_USER=admin
         - ADMIN_PASSWORD=your_admin_password
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
- `JWT_SECRET`: Secret for JWT signing
- `ADMIN_USER`: Initial admin username
- `ADMIN_PASSWORD`: Initial admin password
- `TRUST_PROXY`: Set to `true` if behind a reverse proxy

## License

MIT
