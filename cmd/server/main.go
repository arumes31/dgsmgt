package main

import (
	"context"
	"dgsmgt/internal/api"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/backup"
	"dgsmgt/internal/config"
	"dgsmgt/internal/db"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"dgsmgt/internal/monitor"
	"dgsmgt/internal/notify"
	"dgsmgt/internal/scheduler"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

var osExit = os.Exit
var quit = make(chan os.Signal, 1)

func main() {
	if err := Run(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		osExit(1)
	}
}

func Run() error {
	// Initialize structured logging
	logger, _ := zap.NewProduction()
	if os.Getenv("DEBUG") == "true" {
		logger, _ = zap.NewDevelopment()
	}
	defer func() {
		_ = logger.Sync()
	}()

	cfg := config.Load()
	if cfg.JWTSecret == "" {
		cfg.JWTSecret = "default_secret_change_me"
		logger.Warn("Using default JWT secret — set JWT_SECRET in production")
	}

	// Initialize Database (Postgres)
	database, err := db.InitDB(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Seed root admin if no users exist
	var count int64
	database.Model(&models.User{}).Count(&count)
	if count == 0 {
		hashedPass, _ := auth.HashPassword(cfg.AdminPassword)
		user := models.User{
			Username:           cfg.AdminUser,
			PasswordHash:       hashedPass,
			IsAdmin:            true,
			IsRoot:             true,
			MustChangePassword: cfg.AdminPassword == "admin",
		}
		if err := database.Create(&user).Error; err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}
		logger.Info("Created default superadmin user", zap.String("username", cfg.AdminUser))
	}

	// Docker: local service + remote nodes
	localDocker, err := docker.NewService()
	if err != nil {
		return fmt.Errorf("failed to initialize Docker service: %w", err)
	}
	nodes := docker.NewManager(localDocker)
	var dbNodes []models.Node
	database.Where("enabled = ?", true).Find(&dbNodes)
	for _, n := range dbNodes {
		if err := nodes.Register(n); err != nil {
			logger.Warn("Failed to connect node", zap.String("node", n.Name), zap.Error(err))
		}
	}

	// Services
	notifier := notify.New(database, cfg, logger)
	backups := backup.NewManager(database, nodes, cfg, logger)
	sched := scheduler.New(database, nodes, backups, notifier, logger)
	sched.Reload()
	defer sched.Stop()

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()
	mon := monitor.New(database, nodes, cfg, logger, notifier)
	mon.Start(rootCtx)

	apiServer := api.NewAPI(nodes, database, cfg, []string{"*"}, logger, notifier, backups, sched)

	// Router setup
	r := mux.NewRouter()

	// Global Middleware
	r.Use(middleware.IPMiddleware(cfg.TrustProxy))
	r.Use(middleware.RequestIDMiddleware)
	r.Use(middleware.RecoverMiddleware(logger))
	r.Use(middleware.LoggingMiddleware(logger))
	r.Use(middleware.PayloadLimitMiddleware(256 << 20)) // 256MB (file uploads)
	r.Use(middleware.RateLimitMiddleware(20, 60))
	r.Use(secureHeadersMiddleware)

	// Unauthenticated routes
	r.HandleFunc("/health", apiServer.HealthHandler).Methods("GET")
	r.HandleFunc("/api/version", apiServer.VersionHandler).Methods("GET")
	r.HandleFunc("/api/login", apiServer.LoginHandler).Methods("POST")
	r.HandleFunc("/api/login/totp", apiServer.LoginTOTPHandler).Methods("POST")
	r.HandleFunc("/api/refresh", apiServer.RefreshHandler).Methods("POST")
	r.HandleFunc("/api/invitations/accept", apiServer.AcceptInvitationHandler).Methods("POST")
	r.HandleFunc("/api/password-reset/request", apiServer.RequestPasswordResetHandler).Methods("POST")
	r.HandleFunc("/api/password-reset/confirm", apiServer.ConfirmPasswordResetHandler).Methods("POST")
	r.HandleFunc("/api/oauth/discord/login", apiServer.DiscordLoginHandler).Methods("GET")
	r.HandleFunc("/api/oauth/discord/callback", apiServer.DiscordCallbackHandler).Methods("GET")

	// Authenticated API
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.AuthMiddleware(cfg.JWTSecret))

	// Profile & session
	apiRouter.HandleFunc("/me", apiServer.MeHandler).Methods("GET")
	apiRouter.HandleFunc("/me/profile", apiServer.UpdateProfileHandler).Methods("PUT")
	apiRouter.HandleFunc("/me/password", apiServer.ChangePasswordHandler).Methods("POST")
	apiRouter.HandleFunc("/me/sessions", apiServer.ListSessionsHandler).Methods("GET")
	apiRouter.HandleFunc("/me/sessions/{id:[0-9]+}", apiServer.RevokeSessionHandler).Methods("DELETE")
	apiRouter.HandleFunc("/me/sessions", apiServer.RevokeAllSessionsHandler).Methods("DELETE")
	apiRouter.HandleFunc("/me/totp/setup", apiServer.TOTPSetupHandler).Methods("POST")
	apiRouter.HandleFunc("/me/totp/enable", apiServer.TOTPEnableHandler).Methods("POST")
	apiRouter.HandleFunc("/me/totp/disable", apiServer.TOTPDisableHandler).Methods("POST")
	apiRouter.HandleFunc("/me/discord/link", apiServer.DiscordLinkHandler).Methods("POST")
	apiRouter.HandleFunc("/me/discord/link", apiServer.DiscordUnlinkHandler).Methods("DELETE")
	apiRouter.HandleFunc("/logout", apiServer.LogoutHandler).Methods("POST")

	// Notifications
	apiRouter.HandleFunc("/notifications", apiServer.ListNotificationsHandler).Methods("GET")
	apiRouter.HandleFunc("/notifications/read", apiServer.MarkNotificationsReadHandler).Methods("POST")
	apiRouter.HandleFunc("/notification-prefs", apiServer.GetNotificationPrefsHandler).Methods("GET")
	apiRouter.HandleFunc("/notification-prefs", apiServer.SetNotificationPrefHandler).Methods("POST")
	apiRouter.HandleFunc("/push/subscribe", apiServer.PushSubscribeHandler).Methods("POST")
	apiRouter.HandleFunc("/push/unsubscribe", apiServer.PushUnsubscribeHandler).Methods("POST")
	apiRouter.HandleFunc("/settings/public", apiServer.PublicSettingsHandler).Methods("GET")

	// Servers (user-facing, permission-checked per server)
	apiRouter.HandleFunc("/my-servers", apiServer.ListMyServersHandler).Methods("GET")
	apiRouter.HandleFunc("/status/{id}", apiServer.StatusHandler).Methods("GET")
	apiRouter.HandleFunc("/action/{id}/{action}", apiServer.ActionHandler).Methods("POST")
	apiRouter.HandleFunc("/bulk-action", apiServer.BulkActionHandler).Methods("POST")
	apiRouter.HandleFunc("/logs/{id}", apiServer.LogsHandler)
	apiRouter.HandleFunc("/logs/{id}/download", apiServer.DownloadLogsHandler).Methods("GET")
	apiRouter.HandleFunc("/logs/{id}/archive", apiServer.PersistedLogHandler).Methods("GET")
	apiRouter.HandleFunc("/console/{id}", apiServer.ConsoleHandler)
	apiRouter.HandleFunc("/command/{id}", apiServer.CommandHandler).Methods("POST")
	apiRouter.HandleFunc("/metrics/{id}", apiServer.MetricsHandler)
	apiRouter.HandleFunc("/metrics/{id}/history", apiServer.MetricsHistoryHandler).Methods("GET")
	apiRouter.HandleFunc("/metrics/{id}/availability", apiServer.AvailabilityHandler).Methods("GET")
	apiRouter.HandleFunc("/disk/{id}", apiServer.DiskUsageHandler).Methods("GET")
	apiRouter.HandleFunc("/activity/{id}", apiServer.ServerActivityHandler).Methods("GET")

	// Command snippets & log alerts
	apiRouter.HandleFunc("/snippets/{id}", apiServer.ListSnippetsHandler).Methods("GET")
	apiRouter.HandleFunc("/snippets/{id}", apiServer.CreateSnippetHandler).Methods("POST")
	apiRouter.HandleFunc("/snippets/{id}/{snippetId:[0-9]+}", apiServer.DeleteSnippetHandler).Methods("DELETE")
	apiRouter.HandleFunc("/log-alerts/{id}", apiServer.ListLogAlertsHandler).Methods("GET")
	apiRouter.HandleFunc("/log-alerts/{id}", apiServer.CreateLogAlertHandler).Methods("POST")
	apiRouter.HandleFunc("/log-alerts/{id}/{alertId:[0-9]+}", apiServer.DeleteLogAlertHandler).Methods("DELETE")

	// Files (CanAccessFiles-gated per server)
	apiRouter.HandleFunc("/files/{id}/list", apiServer.FileListHandler).Methods("GET")
	apiRouter.HandleFunc("/files/{id}/read", apiServer.FileReadHandler).Methods("GET")
	apiRouter.HandleFunc("/files/{id}/write", apiServer.FileWriteHandler).Methods("POST")
	apiRouter.HandleFunc("/files/{id}/download", apiServer.FileDownloadHandler).Methods("GET")
	apiRouter.HandleFunc("/files/{id}/upload", apiServer.FileUploadHandler).Methods("POST")
	apiRouter.HandleFunc("/files/{id}/extract", apiServer.FileExtractHandler).Methods("POST")
	apiRouter.HandleFunc("/files/{id}/delete", apiServer.FileDeleteHandler).Methods("POST")
	apiRouter.HandleFunc("/files/{id}/mkdir", apiServer.FileMkdirHandler).Methods("POST")
	apiRouter.HandleFunc("/files/{id}/move", apiServer.FileMoveHandler).Methods("POST")

	// Backups (CanManageBackups-gated per server)
	apiRouter.HandleFunc("/backups/{id}", apiServer.ListBackupsHandler).Methods("GET")
	apiRouter.HandleFunc("/backups/{id}", apiServer.CreateBackupHandler).Methods("POST")
	apiRouter.HandleFunc("/backups/{id}/{backupId:[0-9]+}/restore", apiServer.RestoreBackupHandler).Methods("POST")
	apiRouter.HandleFunc("/backups/{id}/{backupId:[0-9]+}/download", apiServer.DownloadBackupHandler).Methods("GET")
	apiRouter.HandleFunc("/backups/{id}/{backupId:[0-9]+}", apiServer.DeleteBackupHandler).Methods("DELETE")

	// Admin Routes
	adminRouter := apiRouter.PathPrefix("/admin").Subrouter()
	adminRouter.Use(middleware.AdminMiddleware)

	// User management
	adminRouter.HandleFunc("/users", apiServer.ListUsersHandler).Methods("GET")
	adminRouter.HandleFunc("/users", apiServer.CreateUserHandler).Methods("POST")
	adminRouter.HandleFunc("/users/import", apiServer.ImportUsersHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id:[0-9]+}", apiServer.UserDetailHandler).Methods("GET")
	adminRouter.HandleFunc("/users/{id:[0-9]+}", apiServer.UpdateUserHandler).Methods("PUT")
	adminRouter.HandleFunc("/users/{id:[0-9]+}", apiServer.DeleteUserHandler).Methods("DELETE")
	adminRouter.HandleFunc("/lockouts", apiServer.ListLockoutsHandler).Methods("GET")
	adminRouter.HandleFunc("/unlock", apiServer.UnlockHandler).Methods("POST")
	adminRouter.HandleFunc("/invitations", apiServer.CreateInvitationHandler).Methods("POST")
	adminRouter.HandleFunc("/invitations", apiServer.ListInvitationsHandler).Methods("GET")

	// Groups
	adminRouter.HandleFunc("/groups", apiServer.ListGroupsHandler).Methods("GET")
	adminRouter.HandleFunc("/groups", apiServer.CreateGroupHandler).Methods("POST")
	adminRouter.HandleFunc("/groups/{id:[0-9]+}", apiServer.DeleteGroupHandler).Methods("DELETE")
	adminRouter.HandleFunc("/groups/{id:[0-9]+}/members", apiServer.GroupMembersHandler).Methods("PUT")
	adminRouter.HandleFunc("/groups/{id:[0-9]+}/servers", apiServer.GroupServersHandler).Methods("POST")
	adminRouter.HandleFunc("/groups/{id:[0-9]+}/servers/{serverId:[0-9]+}", apiServer.DeleteGroupServerHandler).Methods("DELETE")

	// Server management
	adminRouter.HandleFunc("/servers", apiServer.ListServersHandler).Methods("GET")
	adminRouter.HandleFunc("/servers", apiServer.CreateServerHandler).Methods("POST")
	adminRouter.HandleFunc("/servers/{id:[0-9]+}", apiServer.UpdateServerHandler).Methods("PUT")
	adminRouter.HandleFunc("/servers/{id:[0-9]+}", apiServer.DeleteServerHandler).Methods("DELETE")
	adminRouter.HandleFunc("/servers/{id:[0-9]+}/clone", apiServer.CloneServerHandler).Methods("POST")
	adminRouter.HandleFunc("/servers/{id:[0-9]+}/redeploy", apiServer.RedeployHandler)
	adminRouter.HandleFunc("/servers/from-template", apiServer.DeployTemplateHandler).Methods("POST")
	adminRouter.HandleFunc("/templates", apiServer.ListTemplatesHandler).Methods("GET")
	adminRouter.HandleFunc("/allocate-ports", apiServer.AllocatePortsHandler).Methods("GET")
	adminRouter.HandleFunc("/port-check", apiServer.PortCheckHandler).Methods("POST")
	adminRouter.HandleFunc("/networks", apiServer.NetworksHandler).Methods("GET")
	adminRouter.HandleFunc("/image-tags", apiServer.ImageTagsHandler).Methods("GET")
	adminRouter.HandleFunc("/orphans", apiServer.ListOrphansHandler).Methods("GET")
	adminRouter.HandleFunc("/adopt", apiServer.AdoptContainerHandler).Methods("POST")
	adminRouter.HandleFunc("/trash", apiServer.ListTrashHandler).Methods("GET")
	adminRouter.HandleFunc("/trash/{id:[0-9]+}/restore", apiServer.RestoreServerHandler).Methods("POST")

	// Stacks
	adminRouter.HandleFunc("/stacks", apiServer.ListStacksHandler).Methods("GET")
	adminRouter.HandleFunc("/stacks", apiServer.CreateStackHandler).Methods("POST")
	adminRouter.HandleFunc("/stacks/{id:[0-9]+}", apiServer.DeleteStackHandler).Methods("DELETE")
	adminRouter.HandleFunc("/stacks/{id:[0-9]+}/{action}", apiServer.StackActionHandler).Methods("POST")

	// Assignments
	adminRouter.HandleFunc("/assignments", apiServer.ListAssignmentsHandler).Methods("GET")
	adminRouter.HandleFunc("/assign", apiServer.AssignServerHandler).Methods("POST")
	adminRouter.HandleFunc("/bulk-assign", apiServer.BulkAssignHandler).Methods("POST")
	adminRouter.HandleFunc("/assignments/{userId:[0-9]+}/{serverId:[0-9]+}", apiServer.DeleteAssignmentHandler).Methods("DELETE")
	adminRouter.HandleFunc("/permission-matrix", apiServer.PermissionMatrixHandler).Methods("GET")

	// Audit, diagnostics, updates
	adminRouter.HandleFunc("/audit-logs", apiServer.ListAuditLogsHandler).Methods("GET")
	adminRouter.HandleFunc("/diagnostics", apiServer.DiagnosticsHandler).Methods("GET")
	adminRouter.HandleFunc("/update-check", apiServer.UpdateCheckHandler).Methods("GET")

	// Root-only (superadmin): permanent deletes, panel settings, nodes
	rootRouter := adminRouter.PathPrefix("/root").Subrouter()
	rootRouter.Use(middleware.RootMiddleware)
	rootRouter.HandleFunc("/trash/{id:[0-9]+}", apiServer.PurgeServerHandler).Methods("DELETE")
	rootRouter.HandleFunc("/settings", apiServer.GetSettingsHandler).Methods("GET")
	rootRouter.HandleFunc("/settings", apiServer.SetSettingsHandler).Methods("PUT")
	rootRouter.HandleFunc("/nodes", apiServer.ListNodesHandler).Methods("GET")
	rootRouter.HandleFunc("/nodes", apiServer.CreateNodeHandler).Methods("POST")
	rootRouter.HandleFunc("/nodes/{id:[0-9]+}", apiServer.DeleteNodeHandler).Methods("DELETE")

	// Static files
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static")))

	// 404 Handler
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		http.ServeFile(w, r, "./static/404.html")
	})

	// CORS Setup
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Adjust in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":" + cfg.Port,
		WriteTimeout: 15 * time.Minute, // long-running downloads/uploads
		ReadTimeout:  15 * time.Minute,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful Shutdown Logic
	serverError := make(chan error, 1)
	go func() {
		logger.Info("dgsmgt starting", zap.String("addr", ":"+cfg.Port), zap.String("version", cfg.Version))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	if os.Getenv("TEST_SERVER_ERROR") == "true" {
		serverError <- fmt.Errorf("forced server error")
	}

	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)

	testMode := os.Getenv("TEST_MODE") == "true"
	var testTimeout <-chan time.Time
	if testMode {
		testTimeout = time.After(2 * time.Second)
	}

	select {
	case err := <-serverError:
		return fmt.Errorf("listenandserve failed: %w", err)
	case <-quit:
		logger.Info("Shutting down server...")
	case <-testTimeout:
		logger.Info("Test mode: shutting down after 2 seconds")
	}

	rootCancel()

	timeout := 10 * time.Second
	if os.Getenv("TEST_SHUTDOWN_ERROR") == "true" {
		timeout = 0
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		return fmt.Errorf("server forced to shutdown: %w", err)
	}

	logger.Info("Server exiting")
	return nil
}

func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data: https://cdn.discordapp.com; font-src 'self'; connect-src 'self' ws: wss:;")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
