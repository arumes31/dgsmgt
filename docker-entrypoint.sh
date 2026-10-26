#!/bin/sh
# Starts the panel as the unprivileged "dgsmgt" user.
# Running as root is only needed here to fix ownership of the data volume
# (older releases created it as root) and to grant access to the Docker socket,
# whose group ID differs between hosts.
set -e

if [ "$(id -u)" = "0" ]; then
    mkdir -p /app/data
    chown -R dgsmgt:dgsmgt /app/data
    chmod 0750 /app/data

    sock="${DOCKER_HOST#unix://}"
    if [ -z "$DOCKER_HOST" ]; then
        sock=/var/run/docker.sock
    fi
    if [ -S "$sock" ]; then
        gid="$(stat -c %g "$sock")"
        group="$(awk -F: -v gid="$gid" '$3 == gid { print $1; exit }' /etc/group)"
        if [ -z "$group" ]; then
            addgroup -g "$gid" -S dockerhost
            group=dockerhost
        fi
        addgroup dgsmgt "$group" 2>/dev/null || true
    fi

    exec su-exec dgsmgt "$@"
fi

exec "$@"
