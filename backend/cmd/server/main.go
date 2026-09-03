package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"delivery-platform/backend/internal/api"
	"delivery-platform/backend/internal/bootstrap"
	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/database"
	"delivery-platform/backend/internal/gitlab"
	"delivery-platform/backend/internal/release"
)

func main() {
	cfg := config.Load()
	if err := cfg.Validate(); err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := database.Migrate(db); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	if err := bootstrap.Seed(db, cfg); err != nil {
		log.Fatalf("seed database: %v", err)
	}

	gitlabClient := gitlab.NewClient(gitlab.Config{
		BaseURL: cfg.GitLabBaseURL,
		Token:   cfg.GitLabToken,
		DryRun:  cfg.GitLabDryRun,
	})
	releaseService := release.NewService(db, gitlabClient)

	router := api.NewRouter(cfg, db, releaseService)
	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           router,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	shutdownSignal, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-shutdownSignal.Done()
		shutdownContext, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownContext); err != nil {
			log.Printf("graceful shutdown failed: %v", err)
		}
	}()

	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("start http server: %v", err)
	}
}
