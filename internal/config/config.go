package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds all runtime configuration, sourced from environment variables.
type Config struct {
	DatabaseURL string // Postgres DSN
	Port        string
	JWTSecret   string

	AdminUser     string
	AdminPassword string

	TrustProxy bool // honor CF-Connecting-IP / X-Forwarded-For (reverse proxy & Cloudflare support)

	AccessTokenTTL  time.Duration // short-lived access token
	RefreshTokenTTL time.Duration // sliding refresh session lifetime

	// Discord OAuth (optional)
	DiscordClientID     string
	DiscordClientSecret string
	DiscordRedirectURL  string
	DiscordAutoCreate   bool // auto-provision users on first Discord login

	// Storage paths
	BackupPath string
	LogPath    string // persisted container logs

	// Backup remote targets (optional)
	S3Endpoint  string
	S3AccessKey string
	S3SecretKey string
	S3Bucket    string
	S3UseSSL    bool

	SFTPHost     string
	SFTPPort     string
	SFTPUser     string
	SFTPPassword string
	SFTPPath     string
	SFTPHostKey  string // server public key in authorized_keys format (host key pinning)
	SFTPInsecure bool   // explicit opt-out of host key verification

	// SMTP (optional, for email notifications + password reset)
	SMTPHost string
	SMTPPort string
	SMTPUser string
	SMTPPass string
	SMTPFrom string

	// Public base URL of the panel (used in emails / oauth redirects / webpush)
	BaseURL string

	AuditRetentionDays  int
	MetricRetentionDays int
	MetricInterval      time.Duration

	// Game server deployment
	GamePortRange string // e.g. "25000-30000": host ports auto-allocated from this range
	DataPath      string // host directory for auto-created server volumes

	Version string
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getbool(key string) bool { return os.Getenv(key) == "true" }

func getint(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// Version is injected at build time via -ldflags "-X dgsmgt/internal/config.BuildVersion=..."
var BuildVersion = "dev"

func Load() *Config {
	cfg := &Config{
		DatabaseURL:         getenv("DATABASE_URL", "host=localhost user=dgsmgt password=dgsmgt dbname=dgsmgt port=5432 sslmode=disable"),
		Port:                getenv("PORT", "8080"),
		JWTSecret:           getenv("JWT_SECRET", ""),
		AdminUser:           getenv("ADMIN_USER", "admin"),
		AdminPassword:       getenv("ADMIN_PASSWORD", "admin"),
		TrustProxy:          getbool("TRUST_PROXY"),
		AccessTokenTTL:      getdur("ACCESS_TOKEN_TTL", 15*time.Minute),
		RefreshTokenTTL:     getdur("REFRESH_TOKEN_TTL", 30*24*time.Hour),
		DiscordClientID:     os.Getenv("DISCORD_CLIENT_ID"),
		DiscordClientSecret: os.Getenv("DISCORD_CLIENT_SECRET"),
		DiscordRedirectURL:  os.Getenv("DISCORD_REDIRECT_URL"),
		DiscordAutoCreate:   getbool("DISCORD_AUTO_CREATE"),
		BackupPath:          getenv("BACKUP_PATH", "./backups"),
		LogPath:             getenv("LOG_PATH", "./serverlogs"),
		S3Endpoint:          os.Getenv("S3_ENDPOINT"),
		S3AccessKey:         os.Getenv("S3_ACCESS_KEY"),
		S3SecretKey:         os.Getenv("S3_SECRET_KEY"),
		S3Bucket:            os.Getenv("S3_BUCKET"),
		S3UseSSL:            getenv("S3_USE_SSL", "true") == "true",
		SFTPHost:            os.Getenv("SFTP_HOST"),
		SFTPPort:            getenv("SFTP_PORT", "22"),
		SFTPUser:            os.Getenv("SFTP_USER"),
		SFTPPassword:        os.Getenv("SFTP_PASSWORD"),
		SFTPPath:            getenv("SFTP_PATH", "dgsmgt-backups"),
		SFTPHostKey:         os.Getenv("SFTP_HOST_KEY"),
		SFTPInsecure:        getbool("SFTP_INSECURE_SKIP_VERIFY"),
		SMTPHost:            os.Getenv("SMTP_HOST"),
		SMTPPort:            getenv("SMTP_PORT", "587"),
		SMTPUser:            os.Getenv("SMTP_USER"),
		SMTPPass:            os.Getenv("SMTP_PASS"),
		SMTPFrom:            getenv("SMTP_FROM", "dgsmgt@localhost"),
		BaseURL:             getenv("BASE_URL", "http://localhost:"+getenv("PORT", "8080")),
		AuditRetentionDays:  getint("AUDIT_RETENTION_DAYS", 90),
		MetricRetentionDays: getint("METRIC_RETENTION_DAYS", 14),
		MetricInterval:      getdur("METRIC_INTERVAL", 30*time.Second),
		GamePortRange:       getenv("SERVER_GAME_PORTRANGE", "25000-30000"),
		DataPath:            getenv("SERVER_DATA_PATH", "./serverdata"),
		Version:             BuildVersion,
	}
	// Guard against zero/negative intervals (e.g. METRIC_INTERVAL=0s) which
	// would panic time.NewTicker in the monitor.
	if cfg.MetricInterval <= 0 {
		cfg.MetricInterval = 30 * time.Second
	}
	return cfg
}
