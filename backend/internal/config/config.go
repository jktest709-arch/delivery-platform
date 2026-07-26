package config

import (
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
}

func Load() Config {
	dbDriver := env("DB_DRIVER", "sqlite")
	dbDSN := env("DB_DSN", "data/delivery-platform.db")
	if dbDriver == "mysql" && dbDSN == "data/delivery-platform.db" {
		dbDSN = "delivery:delivery@tcp(mysql:3306)/delivery_platform?charset=utf8mb4&parseTime=True&loc=Local"
	}

	corsOrigins := splitEnv("CORS_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173")

	return Config{
		Env:           env("APP_ENV", "local"),
		HTTPAddr:      env("HTTP_ADDR", ":8080"),
		DBDriver:      dbDriver,
		DBDSN:         dbDSN,
		JWTSecret:     env("JWT_SECRET", "please-change-this-secret"),
		CORSOrigins:   corsOrigins,
		CORSAllowAll:  contains(corsOrigins, "*"),
		GitLabBaseURL: strings.TrimRight(env("GITLAB_BASE_URL", "https://gitlab.example.com"), "/"),
		GitLabToken:   os.Getenv("GITLAB_TOKEN"),
		GitLabDryRun:  boolEnv("GITLAB_DRY_RUN", true),
	}
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
