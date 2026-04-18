package main

import (
	"context"
	"dgsmgt/internal/api"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gorilla/mux"
	"github.com/robfig/cron/v3"
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

	// Configuration
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "dgsmgt.db"
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		jwtSecret = "default_secret_change_me"
		logger.Warn("Using default JWT secret")
	}

	adminUser := os.Getenv("ADMIN_USER")
	if adminUser == "" {
		adminUser = "admin"
	}
	adminPass := os.Getenv("ADMIN_PASSWORD")
	if adminPass == "" {
		adminPass = "admin"
	}

	trustProxy := os.Getenv("TRUST_PROXY") == "true"

	// Initialize Database
	database, err := db.InitDB(dsn)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}

	// Auto Migrate new models
	if err := database.AutoMigrate(&models.AuditLog{}); err != nil {
		return fmt.Errorf("failed to migrate database: %w", err)
	}

	// Seed Admin User if not exists
	var count int64
	database.Model(&models.User{}).Count(&count)
	if count == 0 {
		hashedPass, _ := auth.HashPassword(adminPass)
		user := models.User{
			Username:     adminUser,
			PasswordHash: hashedPass,
			IsAdmin:      true,
		}
		if err := database.Create(&user).Error; err != nil {
			return fmt.Errorf("failed to create admin user: %w", err)
		}
		logger.Info("Created default admin user", zap.String("username", adminUser))
	}

	// Initialize Docker Service
	dockerService, err := docker.NewService()
	if err != nil {
		return fmt.Errorf("failed to initialize Docker service: %w", err)
	}

	// Initialize API
	apiServer := api.NewAPI(dockerService, database, jwtSecret, []string{"*"}, logger)

	// Initialize Cron Scheduler
	crunner := cron.New()
	var servers []models.Server
	database.Where("cron_schedule != ?", "").Find(&servers)
	for _, s := range servers {
		serverID := s.ContainerID
		schedule := s.CronSchedule
		_, err := crunner.AddFunc(schedule, func() {
			logger.Info("Cron: restarting server", zap.String("id", serverID))
			if err := dockerService.Restart(context.Background(), serverID); err != nil {
				logger.Error("Cron: failed to restart server", zap.String("id", serverID), zap.Error(err))
			}
		})
		if err != nil {
			logger.Error("Cron: failed to add job", zap.String("id", serverID), zap.Error(err))
		}
	}
	
	if os.Getenv("TEST_TRIGGER_CRON") == "true" {
		for _, e := range crunner.Entries() {
			e.Job.Run()
		}
	}
	crunner.Start()
	defer crunner.Stop()

	// Router setup
	r := mux.NewRouter()

	// Global Middleware
	r.Use(middleware.IPMiddleware(trustProxy))
	r.Use(middleware.LoggingMiddleware(logger))
	r.Use(middleware.PayloadLimitMiddleware(1 << 20)) // 1MB limit
	r.Use(middleware.RateLimitMiddleware(10, 20))    // 10 RPS, 20 burst
	r.Use(secureHeadersMiddleware)

	// Health Route (unauthenticated)
	r.HandleFunc("/health", apiServer.HealthHandler).Methods("GET")

	// Auth Routes
	r.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		apiServer.LoginHandler(w, r, jwtSecret)
	}).Methods("POST")

	// API Subrouter with Auth
	apiRouter := r.PathPrefix("/api").Subrouter()
	apiRouter.Use(middleware.AuthMiddleware(jwtSecret))

	apiRouter.HandleFunc("/me", apiServer.MeHandler).Methods("GET")
	apiRouter.HandleFunc("/me/password", apiServer.ChangePasswordHandler).Methods("POST")
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

	// CORS Setup
	c := cors.New(cors.Options{
		AllowedOrigins:   []string{"*"}, // Adjust in production
		AllowedMethods:   []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	srv := &http.Server{
		Handler:      c.Handler(r),
		Addr:         ":" + port,
		WriteTimeout: 15 * time.Second,
		ReadTimeout:  15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Graceful Shutdown Logic
	serverError := make(chan error, 1)
	go func() {
		logger.Info("dgsmgt starting", zap.String("addr", ":"+port))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			serverError <- err
		}
	}()

	if os.Getenv("TEST_SERVER_ERROR") == "true" {
		serverError <- fmt.Errorf("forced server error")
	}

	// Wait for interrupt signal to gracefully shutdown the server with a timeout of 10 seconds.
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	
	select {
	case err := <-serverError:
		return fmt.Errorf("listenandserve failed: %w", err)
	case <-quit:
		logger.Info("Shutting down server...")
	case <-time.After(2 * time.Second):
		if os.Getenv("TEST_MODE") == "true" {
			logger.Info("Test mode: shutting down after 2 seconds")
		}
	}

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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self' 'unsafe-inline'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self'; connect-src 'self' ws: wss:;")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")
		next.ServeHTTP(w, r)
	})
}
