package main

import (
	"context"
	"dgsmgt/internal/api"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/robfig/cron/v3"
	"github.com/rs/cors"
	"go.uber.org/zap"
)

func main() {
	// Initialize structured logging
	logger, _ := zap.NewProduction()
	if os.Getenv("DEBUG") == "true" {
		logger, _ = zap.NewDevelopment()
	}
	defer func() {
		_ = logger.Sync()
	}()

	// Configuration
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "dgsmgt.db"
	}

	dataDir := dataDirFromDSN(dsn)

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret != "" {
		if err := auth.ValidateSecret(jwtSecret); err != nil {
			logger.Fatal("Refusing to start with an insecure JWT_SECRET; unset it to auto-generate one or use at least 32 random bytes", zap.Error(err))
		}
	} else {
		secretFile := os.Getenv("JWT_SECRET_FILE")
		if secretFile == "" {
			secretFile = filepath.Join(dataDir, "jwt_secret")
		}
		secret, err := auth.LoadOrCreateSecret(secretFile)
		if err != nil {
			logger.Fatal("Failed to load or create JWT secret", zap.String("file", secretFile), zap.Error(err))
		}
		jwtSecret = secret
		logger.Info("JWT_SECRET not set, using secret stored on disk", zap.String("file", secretFile))
	}

	adminUser := os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := os.Getenv("ADMIN_PASSWORD")

	var trustedProxies []*net.IPNet
	if os.Getenv("TRUST_PROXY") == "true" {
		cidrs := middleware.DefaultTrustedProxies
		if v := os.Getenv("TRUSTED_PROXIES"); v != "" {
			cidrs = strings.Split(v, ",")
		}
		nets, err := middleware.ParseCIDRs(cidrs)
		if err != nil {
			logger.Fatal("Invalid TRUSTED_PROXIES", zap.Error(err))
		}
		trustedProxies = nets
	}

	allowedOrigins := splitList(os.Getenv("ALLOWED_ORIGINS"))
	for _, o := range allowedOrigins {
		if o == "*" {
			logger.Fatal("ALLOWED_ORIGINS must list explicit origins, '*' is not supported")
		}
	}

	// Initialize Database
	database, err := db.InitDB(dsn)
	if err != nil {
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Auto Migrate new models
	if err := database.AutoMigrate(&models.AuditLog{}); err != nil {
		logger.Fatal("Failed to migrate database", zap.Error(err))
	}

	// Seed Admin User if not exists
	var count int64
	database.Model(&models.User{}).Count(&count)
	if count == 0 {
		generated := false
		if adminPass == "" {
			pass, err := auth.RandomString(18)
			if err != nil {
				logger.Fatal("Failed to generate admin password", zap.Error(err))
			}
			adminPass = pass
			generated = true
		} else if auth.IsWeakPassword(adminPass) {
			logger.Fatal("Refusing to create the admin user with a weak ADMIN_PASSWORD (min. 8 characters, no known defaults)")
		}
		hashedPass, err := auth.HashPassword(adminPass)
		if err != nil {
			logger.Fatal("Failed to hash admin password", zap.Error(err))
		}
		user := models.User{
			Username:           adminUser,
			PasswordHash:       hashedPass,
			IsAdmin:            true,
			MustChangePassword: generated,
		}
		if err := database.Create(&user).Error; err != nil {
			logger.Fatal("Failed to create admin user", zap.Error(err))
		}
		if generated {
			// Shown exactly once; the password must be changed on first login
			logger.Warn("Created initial admin user with a generated password, change it after the first login",
				zap.String("username", adminUser), zap.String("password", adminPass))
		} else {
			logger.Info("Created admin user", zap.String("username", adminUser))
		}
	}

	// Force a password change for admins still using a default password from earlier releases
	var admins []models.User
	database.Where("is_admin = ? AND must_change_password = ?", true, false).Find(&admins)
	for _, u := range admins {
		if auth.IsDefaultPassword(u.PasswordHash) {
			logger.Warn("Admin user still has a default password; a password change is required on next login", zap.String("username", u.Username))
			database.Model(&models.User{}).Where("id = ?", u.ID).Update("must_change_password", true)
		}
	}

	// Initialize Docker Service
	dockerService, err := docker.NewService()
	if err != nil {
		logger.Fatal("Failed to initialize Docker service", zap.Error(err))
	}
	policy := docker.Policy{
		BindRoot:             os.Getenv("ALLOWED_BIND_ROOT"),
		AllowedImagePrefixes: splitList(os.Getenv("ALLOWED_IMAGE_PREFIXES")),
	}
	if policy.BindRoot == "/" {
		logger.Fatal("ALLOWED_BIND_ROOT must not be '/'")
	}
	dockerService.SetPolicy(policy)

	// Initialize API
	apiServer := api.NewAPI(dockerService, database, jwtSecret, allowedOrigins, logger)

	// Initialize Cron Scheduler
	crunner := cron.New()
	var servers []models.Server
	database.Where("cron_schedule != ?", "").Find(&servers)
	for _, s := range servers {
		serverID := s.ContainerID
		_, err := crunner.AddFunc(s.CronSchedule, func() {
			logger.Info("Cron: restarting server", zap.String("id", serverID))
			if err := dockerService.Restart(context.Background(), serverID); err != nil {
				logger.Error("Cron: failed to restart server", zap.String("id", serverID), zap.Error(err))
			}
		})
		if err != nil {
			logger.Error("Cron: failed to add job", zap.String("id", serverID), zap.Error(err))
		}
	}
	crunner.Start()
	defer crunner.Stop()

	// Router setup
	r := mux.NewRouter()

	// Global Middleware
	r.Use(middleware.IPMiddleware(trustedProxies))
	r.Use(middleware.LoggingMiddleware(logger))
	r.Use(middleware.PayloadLimitMiddleware(1 << 20)) // 1MB limit
	r.Use(middleware.RateLimitMiddleware(10, 20))    // 10 RPS, 20 burst
	r.Use(secureHeadersMiddleware)

	// Health Route (unauthenticated)
	r.HandleFunc("/health", apiServer.HealthHandler).Methods("GET")

	// Auth Routes
	// Stricter per-IP limit on top of the global limiter and the failed-login lockout
	loginLimit := middleware.RateLimitMiddleware(0.5, 5)
	r.Handle("/api/login", loginLimit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		apiServer.LoginHandler(w, r, jwtSecret)
	}))).Methods("POST")

	// API Subrouter with Auth
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.AuthMiddleware(jwtSecret, database, apiServer.Tickets()))
	apiRouter.Use(middleware.PasswordChangeMiddleware("/api/me", "/api/me/password", "/api/logout"))

	apiRouter.HandleFunc("/me", apiServer.MeHandler).Methods("GET")
	apiRouter.HandleFunc("/me/password", apiServer.ChangePasswordHandler).Methods("POST")
	apiRouter.HandleFunc("/logout", apiServer.LogoutHandler).Methods("POST")
	apiRouter.HandleFunc("/ws-ticket", apiServer.WSTicketHandler).Methods("POST")
	apiRouter.HandleFunc("/status/{id}", apiServer.StatusHandler).Methods("GET")
	apiRouter.HandleFunc("/action/{id}/{action}", apiServer.ActionHandler).Methods("POST")
	apiRouter.HandleFunc("/logs/{id}", apiServer.LogsHandler)
	apiRouter.HandleFunc("/metrics/{id}", apiServer.MetricsHandler)
	apiRouter.HandleFunc("/my-servers", apiServer.ListMyServersHandler).Methods("GET")

	// Admin Routes
	adminRouter := apiRouter.PathPrefix("/admin").Subrouter()
	adminRouter.Use(middleware.AdminMiddleware)
	
	// User Management
	adminRouter.HandleFunc("/users", apiServer.ListUsersHandler).Methods("GET")
	adminRouter.HandleFunc("/users", apiServer.CreateUserHandler).Methods("POST")
	adminRouter.HandleFunc("/users/{id:[0-9]+}", apiServer.UpdateUserHandler).Methods("PUT")
	adminRouter.HandleFunc("/users/{id:[0-9]+}", apiServer.DeleteUserHandler).Methods("DELETE")

	// Server Management
	adminRouter.HandleFunc("/servers", apiServer.ListServersHandler).Methods("GET")
	adminRouter.HandleFunc("/servers", apiServer.CreateServerHandler).Methods("POST")
	adminRouter.HandleFunc("/servers/{id:[0-9]+}", apiServer.DeleteServerHandler).Methods("DELETE")

	// Assignments
	adminRouter.HandleFunc("/assignments", apiServer.ListAssignmentsHandler).Methods("GET")
	adminRouter.HandleFunc("/assign", apiServer.AssignServerHandler).Methods("POST")
	adminRouter.HandleFunc("/assignments/{userId:[0-9]+}/{serverId:[0-9]+}", apiServer.DeleteAssignmentHandler).Methods("DELETE")

	// Audit Logs
	adminRouter.HandleFunc("/audit-logs", apiServer.ListAuditLogsHandler).Methods("GET")

	// Static files
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static")))

	// 404 Handler
	r.NotFoundHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		http.ServeFile(w, r, "./static/404.html")
	})

	// CORS is only needed when the API is called from other origins; the bundled UI is same-origin
	var handler http.Handler = r
	if len(allowedOrigins) > 0 {
		c := cors.New(cors.Options{
			AllowedOrigins: allowedOrigins,
			AllowedMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		})
		handler = c.Handler(r)
	}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Handler:      handler,
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful Shutdown Logic
	go func() {
		logger.Info("dgsmgt starting", zap.String("addr", ":"+port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("ListenAndServe failed", zap.Error(err))
		}
	}()

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
	logger.Info("Shutting down server...")

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		logger.Fatal("Server forced to shutdown", zap.Error(err))
	}

	logger.Info("Server exiting")
}

// dataDirFromDSN returns the directory holding the SQLite database file.
func dataDirFromDSN(dsn string) string {
	p := strings.TrimPrefix(dsn, "file:")
	if i := strings.IndexByte(p, '?'); i >= 0 {
		p = p[:i]
	}
	return filepath.Dir(p)
}

// splitList splits a comma-separated environment value and drops empty entries.
func splitList(v string) []string {
	var out []string
	for _, s := range strings.Split(v, ",") {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func secureHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-XSS-Protection", "1; mode=block")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:;")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
