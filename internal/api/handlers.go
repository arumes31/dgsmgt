package api

import (
	"context"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"dgsmgt/internal/utils"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/gorilla/mux"
	"github.com/gorilla/websocket"
	"github.com/robfig/cron/v3"
	"go.uber.org/zap"
	"gorm.io/gorm"
)

type API struct {
	docker         *docker.Service
	db             *gorm.DB
	jwtSecret      string
	allowedOrigins []string
	upgrader       websocket.Upgrader
	logger         *zap.Logger
	validate       *validator.Validate
	loginLimiter   *loginLimiter
	tickets        *middleware.TicketStore
}

func NewAPI(docker *docker.Service, db *gorm.DB, jwtSecret string, allowedOrigins []string, logger *zap.Logger) *API {
	v := validator.New()
	_ = v.RegisterValidation("cron", validateCron)

	a := &API{
		docker:         docker,
		db:             db,
		jwtSecret:      jwtSecret,
		allowedOrigins: allowedOrigins,
		logger:         logger,
		validate:       v,
		loginLimiter:   newLoginLimiter(),
		tickets:        middleware.NewTicketStore(30 * time.Second),
	}
	a.upgrader = websocket.Upgrader{
		CheckOrigin: a.checkOrigin,
	}
	return a
}

func validateCron(fl validator.FieldLevel) bool {
	schedule := fl.Field().String()
	if schedule == "" {
		return true
	}
	_, err := cron.ParseStandard(schedule)
	return err == nil
}

// Tickets returns the store used to authenticate WebSocket connections.
func (a *API) Tickets() *middleware.TicketStore {
	return a.tickets
}

// checkOrigin accepts same-origin WebSocket requests and explicitly configured origins only.
func (a *API) checkOrigin(r *http.Request) bool {
	origin := r.Header.Get("Origin")
	if origin == "" {
		// Not a browser request; authentication is still required
		return true
	}
	if u, err := url.Parse(origin); err == nil && strings.EqualFold(u.Host, r.Host) {
		return true
	}
	for _, allowed := range a.allowedOrigins {
		if origin == allowed {
			return true
		}
	}
	return false
}

func (a *API) HealthHandler(w http.ResponseWriter, r *http.Request) {
	utils.Success(w, map[string]string{"status": "UP"})
}

func (a *API) MeHandler(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	utils.Success(w, claims)
}

func (a *API) ChangePasswordHandler(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	
	var input struct {
		Password string `json:"password" validate:"required,min=8"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(input); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}
	if auth.IsWeakPassword(input.Password) {
		utils.BadRequest(w, "Password is too weak")
		return
	}

	hashedPass, err := auth.HashPassword(input.Password)
	if err != nil {
		utils.InternalError(w, "Error hashing password")
		return
	}

	var user models.User
	if err := a.db.First(&user, claims.UserID).Error; err != nil {
		utils.NotFound(w, "User not found")
		return
	}
	user.PasswordHash = hashedPass
	user.MustChangePassword = false
	// Revoke all other sessions of this user; the caller gets a fresh token below
	user.TokenVersion++
	if err := a.db.Save(&user).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	token, err := auth.GenerateToken(&user, a.jwtSecret)
	if err != nil {
		utils.InternalError(w, "Failed to generate token")
		return
	}

	a.recordAuditLog(claims, "change_password", nil, "User changed their own password")
	utils.Success(w, map[string]string{"status": "ok", "token": token})
}

// LogoutHandler revokes all tokens of the current user.
func (a *API) LogoutHandler(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	if err := a.db.Model(&models.User{}).Where("id = ?", claims.UserID).
		Update("token_version", gorm.Expr("token_version + 1")).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// WSTicketHandler issues a short-lived, single-use ticket for opening a WebSocket.
func (a *API) WSTicketHandler(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	ticket, err := a.tickets.Issue(claims)
	if err != nil {
		utils.InternalError(w, "Failed to issue ticket")
		return
	}
	utils.Success(w, map[string]string{"ticket": ticket})
}

func (a *API) recordAuditLog(claims *auth.Claims, action string, server *models.Server, details string) {
	log := models.AuditLog{
		UserID:     claims.UserID,
		Username:   claims.Username,
		Action:     action,
		Details:    details,
	}
	if server != nil {
		log.ServerID = server.ID
		log.ServerName = server.Name
	}
	if err := a.db.Create(&log).Error; err != nil {
		a.logger.Error("Failed to record audit log", zap.Error(err))
	}
}

func (a *API) LoginHandler(w http.ResponseWriter, r *http.Request, secret string) {
	var creds struct {
		Username string `json:"username" validate:"required"`
		Password string `json:"password" validate:"required"`
	}
	if err := json.NewDecoder(r.Body).Decode(&creds); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(creds); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}

	ip := middleware.RemoteIP(r)
	if a.loginLimiter.Blocked(ip, creds.Username) {
		a.logger.Warn("Brute force protection triggered", zap.String("ip", ip), zap.String("username", creds.Username))
		utils.Forbidden(w, "Too many login attempts. Try again in 15 minutes.")
		return
	}

	user, err := auth.Authenticate(creds.Username, creds.Password)
	if err != nil {
		a.logger.Warn("Failed login attempt", zap.String("ip", ip), zap.String("username", creds.Username))
		a.loginLimiter.Failure(ip, creds.Username)
		utils.Unauthorized(w, "Invalid credentials")
		return
	}

	// Reset attempts on success
	a.loginLimiter.Success(ip, creds.Username)

	token, err := auth.GenerateToken(user, secret)
	if err != nil {
		a.logger.Error("Token generation failed", zap.Error(err))
		utils.InternalError(w, "Failed to generate token")
		return
	}

	utils.Success(w, map[string]string{
		"token": token,
	})
}

func (a *API) StatusHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	_, _, target, ok := a.authorize(claims, id)
	if !ok {
		utils.Forbidden(w, "Access denied")
		return
	}

	info, err := a.docker.GetStatus(r.Context(), target)
	if err != nil {
		utils.JSON(w, http.StatusNotFound, map[string]string{"status": "not_found"}, err.Error(), nil)
		return
	}
	utils.Success(w, info)
}

func (a *API) ActionHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]
	action := vars["action"]
	ctx := r.Context()

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	server, userServer, target, ok := a.authorize(claims, id)
	if !ok {
		utils.Forbidden(w, "Access denied")
		return
	}
	if server == nil {
		utils.NotFound(w, "Server not found in database")
		return
	}

	// Check specific permissions if not admin
	if !claims.IsAdmin {
		switch action {
		case "start":
			if !userServer.CanStart {
				utils.Forbidden(w, "Permission denied: cannot start")
				return
			}
		case "stop":
			if !userServer.CanStop {
				utils.Forbidden(w, "Permission denied: cannot stop")
				return
			}
		case "restart":
			if !userServer.CanRestart {
				utils.Forbidden(w, "Permission denied: cannot restart")
				return
			}
		}
	}

	var err error
	switch action {
	case "start":
		err = a.docker.Start(ctx, target)
	case "stop":
		err = a.docker.Stop(ctx, target)
	case "restart":
		err = a.docker.Restart(ctx, target)
	default:
		utils.BadRequest(w, "Invalid action")
		return
	}

	if err != nil {
		a.logger.Error("Docker action failed", zap.String("id", id), zap.String("action", action), zap.Error(err))
		utils.InternalError(w, err.Error())
		return
	}

	a.recordAuditLog(claims, action, server, "Triggered container action")
	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) MetricsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	_, _, target, ok := a.authorize(claims, id)
	if !ok {
		utils.Forbidden(w, "Access denied")
		return
	}

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	// Cancel the Docker stream as soon as the client goes away
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	reader, err := a.docker.Stats(ctx, target)
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error reading stats: "+err.Error()))
		return
	}
	defer func() { _ = reader.Close() }()

	go func() {
		defer cancel()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if err := conn.WriteMessage(websocket.TextMessage, buf[:n]); err != nil {
				return
			}
		}
		if err != nil {
			return
		}
	}
}

func (a *API) LogsHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	_, userServer, target, ok := a.authorize(claims, id)
	if !ok {
		utils.Forbidden(w, "Access denied")
		return
	}

	// Check view logs permission if not admin
	if !claims.IsAdmin && !userServer.CanViewLogs {
		utils.Forbidden(w, "Permission denied: cannot view logs")
		return
	}

	conn, err := a.upgrader.Upgrade(w, r, nil)
	if err != nil {
		a.logger.Error("WebSocket upgrade failed", zap.Error(err))
		return
	}
	defer func() { _ = conn.Close() }()

	// Cancel the Docker stream as soon as the client goes away
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	reader, err := a.docker.Logs(ctx, target, "100")
	if err != nil {
		_ = conn.WriteMessage(websocket.TextMessage, []byte("Error reading logs: "+err.Error()))
		return
	}
	defer func() { _ = reader.Close() }()

	go func() {
		defer cancel()
		for {
			_, _, err := conn.ReadMessage()
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 8192)
	for {
		n, err := reader.Read(buf)
		if n > 0 {
			if n > 8 {
				cleanLog := stripDockerHeader(buf[:n])
				if err := conn.WriteMessage(websocket.TextMessage, cleanLog); err != nil {
					return
				}
			}
		}
		if err != nil {
			if err != io.EOF && ctx.Err() == nil {
				a.logger.Warn("Log reader error", zap.Error(err))
			}
			return
		}
	}
}

// findServer looks up a managed server by its full container ID, or by an
// unambiguous hexadecimal ID prefix of at least 12 characters.
func (a *API) findServer(id string) (*models.Server, error) {
	var server models.Server
	if err := a.db.Where("container_id = ?", id).First(&server).Error; err == nil {
		return &server, nil
	}
	if len(id) < 12 || !isHex(id) {
		return nil, gorm.ErrRecordNotFound
	}
	var servers []models.Server
	if err := a.db.Where("container_id LIKE ?", id+"%").Limit(2).Find(&servers).Error; err != nil {
		return nil, err
	}
	if len(servers) != 1 {
		return nil, gorm.ErrRecordNotFound
	}
	return &servers[0], nil
}

func isHex(s string) bool {
	for _, r := range s {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return false
		}
	}
	return true
}

// authorize resolves the container referenced by a request and checks that the
// user may access it. It returns the container ID to pass to Docker, which for
// managed servers is always the stored ID, never the raw request value.
func (a *API) authorize(claims *auth.Claims, id string) (*models.Server, *models.UserServer, string, bool) {
	server, err := a.findServer(id)
	if claims.IsAdmin {
		if err == nil {
			return server, nil, server.ContainerID, true
		}
		// Admins may inspect containers that are not managed by the panel
		return nil, nil, id, true
	}
	if err != nil {
		return nil, nil, "", false
	}

	var userServer models.UserServer
	if err := a.db.Where("user_id = ? AND server_id = ?", claims.UserID, server.ID).First(&userServer).Error; err != nil {
		return nil, nil, "", false
	}

	return server, &userServer, server.ContainerID, true
}

// User Handlers

func (a *API) ListMyServersHandler(w http.ResponseWriter, r *http.Request) {
	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)

	var servers []models.Server
	if claims.IsAdmin {
		if err := a.db.Find(&servers).Error; err != nil {
			utils.InternalError(w, err.Error())
			return
		}
	} else {
		if err := a.db.Joins("JOIN user_servers ON user_servers.server_id = servers.id").
			Where("user_servers.user_id = ?", claims.UserID).
			Find(&servers).Error; err != nil {
			utils.InternalError(w, err.Error())
			return
		}
	}

	type serverWithPerms struct {
		models.Server
		CanStart    bool `json:"can_start"`
		CanStop     bool `json:"can_stop"`
		CanRestart  bool `json:"can_restart"`
		CanViewLogs bool `json:"can_view_logs"`
	}

	var result []serverWithPerms
	for _, s := range servers {
		sp := serverWithPerms{Server: s}
		if !claims.IsAdmin {
			// The config holds env vars (often game server secrets); only admins may see it
			sp.ConfigJSON = ""
		}
		if claims.IsAdmin {
			sp.CanStart = true
			sp.CanStop = true
			sp.CanRestart = true
			sp.CanViewLogs = true
		} else {
			var us models.UserServer
			if err := a.db.Where("user_id = ? AND server_id = ?", claims.UserID, s.ID).First(&us).Error; err == nil {
				sp.CanStart = us.CanStart
				sp.CanStop = us.CanStop
				sp.CanRestart = us.CanRestart
				sp.CanViewLogs = us.CanViewLogs
			}
		}
		result = append(result, sp)
	}

	utils.Success(w, result)
}

// Admin Handlers

func (a *API) ListUsersHandler(w http.ResponseWriter, r *http.Request) {
	var users []models.User
	if err := a.db.Find(&users).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}
	utils.Success(w, users)
}

func (a *API) CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Username string `json:"username" validate:"required,min=3,max=32"`
		Password string `json:"password" validate:"required,min=8"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(input); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}

	hashedPass, err := auth.HashPassword(input.Password)
	if err != nil {
		utils.InternalError(w, "Error hashing password")
		return
	}

	user := models.User{
		Username:     input.Username,
		PasswordHash: hashedPass,
		IsAdmin:      input.IsAdmin,
	}

	if err := a.db.Create(&user).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "create_user", nil, fmt.Sprintf("Created user: %s", user.Username))

	utils.Created(w, user)
}

func (a *API) UpdateUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var user models.User
	if err := a.db.First(&user, id).Error; err != nil {
		utils.NotFound(w, "User not found")
		return
	}

	var input struct {
		Username string `json:"username" validate:"required,min=3,max=32"`
		Password string `json:"password" validate:"omitempty,min=8"`
		IsAdmin  bool   `json:"is_admin"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(input); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}

	if user.IsAdmin && !input.IsAdmin && a.isLastAdmin(&user) {
		utils.BadRequest(w, "Cannot remove admin rights from the last admin")
		return
	}

	// Revoke existing tokens when identity, privileges or password change
	if user.Username != input.Username || user.IsAdmin != input.IsAdmin || input.Password != "" {
		user.TokenVersion++
	}
	user.Username = input.Username
	user.IsAdmin = input.IsAdmin
	if input.Password != "" {
		hashedPass, err := auth.HashPassword(input.Password)
		if err != nil {
			utils.InternalError(w, "Error hashing password")
			return
		}
		user.PasswordHash = hashedPass
	}

	if err := a.db.Save(&user).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "update_user", nil, fmt.Sprintf("Updated user: %s", user.Username))

	utils.Success(w, user)
}

func (a *API) DeleteUserHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var user models.User
	if err := a.db.First(&user, id).Error; err != nil {
		utils.NotFound(w, "User not found")
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	if user.ID == claims.UserID {
		utils.BadRequest(w, "You cannot delete your own account")
		return
	}
	if user.IsAdmin && a.isLastAdmin(&user) {
		utils.BadRequest(w, "Cannot delete the last admin")
		return
	}

	err := a.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", user.ID).Delete(&models.UserServer{}).Error; err != nil {
			return err
		}
		return tx.Delete(&user).Error
	})
	if err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	a.recordAuditLog(claims, "delete_user", nil, fmt.Sprintf("Deleted user: %s", user.Username))

	w.WriteHeader(http.StatusNoContent)
}

// isLastAdmin reports whether user is the only remaining admin.
func (a *API) isLastAdmin(user *models.User) bool {
	var count int64
	if err := a.db.Model(&models.User{}).Where("is_admin = ? AND id <> ?", true, user.ID).Count(&count).Error; err != nil {
		return true
	}
	return count == 0
}

func (a *API) ListServersHandler(w http.ResponseWriter, r *http.Request) {
	var servers []models.Server
	if err := a.db.Find(&servers).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}
	utils.Success(w, servers)
}

func (a *API) CreateServerHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		Name         string   `json:"name" validate:"required,min=3,max=64"`
		Image        string   `json:"image" validate:"required"`
		Ports        []string `json:"ports"`
		Env          []string `json:"env"`
		Volumes      []string `json:"volumes"`
		CronSchedule string   `json:"cron_schedule" validate:"omitempty,cron"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(input); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}

	if err := a.docker.ValidateCreate(input.Image, input.Volumes); err != nil {
		utils.BadRequest(w, err.Error())
		return
	}

	// Deploy in Docker
	containerID, err := a.docker.Create(r.Context(), input.Name, input.Image, input.Ports, input.Env, input.Volumes)
	if err != nil {
		a.logger.Error("Docker creation failed", zap.Error(err))
		utils.InternalError(w, fmt.Sprintf("Docker error: %v", err))
		return
	}

	// Create in DB
	configJSON, _ := json.Marshal(input)
	server := models.Server{
		Name:         input.Name,
		ContainerID:  containerID,
		Image:        input.Image,
		ConfigJSON:   string(configJSON),
		CronSchedule: input.CronSchedule,
	}

	if err := a.db.Create(&server).Error; err != nil {
		_ = a.docker.Delete(r.Context(), containerID, true)
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "create_server", &server, "Created new server and container")

	utils.Created(w, server)
}

func (a *API) DeleteServerHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["id"]

	var server models.Server
	if err := a.db.First(&server, id).Error; err != nil {
		utils.NotFound(w, "Server not found")
		return
	}

	// Delete from Docker
	if server.ContainerID != "" {
		if err := a.docker.Delete(r.Context(), server.ContainerID, true); err != nil {
			a.logger.Warn("Failed to delete container during server deletion", zap.String("id", server.ContainerID), zap.Error(err))
		}
	}

	// Delete from DB
	if err := a.db.Delete(&server).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "delete_server", &server, "Deleted server and container")

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) AssignServerHandler(w http.ResponseWriter, r *http.Request) {
	var input struct {
		UserID      uint `json:"user_id" validate:"required"`
		ServerID    uint `json:"server_id" validate:"required"`
		CanStart    bool `json:"can_start"`
		CanStop     bool `json:"can_stop"`
		CanRestart  bool `json:"can_restart"`
		CanViewLogs bool `json:"can_view_logs"`
	}
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		utils.BadRequest(w, "Invalid request body")
		return
	}

	if err := a.validate.Struct(input); err != nil {
		utils.BadRequest(w, "Validation failed: "+err.Error())
		return
	}

	userServer := models.UserServer{
		UserID:      input.UserID,
		ServerID:    input.ServerID,
		CanStart:    input.CanStart,
		CanStop:     input.CanStop,
		CanRestart:  input.CanRestart,
		CanViewLogs: input.CanViewLogs,
	}

	if err := a.db.Save(&userServer).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "assign_server", nil, fmt.Sprintf("Assigned server %d to user %d", input.ServerID, input.UserID))

	utils.Success(w, map[string]string{"status": "ok"})
}

func (a *API) DeleteAssignmentHandler(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	userID := vars["userId"]
	serverID := vars["serverId"]

	if err := a.db.Where("user_id = ? AND server_id = ?", userID, serverID).Delete(&models.UserServer{}).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}

	claims := r.Context().Value(middleware.ClaimsKey).(*auth.Claims)
	a.recordAuditLog(claims, "delete_assignment", nil, fmt.Sprintf("Removed assignment of server %s from user %s", serverID, userID))

	w.WriteHeader(http.StatusNoContent)
}

func (a *API) ListAuditLogsHandler(w http.ResponseWriter, r *http.Request) {
	var logs []models.AuditLog
	if err := a.db.Order("created_at desc").Limit(100).Find(&logs).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}
	utils.Success(w, logs)
}

func (a *API) ListAssignmentsHandler(w http.ResponseWriter, r *http.Request) {
	var assignments []models.UserServer
	if err := a.db.Find(&assignments).Error; err != nil {
		utils.InternalError(w, err.Error())
		return
	}
	utils.Success(w, assignments)
}

func stripDockerHeader(data []byte) []byte {
	var result []byte
	dataLen := len(data)
	for i := 0; i < dataLen; {
		if i+8 > dataLen {
			result = append(result, data[i:]...)
			break
		}
		header := data[i : i+8]
		b4 := header[4]
		b5 := header[5]
		b6 := header[6]
		b7 := header[7]
		size := int(b4)<<24 | int(b5)<<16 | int(b6)<<8 | int(b7)

		start := i + 8
		end := start + size
		if end > dataLen || end < start {
			result = append(result, data[start:]...)
			break
		}
		result = append(result, data[start:end]...)
		i = end
	}
	if len(result) == 0 && dataLen > 0 {
		return data
	}
	return result
}
