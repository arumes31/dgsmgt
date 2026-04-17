# DGSMgt (Docker Game Server Management)

A modern, lightweight game server management panel built with Go and React.

## Features

- **Modern UI/UX**: Highly dynamic, glassmorphism-inspired design using Tailwind CSS 4 and Framer Motion.
- **Docker Integration**: Direct integration with Docker Engine API for container management.
- **RBAC (Role-Based Access Control)**:
  - **Admins**: Full control over users, servers, and assignments.
  - **Users**: Access only to assigned servers with specific permissions (Start, Stop, Restart, View Logs).
- **Real-time Monitoring**: Real-time container status and live log streaming via WebSockets.
- **Secure**: JWT-based authentication with bcrypt password hashing.

## Tech Stack

- **Backend**: Go 1.25+
- **Database**: SQLite (via GORM)
- **Frontend**: React 19, Vite 8, Tailwind CSS 4, Framer Motion
- **Container Engine**: Docker Engine API

## Getting Started

### Prerequisites

- Go 1.25+
- Node.js 20+
- Docker Engine (running and accessible via local socket)

### Development Setup

1. **Clone the repository**
2. **Backend**:
   ```bash
   go run cmd/server/main.go
   ```
   Default admin credentials: `admin` / `admin` (change via environment variables `ADMIN_USER` and `ADMIN_PASSWORD`).
3. **Frontend**:
   ```bash
   cd frontend
   npm install
   npm run dev
   ```

### Production Build

1. **Build Frontend**:
   ```bash
   cd frontend
   npm run build
   cd ..
   ```
2. **Copy to static**:
   ```bash
   cp -r frontend/dist/* static/
   ```
3. **Build Backend**:
   ```bash
   go build -o dgsmgt.exe ./cmd/server/main.go
   ```

## Deployment

### Using Docker Compose (GHCR)

You can run the latest pre-built image from the GitHub Container Registry:

1. Create a `docker-compose.yml` file:
   ```yaml
   version: '3.8'
   services:
     dgsmgt:
       image: ghcr.io/your-username/dgsmgt:latest
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

A template is also available in `docker-compose.ghcr.example.yml`.

## Configuration

Environment variables:
- `DATABASE_URL`: Path to SQLite DB (default: `dgsmgt.db`)
- `JWT_SECRET`: Secret for JWT signing
- `ADMIN_USER`: Initial admin username
- `ADMIN_PASSWORD`: Initial admin password
- `TRUST_PROXY`: Set to `true` if behind a reverse proxy

## License

MIT
