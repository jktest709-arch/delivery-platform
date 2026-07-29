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
	corsConfig := cors.Config{
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}
	if cfg.CORSAllowAll {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = cfg.CORSOrigins
	}
	router.Use(cors.New(corsConfig))

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
	authenticated.POST("/projects", handler.createProject)
	authenticated.PUT("/projects/order", handler.updateProjectOrder)
	authenticated.PUT("/projects/:code", handler.updateProject)
	authenticated.DELETE("/projects/:code", handler.deleteProject)
	authenticated.GET("/business-lines", handler.listBusinessLines)
	authenticated.POST("/business-lines", handler.createBusinessLine)
	authenticated.PUT("/business-lines/:code", handler.updateBusinessLine)
	authenticated.DELETE("/business-lines/:code", handler.deleteBusinessLine)
	authenticated.PUT("/dependencies/:code", handler.updateDependencies)
	authenticated.GET("/releases", handler.listReleases)
	authenticated.POST("/releases", handler.createRelease)
	authenticated.GET("/releases/:id", handler.getRelease)
	authenticated.GET("/releases/:id/projects/:releaseProjectId/jobs/:jobId/trace", handler.getReleaseJobTrace)

	admin := authenticated.Group("")
	admin.Use(handler.requireRole("admin"))
	admin.GET("/users", handler.listUsers)
	admin.POST("/users", handler.createUser)
	admin.PUT("/users/:id", handler.updateUser)
	admin.DELETE("/users/:id", handler.deleteUser)

	operator := authenticated.Group("")
	operator.Use(handler.requireRole("release_manager", "admin"))
	operator.POST("/releases/:id/tag", handler.createTags)
	operator.POST("/releases/:id/package", handler.packageRelease)
	operator.POST("/releases/:id/deploy", handler.deployRelease)
	operator.DELETE("/releases/:id", handler.deleteRelease)
	operator.POST("/releases/:id/projects/:releaseProjectId/package", handler.packageReleaseProject)
	operator.POST("/releases/:id/projects/:releaseProjectId/deploy", handler.deployReleaseProject)

	return router
}
