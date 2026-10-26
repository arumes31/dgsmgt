#!/bin/bash

# Deployment script for dgsmgt panel
# This script installs Docker/Docker Compose and starts the panel

set -e

echo "Starting dgsmgt installation..."

# Check if Docker is installed
if ! [ -x "$(command -v docker)" ]; then
  echo "Installing Docker..."
  curl -fsSL https://get.docker.com -o get-docker.sh
  sh get-docker.sh
  rm get-docker.sh
  # Add current user to docker group
  sudo usermod -aG docker $USER
fi

# Check if Docker Compose is installed
if ! [ -x "$(command -v docker-compose)" ]; then
  echo "Docker Compose not found. Using 'docker compose' (V2)."
  if ! docker compose version >/dev/null 2>&1; then
    echo "ERROR: Docker Compose V2 not found. Please install it."
    exit 1
  fi
fi

# Setup .env file if it doesn't exist
if [ ! -f .env ]; then
  echo "Setting up .env file..."
  cp .env.example .env
  chmod 600 .env
  # Generate a random JWT secret and initial admin password
  SECRET=$(tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 48)
  ADMIN_PASS=$(tr -dc 'a-zA-Z0-9' < /dev/urandom | head -c 20)
  sed -i "s/^JWT_SECRET=.*/JWT_SECRET=$SECRET/" .env
  sed -i "s/^ADMIN_PASSWORD=.*/ADMIN_PASSWORD=$ADMIN_PASS/" .env
  echo "A random JWT_SECRET and ADMIN_PASSWORD have been generated and saved to .env"
fi

# Build and start the panel
echo "Starting dgsmgt panel..."
docker compose up -d --build

echo "----------------------------------------------------"
echo "dgsmgt panel installation complete!"
echo "The panel listens on 127.0.0.1:8080 (default). Put a TLS reverse proxy"
echo "in front of it, or set BIND_ADDR=0.0.0.0 in .env to expose it directly."
echo "Admin credentials: see ADMIN_USER / ADMIN_PASSWORD in .env"
echo "(if ADMIN_PASSWORD is empty, the generated password is printed once in 'docker compose logs')."
echo "----------------------------------------------------"
