package main

import (
	"context"
	"dgsmgt/internal/api"
	"dgsmgt/internal/auth"
	"dgsmgt/internal/db"
	"dgsmgt/internal/docker"
	"dgsmgt/internal/middleware"
	"dgsmgt/internal/models"
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

func main() {
	// Initialize structured logging
	logger, _ := zap.NewProduction()
	if os.Getenv("DEBUG") == "true" {
		logger, _ = zap.NewDevelopment()
	}
	defer logger.Sync()

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
		logger.Fatal("Failed to initialize database", zap.Error(err))
	}

	// Auto Migrate new models
	database.AutoMigrate(&models.AuditLog{})

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
		database.Create(&user)
		logger.Info("Created default admin user", zap.String("username", adminUser))
	}

	// Initialize Docker Service
	dockerService, err := docker.NewService()
	if err != nil {
		logger.Fatal("Failed to initialize Docker service", zap.Error(err))
	}

	// Initialize API
	apiServer := api.NewAPI(dockerService, database, jwtSecret, []string{"*"}, logger)

	// Initialize Cron Scheduler
	crunner := cron.New()
	var servers []models.Server
	database.Where("cron_schedule != ?", "").Find(&servers)
	for _, s := range servers {
		serverID := s.ContainerID
		crunner.AddFunc(s.CronSchedule, func() {
			logger.Info("Cron: restarting server", zap.String("id", serverID))
			if err := dockerService.Restart(context.Background(), serverID); err != nil {
				logger.Error("Cron: failed to restart server", zap.String("id", serverID), zap.Error(err))
			}
		})
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

	// Assignment
	adminRouter.HandleFunc("/assign", apiServer.AssignServerHandler).Methods("POST")

	// Static files for SPA
	r.PathPrefix("/").Handler(http.FileServer(http.Dir("./static")))

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
