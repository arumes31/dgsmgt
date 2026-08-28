#!/bin/bash

# Deployment script for dgsmgt panel.
# Docker must be installed from the operator's trusted distribution/vendor
# repository. This script deliberately never downloads or executes a root
# installer.

set -eu
umask 077

echo "Starting dgsmgt installation..."

if ! command -v docker >/dev/null 2>&1; then
  echo "ERROR: Docker is not installed." >&2
  echo "Install Docker from your signed distribution or vendor repository, then rerun this script." >&2
  exit 1
fi

if ! docker compose version >/dev/null 2>&1; then
  echo "ERROR: Docker Compose V2 is not installed." >&2
  exit 1
fi

if ! command -v openssl >/dev/null 2>&1; then
  echo "ERROR: openssl is required to generate installation-specific secrets." >&2
  exit 1
fi

# Set up the environment file only once. Existing secrets are never replaced.
if [ ! -f .env ]; then
  echo "Setting up .env file..."
  cp .env.example .env
  JWT_SECRET=$(openssl rand -hex 32)
  ADMIN_PASSWORD=$(openssl rand -base64 24 | tr -d '\n')
  sed -i "s/^JWT_SECRET=.*/JWT_SECRET=$JWT_SECRET/" .env
  sed -i "s/^ADMIN_USER=.*/ADMIN_USER=admin/" .env
  sed -i "s|^ADMIN_PASSWORD=.*|ADMIN_PASSWORD=$ADMIN_PASSWORD|" .env
  chmod 600 .env
  echo "Installation-specific credentials were generated in .env (mode 0600)."
fi

echo "Starting dgsmgt panel..."
docker compose up -d --build

echo "----------------------------------------------------"
echo "dgsmgt panel installation complete!"
echo "You can access the panel on port 8080 (default)."
echo "Retrieve the initial credentials from the protected .env file and rotate them after first use."
echo "Docker-group and Docker-socket access are root-equivalent; restrict access to this host."
echo "----------------------------------------------------"
