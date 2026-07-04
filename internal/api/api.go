package api

import (
	"dgsmgt/internal/auth"
	"dgsmgt/internal/backup"
	"dgsmgt/internal/config"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"dgsmgt/internal/notify"
	"dgsmgt/internal/scheduler"
	"dgsmgt/internal/utils"
	"encoding/json"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/websocket"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type API struct {
	nodes          *docker.Manager
	db             *gorm.DB
	cfg            *config.Config
	jwtSecret      string
	allowedOrigins []string
	upgrader       websocket.Upgrader
	logger         *zap.Logger
	validate       *validator.Validate
	notifier       *notify.Notifier
	backups        *backup.Manager
	scheduler      *scheduler.Scheduler

	loginAttempts sync.Map // ip:username -> *loginAttempt

	updateMu    sync.Mutex
	updateCache *updateInfo
}

// loginAttempt tracks brute-force state. sync.Map only guards the map entry,
// so the fields themselves are protected by mu.
type loginAttempt struct {
	mu        sync.Mutex
	Count     int       `json:"count"`
	LastError time.Time `json:"last_error"`
}

func (l *loginAttempt) snapshot() (int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.Count, l.LastError
}

func NewAPI(nodes *docker.Manager, db *gorm.DB, cfg *config.Config, allowedOrigins []string,
	logger *zap.Logger, notifier *notify.Notifier, backups *backup.Manager, sched *scheduler.Scheduler) *API {
	v := validator.New()
	_ = v.RegisterValidation("cron", validateCron)

	a := &API{
		nodes:          nodes,
		db:             db,
		cfg:            cfg,
		jwtSecret:      cfg.JWTSecret,
		allowedOrigins: allowedOrigins,
		logger:         logger,
		validate:       v,
		notifier:       notifier,
		backups:        backups,
		scheduler:      sched,
	}
	a.upgrader = websocket.Upgrader{CheckOrigin: a.checkOrigin}

	// Evict stale brute-force entries so the map can't grow unbounded.
	go func() {
		for range time.Tick(10 * time.Minute) {
			a.loginAttempts.Range(func(k, v interface{}) bool {
				if att, ok := v.(*loginAttempt); ok {
					if _, last := att.snapshot(); time.Since(last) > 30*time.Minute {
						a.loginAttempts.Delete(k)
					}
				}
				return true
			})
		}
	}()
	return a
}

// docker returns the docker service for a server's node.
func (a *API) dockerFor(server *models.Server) (*docker.Service, error) {
	return a.nodes.For(server.NodeID)
}

func validateCron(fl validator.FieldLevel) bool {
	schedule := fl.Field().String()
	if schedule == "" {
		return true
	}
	_, err := cron.ParseStandard(schedule)
	return err == nil
}

func (a *API) checkOrigin(r *http.Request) bool {
	if len(a.allowedOrigins) == 0 || (len(a.allowedOrigins) == 1 && a.allowedOrigins[0] == "*") {
		return true
	}
	origin := r.Header.Get("Origin")
	for _, allowed := range a.allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func claimsFrom(r *http.Request) *auth.Claims {
	claims, _ := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	return claims
}

// internalError logs the real error and returns a generic message so DB /
// docker internals never leak to clients.
func (a *API) internalError(w http.ResponseWriter, r *http.Request, err error, msg string) {
	a.logger.Error(msg, zap.Error(err),
		zap.String("path", r.URL.Path),
		zap.String("request_id", middleware.RequestID(r)))
	utils.InternalError(w, msg+" (request id: "+middleware.RequestID(r)+")")
}

func (a *API) decodeAndValidate(w http.ResponseWriter, r *http.Request, dst interface{}) bool {
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return false
	}
	if err := a.validate.Struct(dst); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return false
	}
	return true
}

// ---- Audit ------------------------------------------------------------------

type auditOpts struct {
	Server  *models.Server
	Details string
	Success bool
	Diff    interface{}
}

func (a *API) audit(r *http.Request, claims *auth.Claims, action string, o auditOpts) {
	log := models.AuditLog{
		Action:  action,
		Details: o.Details,
		Success: o.Success,
	}
	if claims != nil {
		log.UserID = claims.UserID
		log.Username = claims.Username
	}
	if o.Server != nil {
		log.ServerID = o.Server.ID
		log.ServerName = o.Server.Name
	}
	if r != nil {
		log.IP = middleware.ClientIP(r)
		log.UserAgent = r.UserAgent()
		log.RequestID = middleware.RequestID(r)
	}
	if o.Diff != nil {
		if b, err := json.Marshal(o.Diff); err == nil {
			log.DiffJSON = string(b)
		}
	}
	if err := a.db.Create(&log).Error; err != nil {
		a.logger.Error("Failed to record audit log", zap.Error(err))
	}
}

// ---- Access control -----------------------------------------------------------

// getAccessByServerID resolves effective permissions (direct assignment merged
// with group grants) for a server by DB ID.
func (a *API) getAccessByServerID(claims *auth.Claims, serverID uint) (models.Perms, *models.Server, int) {
	var server models.Server
	if err := a.db.First(&server, serverID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Perms{}, nil, http.StatusNotFound
		}
		return models.Perms{}, nil, http.StatusInternalServerError
	}
	return a.resolvePerms(claims, &server)
}

// getAccess resolves permissions for a server addressed by container ID prefix.
func (a *API) getAccess(claims *auth.Claims, containerID string) (models.Perms, *models.Server, int) {
	var server models.Server
	if err := a.db.Where("container_id LIKE ?", containerID+"%").First(&server).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return models.Perms{}, nil, http.StatusNotFound
		}
		return models.Perms{}, nil, http.StatusInternalServerError
	}
	return a.resolvePerms(claims, &server)
}

func (a *API) resolvePerms(claims *auth.Claims, server *models.Server) (models.Perms, *models.Server, int) {
	if claims.IsAdmin {
		return models.AllPerms(), server, http.StatusOK
	}

	perms := models.Perms{}
	found := false

	var us models.UserServer
	err := a.db.Where("user_id = ? AND server_id = ?", claims.UserID, server.ID).First(&us).Error
	if err == nil {
		if us.ExpiresAt == nil || us.ExpiresAt.After(time.Now()) {
			perms = perms.Merge(us.Perms())
			found = true
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return models.Perms{}, nil, http.StatusInternalServerError
	}

	var groupGrants []models.GroupServer
	if err := a.db.
		Joins("JOIN user_groups ON user_groups.group_id = group_servers.group_id").
		Where("user_groups.user_id = ? AND group_servers.server_id = ?", claims.UserID, server.ID).
		Find(&groupGrants).Error; err != nil {
		// A DB failure must not masquerade as "no access" (false 403).
		return models.Perms{}, nil, http.StatusInternalServerError
	}
	for _, g := range groupGrants {
		perms = perms.Merge(g.Perms())
		found = true
	}

	if !found {
		return models.Perms{}, nil, http.StatusForbidden
	}
	return perms, server, http.StatusOK
}

func writeAccessStatus(w http.ResponseWriter, status int) {
	switch status {
	case http.StatusNotFound:
		utils.NotFound(w, "Server not found")
	case http.StatusForbidden:
		utils.Forbidden(w, "Access denied")
	default:
		utils.InternalError(w, "Failed to check access")
	}
}

// ---- Health / version / diagnostics -------------------------------------------

func (a *API) HealthHandler(w http.ResponseWriter, r *http.Request) {
	status := "UP"
	checks := map[string]string{}

	sqlDB, err := a.db.DB()
	if err == nil {
		err = sqlDB.Ping()
	}
	if err != nil {
		// Log the driver error; never expose connection details to clients.
		a.logger.Error("health: database check failed", zap.Error(err))
		checks["database"] = "down"
		status = "DEGRADED"
	} else {
		checks["database"] = "up"
	}

	if err := a.nodes.Local().Ping(r.Context()); err != nil {
		checks["docker"] = "down"
		status = "DEGRADED"
	} else {
		checks["docker"] = "up"
	}

	code := http.StatusOK
	if status != "UP" {
		code = http.StatusServiceUnavailable
	}
	utils.JSON(w, code, map[string]interface{}{"status": status, "checks": checks}, "", nil)
}

func (a *API) VersionHandler(w http.ResponseWriter, r *http.Request) {
	utils.Success(w, map[string]string{"version": a.cfg.Version})
}

func (a *API) DiagnosticsHandler(w http.ResponseWriter, r *http.Request) {
	var userCount, serverCount, auditCount int64
	a.db.Model(&models.User{}).Count(&userCount)
	a.db.Model(&models.Server{}).Count(&serverCount)
	a.db.Model(&models.AuditLog{}).Count(&auditCount)

	nodes := map[string]string{}
	for _, id := range a.nodes.NodeIDs() {
		svc, err := a.nodes.For(id)
		state := "up"
		if err != nil || svc.Ping(r.Context()) != nil {
			state = "down"
		}
		name := "local"
		if id != 0 {
			var n models.Node
			if a.db.First(&n, id).Error == nil {
				name = n.Name
			}
		}
		nodes[name] = state
	}

	utils.Success(w, map[string]interface{}{
		"version":       a.cfg.Version,
		"users":         userCount,
		"servers":       serverCount,
		"audit_entries": auditCount,
		"nodes":         nodes,
		"cron_jobs":     a.scheduler.EntryCount(),
	})
}

// ---- Update notifier -----------------------------------------------------------

type updateInfo struct {
	Latest    string    `json:"latest"`
	URL       string    `json:"url"`
	CheckedAt time.Time `json:"checked_at"`
}

// UpdateCheckHandler checks GitHub for the latest release (cached 6h).
func (a *API) UpdateCheckHandler(w http.ResponseWriter, r *http.Request) {
	a.updateMu.Lock()
	cached := a.updateCache
	a.updateMu.Unlock()

	if cached == nil || time.Since(cached.CheckedAt) > 6*time.Hour {
		info := &updateInfo{CheckedAt: time.Now()}
		client := http.Client{Timeout: 8 * time.Second}
		resp, err := client.Get("https://api.github.com/repos/arumes31/dgsmgt/releases/latest")
		if err == nil {
			var rel struct {
				TagName string `json:"tag_name"`
				HTMLURL string `json:"html_url"`
			}
			if json.NewDecoder(resp.Body).Decode(&rel) == nil {
				info.Latest = rel.TagName
				info.URL = rel.HTMLURL
			}
			_ = resp.Body.Close()
		}
		a.updateMu.Lock()
		a.updateCache = info
		a.updateMu.Unlock()
		cached = info
	}

	utils.Success(w, map[string]interface{}{
		"current":          a.cfg.Version,
		"latest":           cached.Latest,
		"url":              cached.URL,
		"update_available": cached.Latest != "" && cached.Latest != a.cfg.Version,
	})
}
