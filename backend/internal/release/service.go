package release

import (
	"context"
	"fmt"
	"strings"
	"time"

	"delivery-platform/backend/internal/gitlab"
	"delivery-platform/backend/internal/model"
	"gorm.io/gorm"
)

type GitLabClient interface {
	CreateTag(ctx context.Context, projectID, tagName, ref string) error
	TriggerPipeline(ctx context.Context, projectID, ref string, variables map[string]string) (gitlab.PipelineResponse, error)
}

type Service struct {
	db     *gorm.DB
	gitlab GitLabClient
}

type CreateRequest struct {
	ReleaseWindow time.Time              `json:"releaseWindow"`
	Remark        string                 `json:"remark"`
	Projects      []CreateProjectRequest `json:"projects"`
}

type CreateProjectRequest struct {
	ProjectCode string `json:"projectCode"`
	SourceType  string `json:"sourceType"`
	SourceRef   string `json:"sourceRef"`
}

func NewService(db *gorm.DB, client GitLabClient) *Service {
	return &Service{db: db, gitlab: client}
}

func (s *Service) Create(ctx context.Context, req CreateRequest, applicant model.User) (model.Release, error) {
	if len(req.Projects) == 0 {
		return model.Release{}, fmt.Errorf("at least one project is required")
	}

	projectCodes := make([]string, 0, len(req.Projects))
	sourceByCode := map[string]CreateProjectRequest{}
	for _, item := range req.Projects {
		if strings.TrimSpace(item.ProjectCode) == "" || strings.TrimSpace(item.SourceRef) == "" {
			return model.Release{}, fmt.Errorf("projectCode and sourceRef are required")
		}
		projectCodes = append(projectCodes, item.ProjectCode)
		sourceByCode[item.ProjectCode] = item
	}

	var projects []model.Project
	if err := s.db.WithContext(ctx).
		Preload("BusinessLine").
		Where("code IN ? AND enabled = ?", projectCodes, true).
		Order("sort_order asc").
		Find(&projects).Error; err != nil {
		return model.Release{}, err
	}
	if len(projects) != len(projectCodes) {
		return model.Release{}, fmt.Errorf("some selected projects are not found or disabled")
	}

	batchNo, releaseNo, err := s.nextBatchNo(ctx)
	if err != nil {
		return model.Release{}, err
	}
	if req.ReleaseWindow.IsZero() {
		req.ReleaseWindow = time.Now().Add(2 * time.Hour)
	}
	tagTime := time.Now()

	var created model.Release
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		created = model.Release{
			BatchNo:       batchNo,
			ApplicantID:   applicant.ID,
			Status:        model.ReleaseStatusPending,
			ReleaseWindow: req.ReleaseWindow,
			Remark:        req.Remark,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}

		for _, project := range projects {
			source := sourceByCode[project.Code]
			targetTag := renderTagAt(project.BusinessLine, releaseNo, tagTime)
			releaseProject := model.ReleaseProject{
				ReleaseID:  created.ID,
				ProjectID:  project.ID,
				SourceType: source.SourceType,
				SourceRef:  source.SourceRef,
				TargetTag:  targetTag,
				Status:     model.ProjectStatusPending,
				SortOrder:  project.SortOrder,
			}
			if err := tx.Create(&releaseProject).Error; err != nil {
				return err
			}
		}

		return tx.Create(&model.ReleaseEvent{
			ReleaseID:  created.ID,
			OperatorID: &applicant.ID,
			Action:     "create_release",
			Message:    fmt.Sprintf("提交上线申请，选择 %d 个项目。", len(projects)),
		}).Error
	})
	if err != nil {
		return model.Release{}, err
	}
	return s.Get(ctx, created.ID)
}

func (s *Service) CreateTags(ctx context.Context, releaseID uint, operator model.User) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range release.Projects {
			if err := s.gitlab.CreateTag(ctx, item.Project.GitLabProjectID, item.TargetTag, item.SourceRef); err != nil {
				tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
					"status":        model.ProjectStatusBuildFailed,
					"error_message": err.Error(),
				})
				return err
			}
			if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
				"status": model.ProjectStatusTagged,
			}).Error; err != nil {
				return err
			}
		}
		if err := tx.Model(&model.Release{}).Where("id = ?", release.ID).Update("status", model.ReleaseStatusTagged).Error; err != nil {
			return err
		}
		return tx.Create(event(release.ID, operator.ID, "create_tags", "已按业务线配置统一创建生产 tag。")).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) Package(ctx context.Context, releaseID uint, target string, operator model.User) (model.Release, error) {
	return s.runPipelines(ctx, releaseID, target, operator, "package")
}

func (s *Service) Deploy(ctx context.Context, releaseID uint, target string, operator model.User) (model.Release, error) {
	return s.runPipelines(ctx, releaseID, target, operator, "deploy")
}

func (s *Service) PackageOne(ctx context.Context, releaseID uint, releaseProjectID uint, operator model.User) (model.Release, error) {
	return s.runOne(ctx, releaseID, releaseProjectID, operator, "package")
}

func (s *Service) DeployOne(ctx context.Context, releaseID uint, releaseProjectID uint, operator model.User) (model.Release, error) {
	return s.runOne(ctx, releaseID, releaseProjectID, operator, "deploy")
}

func (s *Service) Get(ctx context.Context, id uint) (model.Release, error) {
	var release model.Release
	err := s.db.WithContext(ctx).
		Preload("Applicant").
		Preload("Approver").
		Preload("Projects.Project.BusinessLine").
		Preload("Events.Operator").
		First(&release, id).Error
	if err != nil {
		return model.Release{}, err
	}
	sortRelease(&release)
	return release, nil
}

func (s *Service) List(ctx context.Context) ([]model.Release, error) {
	var releases []model.Release
	err := s.db.WithContext(ctx).
		Preload("Applicant").
		Preload("Approver").
		Preload("Projects.Project.BusinessLine").
		Preload("Events.Operator").
		Order("created_at desc").
		Find(&releases).Error
	if err != nil {
		return nil, err
	}
	for i := range releases {
		sortRelease(&releases[i])
	}
	return releases, nil
}

func (s *Service) Delete(ctx context.Context, id uint) error {
	var release model.Release
	if err := s.db.WithContext(ctx).First(&release, id).Error; err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_id = ?", id).Delete(&model.ReleaseEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("release_id = ?", id).Delete(&model.ReleaseProject{}).Error; err != nil {
			return err
		}
		return tx.Delete(&release).Error
	})
}

func (s *Service) runPipelines(ctx context.Context, releaseID uint, target string, operator model.User, action string) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range filterByTarget(release.Projects, target) {
			if err := s.trigger(ctx, tx, item, action); err != nil {
				return err
			}
		}
		if err := s.refreshReleaseStatus(tx, release.ID); err != nil {
			return err
		}
		label := "全量"
		if target == model.ProjectKindBackend {
			label = "后端"
		}
		if target == model.ProjectKindFrontend {
			label = "前端"
		}
		return tx.Create(event(release.ID, operator.ID, action+"_"+target, fmt.Sprintf("已触发%s%s流水线。", label, actionLabel(action)))).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) runOne(ctx context.Context, releaseID uint, releaseProjectID uint, operator model.User, action string) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}

	var target *model.ReleaseProject
	for i := range release.Projects {
		if release.Projects[i].ID == releaseProjectID {
			target = &release.Projects[i]
			break
		}
	}
	if target == nil {
		return release, fmt.Errorf("release project not found")
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.trigger(ctx, tx, *target, action); err != nil {
			return err
		}
		if err := s.refreshReleaseStatus(tx, release.ID); err != nil {
			return err
		}
		return tx.Create(event(release.ID, operator.ID, action+"_single", fmt.Sprintf("已单独触发 %s 的%s流水线。", target.Project.Name, actionLabel(action)))).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) trigger(ctx context.Context, tx *gorm.DB, item model.ReleaseProject, action string) error {
	jobName := item.Project.PackageJob
	nextStatus := model.ProjectStatusBuilding
	successStatus := model.ProjectStatusBuildSuccess
	releaseStatus := model.ReleaseStatusBuilding
	if action == "deploy" {
		jobName = item.Project.DeployJob
		nextStatus = model.ProjectStatusDeploying
		successStatus = model.ProjectStatusDeploySuccess
		releaseStatus = model.ReleaseStatusDeploying
	}

	if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Update("status", nextStatus).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.Release{}).Where("id = ?", item.ReleaseID).Update("status", releaseStatus).Error; err != nil {
		return err
	}

	pipeline, err := s.gitlab.TriggerPipeline(ctx, item.Project.GitLabProjectID, item.TargetTag, map[string]string{
		"RELEASE_ID": item.TargetTag,
		"JOB_NAME":   jobName,
		"ACTION":     action,
	})
	if err != nil {
		failedStatus := model.ProjectStatusBuildFailed
		if action == "deploy" {
			failedStatus = model.ProjectStatusDeployFailed
		}
		tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(map[string]interface{}{
			"status":        failedStatus,
			"error_message": err.Error(),
		})
		return err
	}

	updates := map[string]interface{}{
		"pipeline_id":   pipeline.ID,
		"status":        successStatus,
		"error_message": "",
	}
	if action == "package" {
		updates["build_job_id"] = pipeline.ID
	} else {
		updates["deploy_job_id"] = pipeline.ID
	}
	return tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(updates).Error
}

func (s *Service) nextBatchNo(ctx context.Context) (string, string, error) {
	today := time.Now().Format("20060102")
	var count int64
	prefix := fmt.Sprintf("PRD-%s-", today)
	if err := s.db.WithContext(ctx).Model(&model.Release{}).Where("batch_no LIKE ?", prefix+"%").Count(&count).Error; err != nil {
		return "", "", err
	}
	releaseNo := fmt.Sprintf("%03d", count+1)
	return prefix + releaseNo, releaseNo, nil
}

func renderTag(line model.BusinessLine, releaseNo string) string {
	return renderTagAt(line, releaseNo, time.Now())
}

func renderTagAt(line model.BusinessLine, releaseNo string, timestamp time.Time) string {
	value := line.TagTemplate
	stamp := timestamp.Format("20060102150405")
	value = strings.ReplaceAll(value, "{prefix}", line.TagPrefix)
	value = strings.ReplaceAll(value, "{timestamp}", stamp)
	value = strings.ReplaceAll(value, "{datetime}", stamp)
	value = strings.ReplaceAll(value, "{date}", stamp)
	value = strings.ReplaceAll(value, "{releaseNo}", releaseNo)
	return value
}

func filterByTarget(projects []model.ReleaseProject, target string) []model.ReleaseProject {
	if target == "" || target == "all" {
		return projects
	}
	result := make([]model.ReleaseProject, 0, len(projects))
	for _, item := range projects {
		if item.Project.Kind == target {
			result = append(result, item)
		}
	}
	return result
}

func (s *Service) refreshReleaseStatus(tx *gorm.DB, releaseID uint) error {
	var projects []model.ReleaseProject
	if err := tx.Where("release_id = ?", releaseID).Find(&projects).Error; err != nil {
		return err
	}
	if len(projects) == 0 {
		return nil
	}

	status := model.ReleaseStatusPending
	failed := false
	allTagged := true
	allBuildSuccess := true
	allDeploySuccess := true
	anyBuilding := false
	anyDeploying := false

	for _, item := range projects {
		switch item.Status {
		case model.ProjectStatusBuildFailed, model.ProjectStatusDeployFailed:
			failed = true
		case model.ProjectStatusBuilding:
			anyBuilding = true
		case model.ProjectStatusDeploying:
			anyDeploying = true
		}
		if item.Status != model.ProjectStatusTagged &&
			item.Status != model.ProjectStatusBuildSuccess &&
			item.Status != model.ProjectStatusDeploying &&
			item.Status != model.ProjectStatusDeploySuccess {
			allTagged = false
		}
		if item.Status != model.ProjectStatusBuildSuccess &&
			item.Status != model.ProjectStatusDeploying &&
			item.Status != model.ProjectStatusDeploySuccess {
			allBuildSuccess = false
		}
		if item.Status != model.ProjectStatusDeploySuccess {
			allDeploySuccess = false
		}
	}

	switch {
	case failed:
		status = model.ReleaseStatusPartialFailed
	case allDeploySuccess:
		status = model.ReleaseStatusDeploySuccess
	case anyDeploying:
		status = model.ReleaseStatusDeploying
	case allBuildSuccess:
		status = model.ReleaseStatusBuildSuccess
	case anyBuilding:
		status = model.ReleaseStatusBuilding
	case allTagged:
		status = model.ReleaseStatusTagged
	}
	return tx.Model(&model.Release{}).Where("id = ?", releaseID).Update("status", status).Error
}

func actionLabel(action string) string {
	if action == "deploy" {
		return "部署"
	}
	return "打包"
}

func event(releaseID uint, operatorID uint, action, message string) model.ReleaseEvent {
	return model.ReleaseEvent{
		ReleaseID:  releaseID,
		OperatorID: &operatorID,
		Action:     action,
		Message:    message,
	}
}

func sortRelease(release *model.Release) {
	for i := range release.Projects {
		for j := i + 1; j < len(release.Projects); j++ {
			if release.Projects[j].SortOrder < release.Projects[i].SortOrder {
				release.Projects[i], release.Projects[j] = release.Projects[j], release.Projects[i]
			}
		}
	}
	for i := range release.Events {
		for j := i + 1; j < len(release.Events); j++ {
			if release.Events[j].CreatedAt.After(release.Events[i].CreatedAt) {
				release.Events[i], release.Events[j] = release.Events[j], release.Events[i]
			}
		}
	}
}
