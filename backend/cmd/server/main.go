package main

import (
	"log"

	"delivery-platform/backend/internal/api"
	"delivery-platform/backend/internal/bootstrap"
	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/database"
	"delivery-platform/backend/internal/gitlab"
	"delivery-platform/backend/internal/model"
	"delivery-platform/backend/internal/release"
)

func main() {
	cfg := config.Load()

	db, err := database.Open(cfg)
	if err != nil {
		log.Fatalf("open database: %v", err)
	}

	if err := db.AutoMigrate(
		&model.User{},
		&model.BusinessLine{},
		&model.Project{},
		&model.ProjectDependency{},
		&model.Release{},
		&model.ReleaseProject{},
		&model.ReleaseEvent{},
	); err != nil {
		log.Fatalf("migrate database: %v", err)
	}

	if err := bootstrap.Seed(db); err != nil {
		log.Fatalf("seed database: %v", err)
	}

	gitlabClient := gitlab.NewClient(gitlab.Config{
		BaseURL: cfg.GitLabBaseURL,
		Token:   cfg.GitLabToken,
		DryRun:  cfg.GitLabDryRun,
	})
	releaseService := release.NewService(db, gitlabClient)

	router := api.NewRouter(cfg, db, releaseService)
	if err := router.Run(cfg.HTTPAddr); err != nil {
		log.Fatalf("start http server: %v", err)
	}
}
