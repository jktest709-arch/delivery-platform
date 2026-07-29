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

type userRequest struct {
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Status      string `json:"status"`
	Password    string `json:"password"`
}

type projectDTO struct {
	model.Project
	BusinessLineCode  string   `json:"businessLineCode"`
	BusinessLineCodes []string `json:"businessLineCodes"`
	Dependencies      []string `json:"dependencies"`
}

type projectRequest struct {
	Code              string   `json:"code"`
	Name              string   `json:"name"`
	Kind              string   `json:"kind"`
	Owner             string   `json:"owner"`
	BusinessLineCode  string   `json:"businessLineCode"`
	BusinessLineCodes []string `json:"businessLineCodes"`
	GitLabURL         string   `json:"gitlabUrl"`
	GitLabProjectID   string   `json:"gitlabProjectId"`
	DefaultBranch     string   `json:"defaultBranch"`
	SortOrder         int      `json:"sortOrder"`
	Enabled           *bool    `json:"enabled"`
}

type businessLineRequest struct {
	Code        string `json:"code"`
	Name        string `json:"name"`
	Platform    string `json:"platform"`
	TagPrefix   string `json:"tagPrefix"`
	TagTemplate string `json:"tagTemplate"`
	Approver    string `json:"approver"`
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

func (h Handler) listUsers(c *gin.Context) {
	var users []model.User
	if err := h.db.Order("username asc").Find(&users).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	c.JSON(http.StatusOK, users)
}

func (h Handler) createUser(c *gin.Context) {
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	username := strings.TrimSpace(req.Username)
	if username == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户名不能为空"})
		return
	}
	if strings.TrimSpace(req.Password) == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "密码不能为空"})
		return
	}
	updates, err := userUpdates(req, true)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	var existing model.User
	err = h.db.Where("username = ?", username).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户名已存在"})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	user := model.User{
		Username:     username,
		DisplayName:  updates["display_name"].(string),
		PasswordHash: updates["password_hash"].(string),
		Role:         updates["role"].(string),
		Status:       updates["status"].(string),
	}
	if err := h.db.Create(&user).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listUsers(c)
}

func (h Handler) updateUser(c *gin.Context) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	var req userRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	updates, err := userUpdates(req, false)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	current := currentUser(c)
	if userID == current.ID {
		if status, ok := updates["status"].(string); ok && status != "enabled" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "不能禁用当前登录用户"})
			return
		}
	}
	result := h.db.Model(&model.User{}).Where("id = ?", userID).Updates(updates)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
		return
	}
	h.listUsers(c)
}

func (h Handler) deleteUser(c *gin.Context) {
	userID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if userID == currentUser(c).ID {
		c.JSON(http.StatusBadRequest, gin.H{"message": "不能删除当前登录用户"})
		return
	}
	var usageCount int64
	if err := h.db.Model(&model.Release{}).
		Where("applicant_id = ? OR approver_id = ?", userID, userID).
		Count(&usageCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if usageCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户已存在上线记录，请改为禁用"})
		return
	}
	if err := h.db.Model(&model.ReleaseEvent{}).
		Where("operator_id = ?", userID).
		Count(&usageCount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if usageCount > 0 {
		c.JSON(http.StatusBadRequest, gin.H{"message": "用户已存在操作记录，请改为禁用"})
		return
	}
	result := h.db.Delete(&model.User{}, userID)
	if result.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": result.Error.Error()})
		return
	}
	if result.RowsAffected == 0 {
		c.JSON(http.StatusNotFound, gin.H{"message": "用户不存在"})
		return
	}
	h.listUsers(c)
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
	var project model.Project
	if err := h.db.Where("code = ?", c.Param("code")).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "项目不存在"})
		return
	}
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	req.Code = c.Param("code")

	updates, businessLines, err := h.projectUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&project).Updates(updates).Error; err != nil {
			return err
		}
		return replaceProjectBusinessLines(tx, project.ID, businessLines)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listProjects(c)
}

func (h Handler) createProject(c *gin.Context) {
	var req projectRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	code := strings.TrimSpace(req.Code)
	req.Code = code
	updates, businessLines, err := h.projectUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	updates["code"] = code
	updates["enabled"] = true

	var existing model.Project
	err = h.db.Where("code = ?", code).First(&existing).Error
	if err == nil {
		if existing.Enabled {
			c.JSON(http.StatusBadRequest, gin.H{"message": "项目 Code 已存在"})
			return
		}
		err := h.db.Transaction(func(tx *gorm.DB) error {
			if err := tx.Model(&existing).Updates(updates).Error; err != nil {
				return err
			}
			return replaceProjectBusinessLines(tx, existing.ID, businessLines)
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		h.listProjects(c)
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	project := model.Project{
		Code:            updates["code"].(string),
		Name:            updates["name"].(string),
		Kind:            updates["kind"].(string),
		Owner:           updates["owner"].(string),
		BusinessLineID:  updates["business_line_id"].(uint),
		GitLabURL:       updates["git_lab_url"].(string),
		GitLabProjectID: updates["git_lab_project_id"].(string),
		DefaultBranch:   updates["default_branch"].(string),
		SortOrder:       updates["sort_order"].(int),
		Enabled:         true,
	}
	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&project).Error; err != nil {
			return err
		}
		return replaceProjectBusinessLines(tx, project.ID, businessLines)
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listProjects(c)
}

func (h Handler) deleteProject(c *gin.Context) {
	var project model.Project
	if err := h.db.Where("code = ?", c.Param("code")).First(&project).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "项目不存在"})
		return
	}
	err := h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("project_id = ?", project.ID).Delete(&model.ProjectBusinessLine{}).Error; err != nil {
			return err
		}
		if err := tx.Where("project_id = ? OR depends_on_project_id = ?", project.ID, project.ID).Delete(&model.ProjectDependency{}).Error; err != nil {
			return err
		}
		return tx.Model(&project).Update("enabled", false).Error
	})
	if err != nil {
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
	var req businessLineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	updates, err := h.businessLineUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	if err := h.db.Model(&model.BusinessLine{}).Where("code = ?", c.Param("code")).Updates(updates).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listBusinessLines(c)
}

func (h Handler) createBusinessLine(c *gin.Context) {
	var req businessLineRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	updates, err := h.businessLineUpdates(req)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": err.Error()})
		return
	}
	code := strings.TrimSpace(req.Code)
	if code == "" {
		c.JSON(http.StatusBadRequest, gin.H{"message": "业务线 Code 不能为空"})
		return
	}
	var existing model.BusinessLine
	err = h.db.Where("code = ?", code).First(&existing).Error
	if err == nil {
		c.JSON(http.StatusBadRequest, gin.H{"message": "业务线 Code 已存在"})
		return
	}
	if err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	line := model.BusinessLine{
		Code:        code,
		Name:        updates["name"].(string),
		Platform:    updates["platform"].(string),
		TagPrefix:   updates["tag_prefix"].(string),
		TagTemplate: updates["tag_template"].(string),
		Approver:    updates["approver"].(string),
	}
	if err := h.db.Create(&line).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	h.listBusinessLines(c)
}

func (h Handler) deleteBusinessLine(c *gin.Context) {
	var line model.BusinessLine
	if err := h.db.Where("code = ?", c.Param("code")).First(&line).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "业务线不存在"})
		return
	}
	projectIDs, err := h.projectIDsUsingBusinessLine(line.ID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
		return
	}
	if len(projectIDs) > 0 {
		replacementCode := strings.TrimSpace(c.Query("replacementCode"))
		if replacementCode == "" {
			c.JSON(http.StatusBadRequest, gin.H{"message": "业务线已被项目使用，请选择替代业务线后删除"})
			return
		}
		if replacementCode == line.Code {
			c.JSON(http.StatusBadRequest, gin.H{"message": "替代业务线不能与待删除业务线相同"})
			return
		}
		var replacement model.BusinessLine
		if err := h.db.Where("code = ?", replacementCode).First(&replacement).Error; err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"message": "替代业务线不存在"})
			return
		}
		err := h.db.Transaction(func(tx *gorm.DB) error {
			for _, projectID := range projectIDs {
				item := model.ProjectBusinessLine{
					ProjectID:      projectID,
					BusinessLineID: replacement.ID,
				}
				if err := tx.Where("project_id = ? AND business_line_id = ?", item.ProjectID, item.BusinessLineID).
					FirstOrCreate(&item).Error; err != nil {
					return err
				}
			}
			if err := tx.Where("business_line_id = ?", line.ID).Delete(&model.ProjectBusinessLine{}).Error; err != nil {
				return err
			}
			if err := tx.Model(&model.Project{}).
				Where("business_line_id = ?", line.ID).
				Update("business_line_id", replacement.ID).Error; err != nil {
				return err
			}
			return tx.Delete(&line).Error
		})
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"message": err.Error()})
			return
		}
		h.listBusinessLines(c)
		return
	}
	if err := h.db.Delete(&line).Error; err != nil {
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
	item, err := h.releaseService.SyncPipelines(c.Request.Context(), releaseID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "上线单不存在"})
		return
	}
	c.JSON(http.StatusOK, item)
}

func (h Handler) deleteRelease(c *gin.Context) {
	releaseID, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	if err := h.releaseService.Delete(c.Request.Context(), releaseID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"message": "发布任务不存在"})
		return
	}
	h.listReleases(c)
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
	if err := h.db.
		Preload("BusinessLine").
		Preload("BusinessLines").
		Where("enabled = ?", true).
		Order("sort_order asc").
		Find(&projects).Error; err != nil {
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
		if dependencies == nil {
			dependencies = []string{}
		}
		sort.Strings(dependencies)
		businessLineCodes := projectBusinessLineCodes(project)
		result = append(result, projectDTO{
			Project:           project,
			BusinessLineCode:  project.BusinessLine.Code,
			BusinessLineCodes: businessLineCodes,
			Dependencies:      dependencies,
		})
	}
	return result, nil
}

func (h Handler) projectUpdates(req projectRequest) (map[string]interface{}, []model.BusinessLine, error) {
	code := strings.TrimSpace(req.Code)
	if code == "" {
		return nil, nil, errMessage("项目 Code 不能为空")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return nil, nil, errMessage("项目名称不能为空")
	}
	if req.Kind != model.ProjectKindBackend && req.Kind != model.ProjectKindFrontend {
		return nil, nil, errMessage("项目类型只能是 backend 或 frontend")
	}
	businessLines, err := h.projectBusinessLineSelection(req)
	if err != nil {
		return nil, nil, err
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	return map[string]interface{}{
		"name":               name,
		"kind":               req.Kind,
		"owner":              strings.TrimSpace(req.Owner),
		"business_line_id":   businessLines[0].ID,
		"git_lab_url":        strings.TrimSpace(req.GitLabURL),
		"git_lab_project_id": strings.TrimSpace(req.GitLabProjectID),
		"default_branch":     strings.TrimSpace(req.DefaultBranch),
		"sort_order":         req.SortOrder,
		"enabled":            enabled,
	}, businessLines, nil
}

func (h Handler) projectBusinessLineSelection(req projectRequest) ([]model.BusinessLine, error) {
	codes := normalizedProjectBusinessLineCodes(req)
	if len(codes) == 0 {
		return nil, errMessage("至少选择一条业务线")
	}

	lines := make([]model.BusinessLine, 0, len(codes))
	for _, code := range codes {
		var line model.BusinessLine
		if err := h.db.Where("code = ?", code).First(&line).Error; err != nil {
			return nil, errMessage("业务线不存在：" + code)
		}
		lines = append(lines, line)
	}
	return lines, nil
}

func normalizedProjectBusinessLineCodes(req projectRequest) []string {
	codes := []string{}
	seen := map[string]bool{}
	add := func(value string) {
		code := strings.TrimSpace(value)
		if code == "" || seen[code] {
			return
		}
		seen[code] = true
		codes = append(codes, code)
	}
	add(req.BusinessLineCode)
	for _, code := range req.BusinessLineCodes {
		add(code)
	}
	return codes
}

func replaceProjectBusinessLines(tx *gorm.DB, projectID uint, lines []model.BusinessLine) error {
	if err := tx.Where("project_id = ?", projectID).Delete(&model.ProjectBusinessLine{}).Error; err != nil {
		return err
	}
	for _, line := range lines {
		item := model.ProjectBusinessLine{
			ProjectID:      projectID,
			BusinessLineID: line.ID,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func (h Handler) projectIDsUsingBusinessLine(businessLineID uint) ([]uint, error) {
	projectIDSet := map[uint]bool{}

	var relationIDs []uint
	if err := h.db.Model(&model.ProjectBusinessLine{}).
		Where("business_line_id = ?", businessLineID).
		Pluck("project_id", &relationIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range relationIDs {
		projectIDSet[id] = true
	}

	var defaultIDs []uint
	if err := h.db.Model(&model.Project{}).
		Where("business_line_id = ?", businessLineID).
		Pluck("id", &defaultIDs).Error; err != nil {
		return nil, err
	}
	for _, id := range defaultIDs {
		projectIDSet[id] = true
	}

	result := make([]uint, 0, len(projectIDSet))
	for id := range projectIDSet {
		result = append(result, id)
	}
	return result, nil
}

func projectBusinessLineCodes(project model.Project) []string {
	codes := []string{}
	seen := map[string]bool{}
	add := func(code string) {
		code = strings.TrimSpace(code)
		if code == "" || seen[code] {
			return
		}
		seen[code] = true
		codes = append(codes, code)
	}
	add(project.BusinessLine.Code)
	for _, line := range project.BusinessLines {
		add(line.Code)
	}
	return codes
}

func (h Handler) businessLineUpdates(req businessLineRequest) (map[string]interface{}, error) {
	if strings.TrimSpace(req.Name) == "" {
		return nil, errMessage("业务线名称不能为空")
	}
	if strings.TrimSpace(req.TagPrefix) == "" {
		return nil, errMessage("Tag 前缀不能为空")
	}
	template := strings.TrimSpace(req.TagTemplate)
	if template == "" {
		template = "{prefix}-{timestamp}-{releaseNo}"
	}
	return map[string]interface{}{
		"name":         strings.TrimSpace(req.Name),
		"platform":     strings.TrimSpace(req.Platform),
		"tag_prefix":   strings.TrimSpace(req.TagPrefix),
		"tag_template": template,
		"approver":     strings.TrimSpace(req.Approver),
	}, nil
}

func userUpdates(req userRequest, requirePassword bool) (map[string]interface{}, error) {
	displayName := strings.TrimSpace(req.DisplayName)
	if displayName == "" {
		return nil, errMessage("姓名不能为空")
	}
	role := strings.TrimSpace(req.Role)
	if role != model.RoleDeveloper && role != model.RoleReleaseManager && role != model.RoleAdmin {
		return nil, errMessage("角色只能是 developer、release_manager 或 admin")
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = "enabled"
	}
	if status != "enabled" && status != "disabled" {
		return nil, errMessage("状态只能是 enabled 或 disabled")
	}
	updates := map[string]interface{}{
		"display_name": displayName,
		"role":         role,
		"status":       status,
	}
	password := strings.TrimSpace(req.Password)
	if password == "" {
		if requirePassword {
			return nil, errMessage("密码不能为空")
		}
		return updates, nil
	}
	hash, err := auth.HashPassword(password)
	if err != nil {
		return nil, err
	}
	updates["password_hash"] = hash
	return updates, nil
}

type errMessage string

func (e errMessage) Error() string {
	return string(e)
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
		if err := h.db.Where("status = ?", "enabled").First(&user, claims.UserID).Error; err != nil {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"message": "用户不存在或已禁用"})
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
