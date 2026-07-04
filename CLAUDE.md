# DGSMgt — repo conventions

## File size limit (enforced in CI)
No source file (`.go`, `.js`, `.html`, `.css`) may exceed **500 lines** — CI runs
`scripts/check-file-size.sh` and fails the build otherwise. When a file grows,
split it by topic, following the existing patterns:

- `internal/api/`: one handler group per file (`server_*.go`, `admin_*.go`,
  `auth_*.go`, `logs_handlers.go`, `metrics_handlers.go`, ...). Shared helpers
  (access control, audit, responses) live in `api.go`.
- `internal/docker/`: `service.go` (types/status/list), `lifecycle.go`
  (create/start/stop/recreate), `exec.go` (logs/exec/attach), `stats.go`,
  `files.go`, `nodes.go`.
- Frontend: HTML files are markup-only shells; scripts live in `static/js/`
  split per page section (`dashboard*.js`, `admin*.js`, shared core `app.js`).

## Other conventions
- Runtime DB is **PostgreSQL only**. Tests swap the `db.Open` dialector hook to
  in-memory SQLite (`glebarez/sqlite`) — never import sqlite outside `_test.go`.
- Never return raw `err.Error()` from GORM/Docker to HTTP clients; use
  `a.internalError(...)` which logs and returns the request ID.
- Every state-changing handler records an audit entry via `a.audit(...)`.
- Per-server permission checks go through `a.getAccess` / `a.resolvePerms`
  (direct assignments merged with group grants); root-only routes live under
  `/api/admin/root/*`.
- `static/js/auth.js` is a deprecated stub — do not add code there.
- Game templates: `internal/api/templates_catalog.go`-style data belongs in the
  catalog; host ports come from `SERVER_GAME_PORTRANGE` allocation, volumes
  under `SERVER_DATA_PATH` (same absolute path on host and in the panel
  container).
