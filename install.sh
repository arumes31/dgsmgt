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
  # Generate a random JWT secret
  SECRET=$(cat /dev/urandom | tr -dc 'a-zA-Z0-9' | fold -w 32 | head -n 1)
  sed -i "s/replace_me_with_a_long_random_string_in_production/$SECRET/g" .env
  echo "A random JWT_SECRET has been generated and saved to .env"
fi

# Build and start the panel
echo "Starting dgsmgt panel..."
docker compose up -d --build

echo "----------------------------------------------------"
echo "dgsmgt panel installation complete!"
echo "You can access the panel on port 8080 (default)."
echo "Default credentials (if not changed in .env): admin / admin"
echo "----------------------------------------------------"
