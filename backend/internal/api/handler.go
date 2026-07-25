package api

import (
	"net/http"
	"sort"
	"strconv"
	"strings"

	"delivery-platform/backend/internal/auth"
	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/model"
	"delivery-platform/backend/internal/release"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type Handler struct {
	cfg            config.Config
	db             *gorm.DB
	releaseService *release.Service
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type projectDTO struct {
	model.Project
	BusinessLineCode string   `json:"businessLineCode"`
	Dependencies     []string `json:"dependencies"`
}

func (h Handler) health(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func (h Handler) login(c *gin.Context) {
	var req loginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	var user model.User
	if err := h.db.Where("username = ? AND status = ?", req.Username, "enabled").First(&user).Error; err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
		return
	}
	if !auth.CheckPassword(user.PasswordHash, req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{"message": "用户名或密码错误"})
		return
	}
	token, err := auth.GenerateToken(h.cfg.JWTSecret, user.ID, user.Role)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"token": token, "user": user})
}

func (h Handler) me(c *gin.Context) {
	c.JSON(http.StatusOK, currentUser(c))
}

func (h Handler) listProjects(c *gin.Context) {
	projects, err := h.projectsWithDependencies()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, projects)
}

func (h Handler) updateProject(c *gin.Context) {
	var req struct {
		Name             string `json:"name"`
		Kind             string `json:"kind"`
		Owner            string `json:"owner"`
		BusinessLineCode string `json:"businessLineCode"`
		GitLabURL        string `json:"gitlabUrl"`
		GitLabProjectID  string `json:"gitlabProjectId"`
		DefaultBranch    string `json:"defaultBranch"`
		PackageJob       string `json:"packageJob"`
		DeployJob        string `json:"deployJob"`
		SortOrder        int    `json:"sortOrder"`
		Enabled          bool   `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var line model.BusinessLine
	if err := h.db.Where("code = ?", req.BusinessLineCode).First(&line).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "业务线不存在"})
		return
	}

	updates := map[string]interface{}{
		"name":               req.Name,
		"kind":               req.Kind,
		"owner":              req.Owner,
		"business_line_id":   line.ID,
		"git_lab_url":        req.GitLabURL,
		"git_lab_project_id": req.GitLabProjectID,
		"default_branch":     req.DefaultBranch,
		"package_job":        req.PackageJob,
		"deploy_job":         req.DeployJob,
		"sort_order":         req.SortOrder,
		"enabled":            req.Enabled,
	}
	if err := h.db.Model(&model.Project{}).Where("code = ?", c.Param("code")).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listProjects(c)
}

func (h Handler) listBusinessLines(c *gin.Context) {
	var lines []model.BusinessLine
	if err := h.db.Order("code asc").Find(&lines).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, lines)
}

func (h Handler) updateBusinessLine(c *gin.Context) {
	var req struct {
		Name        string `json:"name"`
		Platform    string `json:"platform"`
		TagPrefix   string `json:"tagPrefix"`
		TagTemplate string `json:"tagTemplate"`
		Approver    string `json:"approver"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err := h.db.Model(&model.BusinessLine{}).Where("code = ?", c.Param("code")).Updates(map[string]interface{}{
		"name":         req.Name,
		"platform":     req.Platform,
		"tag_prefix":   req.TagPrefix,
		"tag_template": req.TagTemplate,
		"approver":     req.Approver,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listBusinessLines(c)
}

func (h Handler) updateDependencies(c *gin.Context) {
	var req struct {
		Dependencies []string `json:"dependencies"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}

	var project model.Project
	if err := h.db.Where("code = ?", c.Param("code")).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "项目不存在"})
		return
	}
	var dependencies []model.Project
	if len(req.Dependencies) > 0 {
		if err := h.db.Where("code IN ?", req.Dependencies).Find(&dependencies).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
	}

	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.ProjectDependency{}).Error; err != nil {
			return err
		}
		for _, dep := range dependencies {
			if dep.ID == project.ID {
				continue
			}
			if err := tx.Create(&model.ProjectDependency{
				ProjectID:          project.ID,
				DependsOnProjectID: dep.ID,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listProjects(c)
}

func (h Handler) createRelease(c *gin.Context) {
	var req release.CreateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	created, err := h.releaseService.Create(c.Request.Context(), req, currentUser(c))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, created)
}

func (h Handler) listReleases(c *gin.Context) {
	releases, err := h.releaseService.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, releases)
}

func (h Handler) getRelease(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	item, err := h.releaseService.Get(c.Request.Context(), releaseID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "上线单不存在"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) createTags(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	item, err := h.releaseService.CreateTags(c.Request.Context(), releaseID, currentUser(c))
	writeReleaseResult(c, item, err)
}

func (h Handler) packageRelease(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	target := c.DefaultQuery("target", "all")
	item, err := h.releaseService.Package(c.Request.Context(), releaseID, target, currentUser(c))
	writeReleaseResult(c, item, err)
}

func (h Handler) deployRelease(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	target := c.DefaultQuery("target", "all")
	item, err := h.releaseService.Deploy(c.Request.Context(), releaseID, target, currentUser(c))
	writeReleaseResult(c, item, err)
}

func (h Handler) packageReleaseProject(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	releaseProjectID, ok := parseUintParam(c, "releaseProjectId")
	if !ok {
		return
	}
	item, err := h.releaseService.PackageOne(c.Request.Context(), releaseID, releaseProjectID, currentUser(c))
	writeReleaseResult(c, item, err)
}

func (h Handler) deployReleaseProject(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	releaseProjectID, ok := parseUintParam(c, "releaseProjectId")
	if !ok {
		return
	}
	item, err := h.releaseService.DeployOne(c.Request.Context(), releaseID, releaseProjectID, currentUser(c))
	writeReleaseResult(c, item, err)
}

func (h Handler) projectsWithDependencies() ([]projectDTO, error) {
	var projects []model.Project
	if err := h.db.Preload("BusinessLine").Order("sort_order asc").Find(&projects).Error; err != nil {
		return nil, err
	}

	var deps []model.ProjectDependency
	if err := h.db.Find(&deps).Error; err != nil {
		return nil, err
	}

	projectByID := map[uint]model.Project{}
	for _, project := range projects {
		projectByID[project.ID] = project
	}

	depMap := map[uint][]string{}
	for _, dep := range deps {
		if dependsOn, ok := projectByID[dep.DependsOnProjectID]; ok {
			depMap[dep.ProjectID] = append(depMap[dep.ProjectID], dependsOn.Code)
		}
	}

	result := make([]projectDTO, 0, len(projects))
	for _, project := range projects {
		dependencies := depMap[project.ID]
		sort.Strings(dependencies)
		result = append(result, projectDTO{
			Project:          project,
			BusinessLineCode: project.BusinessLine.Code,
			Dependencies:     dependencies,
		})
	}
	return result, nil
}

func writeReleaseResult(c *gin.Context, item model.Release, err error) {
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, item)
}

func parseUintParam(c *gin.Context, name string) (uint, bool) {
	value, err := strconv.ParseUint(c.Param(name), 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": name + " 参数错误"})
		return 0, false
	}
	return uint(value), true
}

func (h Handler) requireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		tokenString := strings.TrimSpace(strings.TrimPrefix(header, "Bearer "))
		if tokenString == "" || tokenString == header {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "未登录"})
			return
		}

		claims, err := auth.ParseToken(h.cfg.JWTSecret, tokenString)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "登录已失效"})
			return
		}

		var user model.User
		if err := h.db.First(&user, claims.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "用户不存在"})
			return
		}
		c.Set("user", user)
		c.Next()
	}
}

func (h Handler) requireRole(roles ...string) gin.HandlerFunc {
	allowed := map[string]bool{}
	for _, role := range roles {
		allowed[role] = true
	}
	return func(c *gin.Context) {
		user := currentUser(c)
		if !allowed[user.Role] {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"message": "没有权限执行该操作"})
			return
		}
		c.Next()
	}
}

func currentUser(c *gin.Context) model.User {
	value, _ := c.Get("user")
	user, _ := value.(model.User)
	return user
}
