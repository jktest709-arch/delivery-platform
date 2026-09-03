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
	router.Use(func(c *gin.Context) {
		const maxRequestBodyBytes = 2 << 20
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxRequestBodyBytes)
		c.Next()
	})
	corsConfig := cors.Config{
		AllowMethods: []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions},
		AllowHeaders: []string{"Authorization", "Content-Type"},
	}
	if cfg.CORSAllowAll {
		corsConfig.AllowAllOrigins = true
	} else {
		corsConfig.AllowOrigins = cfg.CORSOrigins
		corsConfig.AllowCredentials = true
	}
	router.Use(cors.New(corsConfig))

	handler := Handler{
		cfg:            cfg,
		db:             db,
		releaseService: releaseService,
		operationSlots: make(chan struct{}, 4),
	}

	router.GET("/healthz", handler.health)
	router.GET("/readyz", handler.ready)

	api := router.Group("/api")
	api.POST("/auth/login", handler.login)
	api.POST("/auth/logout", handler.logout)

	authenticated := api.Group("")
	authenticated.Use(handler.requireAuth())
	authenticated.GET("/me", handler.me)
	authenticated.GET("/projects", handler.listProjects)
	authenticated.GET("/business-lines", handler.listBusinessLines)
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
	admin.POST("/projects", handler.createProject)
	admin.PUT("/projects/order", handler.updateProjectOrder)
	admin.PUT("/projects/:code", handler.updateProject)
	admin.DELETE("/projects/:code", handler.deleteProject)
	admin.POST("/business-lines", handler.createBusinessLine)
	admin.PUT("/business-lines/:code", handler.updateBusinessLine)
	admin.DELETE("/business-lines/:code", handler.deleteBusinessLine)
	admin.PUT("/dependencies/:code", handler.updateDependencies)

	operator := authenticated.Group("")
	operator.Use(handler.requireRole("release_manager", "admin"))
	operator.POST("/releases/:id/approve", handler.approveRelease)
	operator.POST("/releases/:id/tag", handler.createTags)
	operator.POST("/releases/:id/package", handler.packageRelease)
	operator.POST("/releases/:id/deploy", handler.deployRelease)
	operator.DELETE("/releases/:id", handler.deleteRelease)
	operator.PUT("/releases/:id/projects/:releaseProjectId/source", handler.updateReleaseProjectSource)
	operator.POST("/releases/:id/projects/:releaseProjectId/tag", handler.tagReleaseProject)
	operator.POST("/releases/:id/projects/:releaseProjectId/package", handler.packageReleaseProject)
	operator.POST("/releases/:id/projects/:releaseProjectId/deploy", handler.deployReleaseProject)

	return router
}
