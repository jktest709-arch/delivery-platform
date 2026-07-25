package api

import (
	"net/http"

	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/release"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(cfg config.Config, db *gorm.DB, releaseService *release.Service) *gin.Engine {
	if cfg.Env == "production" {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Logger(), gin.Recovery())
	router.Use(cors.New(cors.Config{
		AllowOrigins:     cfg.CORSOrigins,
		AllowMethods:     []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders:     []string{"Authorization", "Content-Type"},
		AllowCredentials: true,
	}))

	handler := Handler{
		cfg:            cfg,
		db:             db,
		releaseService: releaseService,
	}

	router.GET("/healthz", handler.health)

	api := router.Group("/api")
	api.POST("/auth/login", handler.login)

	authenticated := api.Group("")
	authenticated.Use(handler.requireAuth())
	authenticated.GET("/me", handler.me)
	authenticated.GET("/projects", handler.listProjects)
	authenticated.PUT("/projects/:code", handler.updateProject)
	authenticated.GET("/business-lines", handler.listBusinessLines)
	authenticated.PUT("/business-lines/:code", handler.updateBusinessLine)
	authenticated.PUT("/dependencies/:code", handler.updateDependencies)
	authenticated.GET("/releases", handler.listReleases)
	authenticated.POST("/releases", handler.createRelease)
	authenticated.GET("/releases/:id", handler.getRelease)

	operator := authenticated.Group("")
	operator.Use(handler.requireRole("release_manager", "admin"))
	operator.POST("/releases/:id/tag", handler.createTags)
	operator.POST("/releases/:id/package", handler.packageRelease)
	operator.POST("/releases/:id/deploy", handler.deployRelease)
	operator.POST("/releases/:id/projects/:releaseProjectId/package", handler.packageReleaseProject)
	operator.POST("/releases/:id/projects/:releaseProjectId/deploy", handler.deployReleaseProject)

	return router
}
