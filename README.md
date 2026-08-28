# DGSMgt

DGSMgt is a lightweight Docker game-server management panel written in Go.
It provides role-based access, container lifecycle controls, logs, metrics,
scheduled restarts, and an HTML/JavaScript frontend.

## Security model

DGSMgt deliberately has no default JWT key or administrator password. A new
database is initialized only when `ADMIN_USER` and a password of at least 16
characters are supplied. `JWT_SECRET` is always required and must contain at
least 32 bytes. Remove the bootstrap credentials from the environment after
the first administrator is created.

The public web process does not receive `/var/run/docker.sock`. A separate,
non-networked helper exposes only DGSMgt's high-level operations through a
shared Unix socket. The helper:

- manages only containers carrying `com.dgsmgt.managed=true`;
- creates only exact image references listed in `ALLOWED_IMAGES`;
- requires every allowed image to use a `sha256` digest;
- rejects privileged host ports below 1024;
- permits only exact bind sources listed in `ALLOWED_VOLUME_ROOTS` (children
  must be listed separately, preventing symlink-based allowlist escapes);
- rejects root, relative, traversal, and unsafe bind-propagation mappings.

This makes existing unlabeled containers inaccessible after upgrading. Back
up their data, recreate them through the secured panel, and verify the label
before removing the old containers. Do not manually add the label to an
untrusted container.

## Requirements

- Go 1.27 for local development
- Docker Engine and Docker Compose V2 for deployment
- OpenSSL when using `install.sh`

Install Docker from a signed distribution or vendor repository. The installer
does not download or execute remote root scripts. Docker group and socket
access are root-equivalent and must be restricted to trusted operators.

## Quick start

1. Copy `.env.example` to `.env` and set:

   - `JWT_SECRET` to a random value of at least 32 bytes;
   - `ADMIN_USER` and `ADMIN_PASSWORD` for a new database;
   - `ALLOWED_IMAGES` to comma-separated digest references such as
     `registry.example/game@sha256:<64 lowercase hex characters>`;
   - `ALLOWED_VOLUME_ROOTS` to the exact dedicated host data directories that
     containers may bind, such as `/srv/dgsmgt`—never `/`.

2. Protect the file and start the deployment:

   ```sh
   chmod 600 .env
   docker compose up -d --build
   ```

Alternatively, `./install.sh` creates installation-specific JWT and bootstrap
credentials in `.env` without printing them.

3. Place the management endpoint behind HTTPS and authenticated network
   access. Leave `TRUST_PROXY=false` unless direct access to the listener is
   blocked and the immediate proxy is trusted. When enabled, set
   `TRUSTED_PROXY_CIDRS` to the exact proxy networks; forwarded addresses from
   any other peer are ignored. Browser access is same-origin by default. Set
   `ALLOWED_ORIGINS` to exact `https://host` origins only when cross-origin
   access is required; wildcards are rejected.

4. Sign in, change the initial password, remove `ADMIN_USER` and
   `ADMIN_PASSWORD` from `.env`, and redeploy the web service.

## GHCR deployment

`docker-compose.ghcr.example.yml` requires `DGSMGT_IMAGE` to be a complete
digest-pinned reference. Resolve the digest produced by a reviewed release and
set, for example:

```sh
DGSMGT_IMAGE=ghcr.io/arumes31/dgsmgt@sha256:<digest>
docker compose -f docker-compose.ghcr.example.yml up -d
```

Do not deploy the mutable `latest` tag.

## Development

Run the complete local checks:

```sh
go mod tidy
git diff --exit-code -- go.mod go.sum
go test -race ./...
go vet ./...
go run golang.org/x/vuln/cmd/govulncheck@v1.7.0 ./...
```

The web server expects the helper socket at
`/run/dgsmgt/docker-proxy.sock`. For an end-to-end development environment,
use Docker Compose rather than mounting the Docker socket into the web process.

## Operations

- Rotate `JWT_SECRET` to invalidate every issued token after suspected
  exposure.
- Treat any deployment that used the former public defaults as compromised;
  rotate administrator credentials and inspect Docker daemon events, images,
  containers, and mounts.
- Review and update allowed image digests through pull requests.
- Keep the panel private even with application authentication enabled.

See [SECURITY.md](SECURITY.md) for private vulnerability reporting.

## License

MIT
