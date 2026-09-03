package config

import (
	"fmt"
	"os"
	"strings"
)

type Config struct {
	Env           string
	HTTPAddr      string
	DBDriver      string
	DBDSN         string
	JWTSecret     string
	CORSOrigins   []string
	CORSAllowAll  bool
	GitLabBaseURL string
	GitLabToken   string
	GitLabDryRun  bool
	SeedDemoData  bool

	BootstrapAdminUsername    string
	BootstrapAdminDisplayName string
	BootstrapAdminPassword    string
}

func Load() Config {
	envName := env("APP_ENV", "local")
	dbDriver := env("DB_DRIVER", "sqlite")
	dbDSN := env("DB_DSN", "data/delivery-platform.db")
	if dbDriver == "mysql" && dbDSN == "data/delivery-platform.db" {
		dbDSN = "delivery:delivery@tcp(mysql:3306)/delivery_platform?charset=utf8mb4&parseTime=True&loc=Local"
	}

	corsOrigins := splitEnv("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")

	return Config{
		Env:           envName,
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		DBDriver:      dbDriver,
		DBDSN:         dbDSN,
		JWTSecret:     env("JWT_SECRET", "please-change-this-secret"),
		CORSOrigins:   corsOrigins,
		CORSAllowAll:  contains(corsOrigins, "*"),
		GitLabBaseURL: strings.TrimRight(env("GITLAB_BASE_URL", "https://gitlab.example.com"), "/"),
		GitLabToken:   os.Getenv("GITLAB_TOKEN"),
		GitLabDryRun:  boolEnv("GITLAB_DRY_RUN", true),
		SeedDemoData:  boolEnv("SEED_DEMO_DATA", envName != "production"),

		BootstrapAdminUsername:    env("BOOTSTRAP_ADMIN_USERNAME", "admin"),
		BootstrapAdminDisplayName: env("BOOTSTRAP_ADMIN_DISPLAY_NAME", "系统管理员"),
		BootstrapAdminPassword:    strings.TrimSpace(os.Getenv("BOOTSTRAP_ADMIN_PASSWORD")),
	}
}

func (cfg Config) Validate() error {
	if cfg.Env != "production" {
		return nil
	}
	if strings.TrimSpace(cfg.JWTSecret) == "" || cfg.JWTSecret == "please-change-this-secret" || len(cfg.JWTSecret) < 32 {
		return fmt.Errorf("production requires JWT_SECRET to be a non-default value with at least 32 characters")
	}
	if cfg.CORSAllowAll || len(cfg.CORSOrigins) == 0 {
		return fmt.Errorf("production requires explicit CORS_ORIGINS and does not allow wildcard origins")
	}
	if cfg.DBDriver == "mysql" && (strings.TrimSpace(cfg.DBDSN) == "" || strings.Contains(cfg.DBDSN, "delivery:delivery@")) {
		return fmt.Errorf("production requires a non-default MySQL DB_DSN")
	}
	if strings.TrimSpace(cfg.BootstrapAdminPassword) != "" && IsWeakBootstrapPassword(cfg.BootstrapAdminPassword) {
		return fmt.Errorf("BOOTSTRAP_ADMIN_PASSWORD is too weak for production")
	}
	if !cfg.GitLabDryRun {
		if strings.TrimSpace(cfg.GitLabToken) == "" {
			return fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
		}
		if strings.TrimSpace(cfg.GitLabBaseURL) == "" || cfg.GitLabBaseURL == "https://gitlab.example.com" {
			return fmt.Errorf("GITLAB_BASE_URL must be set to the real GitLab root URL when GITLAB_DRY_RUN=false")
		}
	}
	return nil
}

func IsWeakBootstrapPassword(password string) bool {
	password = strings.TrimSpace(password)
	return len(password) < 12 || password == "admin123" || password == "release123" || password == "dev123"
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitEnv(key, fallback string) []string {
	value := env(key, fallback)
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}

func boolEnv(key string, fallback bool) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	if value == "" {
		return fallback
	}
	return value == "1" || value == "true" || value == "yes" || value == "on"
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
