package release

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"delivery-platform/backend/internal/gitlab"
	"delivery-platform/backend/internal/model"
	"gorm.io/gorm"
)

type GitLabClient interface {
	CreateTag(ctx context.Context, projectID, tagName, ref string) error
	FindPipelineByRef(ctx context.Context, projectID, ref string) (gitlab.PipelineResponse, error)
	ListPipelineJobs(ctx context.Context, projectID, pipelineID string) ([]gitlab.JobResponse, error)
	PlayJob(ctx context.Context, projectID, jobID string) (gitlab.JobResponse, error)
	RetryJob(ctx context.Context, projectID, jobID string) (gitlab.JobResponse, error)
	GetJobTrace(ctx context.Context, projectID, jobID string) (string, error)
}

type Service struct {
	db     *gorm.DB
	gitlab GitLabClient
}

var (
	autoBuildPollInterval = 2 * time.Second
	autoBuildPollAttempts = 150
)

type CreateRequest struct {
	BusinessLineCode string                 `json:"businessLineCode"`
	ReleaseWindow    time.Time              `json:"releaseWindow"`
	Remark           string                 `json:"remark"`
	Projects         []CreateProjectRequest `json:"projects"`
}

type CreateProjectRequest struct {
	ProjectCode      string `json:"projectCode"`
	BusinessLineCode string `json:"businessLineCode"`
	SourceType       string `json:"sourceType"`
	SourceRef        string `json:"sourceRef"`
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
		Preload("BusinessLines").
		Where("code IN ? AND enabled = ?", projectCodes, true).
		Order("sort_order asc").
		Find(&projects).Error; err != nil {
		return model.Release{}, err
	}
	if len(projects) != len(projectCodes) {
		return model.Release{}, fmt.Errorf("some selected projects are not found or disabled")
	}
	orderedProjects, err := s.orderProjectsByDependencies(ctx, projects)
	if err != nil {
		return model.Release{}, err
	}
	projects = orderedProjects
	releaseBusinessLineCode := strings.TrimSpace(req.BusinessLineCode)
	businessLineByProjectCode := map[string]model.BusinessLine{}
	var releaseBusinessLine model.BusinessLine
	for _, project := range projects {
		source := sourceByCode[project.Code]
		requestedCode := strings.TrimSpace(source.BusinessLineCode)
		if requestedCode == "" {
			requestedCode = releaseBusinessLineCode
		}
		businessLine, err := selectProjectBusinessLine(project, requestedCode)
		if err != nil {
			return model.Release{}, err
		}
		businessLineByProjectCode[project.Code] = businessLine
		if releaseBusinessLine.ID == 0 {
			releaseBusinessLine = businessLine
		}
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
			BatchNo:        batchNo,
			ApplicantID:    applicant.ID,
			BusinessLineID: releaseBusinessLine.ID,
			Status:         model.ReleaseStatusPending,
			ReleaseWindow:  req.ReleaseWindow,
			Remark:         req.Remark,
		}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}

		for index, project := range projects {
			source := sourceByCode[project.Code]
			businessLine := businessLineByProjectCode[project.Code]
			targetTag := renderTagAt(businessLine, releaseNo, tagTime)
			releaseProject := model.ReleaseProject{
				ReleaseID:      created.ID,
				ProjectID:      project.ID,
				BusinessLineID: businessLine.ID,
				SourceType:     source.SourceType,
				SourceRef:      source.SourceRef,
				TargetTag:      targetTag,
				Status:         model.ProjectStatusPending,
				SortOrder:      (index + 1) * 10,
			}
			if err := tx.Create(&releaseProject).Error; err != nil {
				return err
			}
		}

		return tx.Create(&model.ReleaseEvent{
			ReleaseID:  created.ID,
			OperatorID: &applicant.ID,
			Action:     "create_release",
			Message:    fmt.Sprintf("提交上线申请，业务线 %s，选择 %d 个项目。", releaseBusinessLine.Name, len(projects)),
		}).Error
	})
	if err != nil {
		return model.Release{}, err
	}
	return s.Get(ctx, created.ID)
}

func (s *Service) CreateTags(ctx context.Context, releaseID uint, target string, mode string, operator model.User) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}

	target = normalizeTarget(target)
	selectedProjects := filterByTarget(release.Projects, target)
	if len(selectedProjects) == 0 {
		return release, fmt.Errorf("没有匹配的%s项目", targetLabel(target))
	}
	if mode == "restart" {
		if err := s.resetTagsForTarget(ctx, release, target); err != nil {
			return release, err
		}
		release, err = s.Get(ctx, releaseID)
		if err != nil {
			return release, err
		}
		selectedProjects = filterByTarget(release.Projects, target)
	}

	for _, item := range selectedProjects {
		if err := s.createOrResumeTagBuild(ctx, item); err != nil {
			return release, err
		}
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.refreshReleaseStatus(tx, release.ID); err != nil {
			return err
		}
		action := "create_tags_" + target
		message := fmt.Sprintf("已按依赖顺序创建%s项目 tag，并同步 GitLab pipeline/jobs。", targetLabel(target))
		if mode == "restart" {
			action = "restart_tags_" + target
			message = fmt.Sprintf("已重置并重新创建%s项目 tag，按依赖顺序触发 GitLab CI 构建。", targetLabel(target))
		}
		item := event(release.ID, operator.ID, action, message)
		return tx.Create(&item).Error
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
	return s.runOne(ctx, releaseID, releaseProjectID, operator, "rebuild")
}

func (s *Service) TagOne(ctx context.Context, releaseID uint, releaseProjectID uint, operator model.User) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}
	target, err := findReleaseProject(release, releaseProjectID)
	if err != nil {
		return release, err
	}
	target, err = s.resetTagForProject(ctx, release, target)
	if err != nil {
		return release, err
	}
	if err := s.createTagAndWaitBuild(ctx, target); err != nil {
		return release, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.refreshReleaseStatus(tx, release.ID); err != nil {
			return err
		}
		item := event(release.ID, operator.ID, "retag_single", fmt.Sprintf("已使用最新来源为 %s 重新创建 tag 并触发 GitLab CI。", target.Project.Name))
		return tx.Create(&item).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) DeployOne(ctx context.Context, releaseID uint, releaseProjectID uint, operator model.User) (model.Release, error) {
	return s.runOne(ctx, releaseID, releaseProjectID, operator, "deploy")
}

func (s *Service) UpdateProjectSource(ctx context.Context, releaseID uint, releaseProjectID uint, sourceType string, sourceRef string, operator model.User) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil {
		return release, err
	}
	target, err := findReleaseProject(release, releaseProjectID)
	if err != nil {
		return release, err
	}
	sourceType, sourceRef, err = normalizeSource(sourceType, sourceRef)
	if err != nil {
		return release, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updates := map[string]interface{}{
			"source_type":  sourceType,
			"source_ref":   sourceRef,
			"source_dirty": true,
		}
		if err := tx.Model(&model.ReleaseProject{}).
			Where("id = ? AND release_id = ?", releaseProjectID, releaseID).
			Updates(updates).Error; err != nil {
			return err
		}
		item := event(release.ID, operator.ID, "update_project_source", fmt.Sprintf("已调整 %s 来源为 %s: %s。", target.Project.Name, sourceLabel(sourceType), sourceRef))
		return tx.Create(&item).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) Get(ctx context.Context, id uint) (model.Release, error) {
	var release model.Release
	err := s.db.WithContext(ctx).
		Preload("Applicant").
		Preload("BusinessLine").
		Preload("Approver").
		Preload("Projects.BusinessLine").
		Preload("Projects.Jobs").
		Preload("Projects.Project.BusinessLine").
		Preload("Projects.Project.BusinessLines").
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
		Preload("BusinessLine").
		Preload("Approver").
		Preload("Projects.BusinessLine").
		Preload("Projects.Jobs").
		Preload("Projects.Project.BusinessLine").
		Preload("Projects.Project.BusinessLines").
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

func (s *Service) SyncPipelines(ctx context.Context, releaseID uint) (model.Release, error) {
	release, err := s.Get(ctx, releaseID)
	if err != nil || s.gitlab == nil {
		return release, err
	}

	for _, item := range release.Projects {
		if item.PipelineID == "" || !shouldRefreshPipelineJobs(item) {
			continue
		}
		jobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, item.PipelineID)
		if err != nil {
			continue
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := s.updateReleaseProjectFromJobs(tx, item.ID, item.PipelineID, jobs, ""); err != nil {
				return err
			}
			return s.refreshReleaseStatus(tx, item.ReleaseID)
		}); err != nil {
			return release, err
		}
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) JobTrace(ctx context.Context, releaseID uint, releaseProjectID uint, releaseJobID uint) (string, error) {
	if s.gitlab == nil {
		return "", fmt.Errorf("gitlab client is not configured")
	}
	var releaseProject model.ReleaseProject
	if err := s.db.WithContext(ctx).
		Preload("Project").
		Where("id = ? AND release_id = ?", releaseProjectID, releaseID).
		First(&releaseProject).Error; err != nil {
		return "", err
	}

	var job model.ReleasePipelineJob
	if err := s.db.WithContext(ctx).
		Where("id = ? AND release_project_id = ?", releaseJobID, releaseProjectID).
		First(&job).Error; err != nil {
		return "", err
	}
	return s.gitlab.GetJobTrace(ctx, releaseProject.Project.GitLabProjectID, job.GitLabJobID)
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
		var releaseProjectIDs []uint
		if err := tx.Model(&model.ReleaseProject{}).Where("release_id = ?", id).Pluck("id", &releaseProjectIDs).Error; err != nil {
			return err
		}
		if len(releaseProjectIDs) > 0 {
			if err := tx.Where("release_project_id IN ?", releaseProjectIDs).Delete(&model.ReleasePipelineJob{}).Error; err != nil {
				return err
			}
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
			if err := s.playJobs(ctx, tx, item, action); err != nil {
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
		item := event(release.ID, operator.ID, action+"_"+target, fmt.Sprintf("已触发%s%s job。", label, actionLabel(action)))
		return tx.Create(&item).Error
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
		if err := s.playJobs(ctx, tx, *target, action); err != nil {
			return err
		}
		if err := s.refreshReleaseStatus(tx, release.ID); err != nil {
			return err
		}
		item := event(release.ID, operator.ID, action+"_single", fmt.Sprintf("已单独触发 %s 的%s job。", target.Project.Name, actionLabel(action)))
		return tx.Create(&item).Error
	})
	if err != nil {
		return release, err
	}
	return s.Get(ctx, releaseID)
}

func (s *Service) playJobs(ctx context.Context, tx *gorm.DB, item model.ReleaseProject, action string) error {
	nextStatus := model.ProjectStatusBuilding
	releaseStatus := model.ReleaseStatusBuilding
	if action == "deploy" {
		nextStatus = model.ProjectStatusDeploying
		releaseStatus = model.ReleaseStatusDeploying
	}

	if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Update("status", nextStatus).Error; err != nil {
		return err
	}
	if err := tx.Model(&model.Release{}).Where("id = ?", item.ReleaseID).Update("status", releaseStatus).Error; err != nil {
		return err
	}

	if item.PipelineID == "" {
		pipeline, err := s.findPipelineAfterTag(ctx, item)
		if err != nil {
			return s.markActionFailed(tx, item.ID, action, err)
		}
		if err := s.syncPipelineJobs(ctx, tx, item, pipeline); err != nil {
			return err
		}
		item.PipelineID = pipeline.ID
	}

	jobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, item.PipelineID)
	if err != nil {
		return s.markActionFailed(tx, item.ID, action, err)
	}
	jobAction := actionJobKind(action)
	matchingJobs := filterActionJobs(jobs, jobAction)
	if len(matchingJobs) == 0 {
		return s.markActionFailed(tx, item.ID, action, fmt.Errorf("未找到可匹配的%s job", actionLabel(action)))
	}
	playableJobs := filterPlayableJobs(matchingJobs)
	if action == "rebuild" {
		return s.rebuildJobs(ctx, tx, item, jobs, matchingJobs, playableJobs)
	}
	if len(playableJobs) == 0 {
		if err := s.replacePipelineJobs(tx, item.ID, jobs); err != nil {
			return err
		}
		status := projectStatusFromJobs(jobs)
		if status == model.ProjectStatusBuildSuccess || status == model.ProjectStatusDeploySuccess ||
			status == model.ProjectStatusBuilding || status == model.ProjectStatusDeploying {
			return s.updateReleaseProjectFromJobs(tx, item.ID, item.PipelineID, jobs, "")
		}
		return s.markActionFailed(tx, item.ID, action, fmt.Errorf("未找到 manual 状态的%s job", actionLabel(action)))
	}

	for _, job := range playableJobs {
		if _, err := s.gitlab.PlayJob(ctx, item.Project.GitLabProjectID, job.ID); err != nil {
			return s.markActionFailed(tx, item.ID, action, err)
		}
	}

	refreshedJobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, item.PipelineID)
	if err != nil {
		return s.markActionFailed(tx, item.ID, action, err)
	}
	return s.updateReleaseProjectFromJobs(tx, item.ID, item.PipelineID, refreshedJobs, "")
}

func (s *Service) rebuildJobs(ctx context.Context, tx *gorm.DB, item model.ReleaseProject, jobs []gitlab.JobResponse, matchingJobs []gitlab.JobResponse, playableJobs []gitlab.JobResponse) error {
	if len(playableJobs) > 0 {
		for _, job := range playableJobs {
			if _, err := s.gitlab.PlayJob(ctx, item.Project.GitLabProjectID, job.ID); err != nil {
				return s.markActionFailed(tx, item.ID, "rebuild", err)
			}
		}
	} else {
		retryJob, ok := latestRetryableJob(matchingJobs)
		if !ok {
			if err := s.replacePipelineJobs(tx, item.ID, jobs); err != nil {
				return err
			}
			if actionState(jobs, "package") == "running" {
				return s.updateReleaseProjectFromJobs(tx, item.ID, item.PipelineID, jobs, "")
			}
			return s.markActionFailed(tx, item.ID, "rebuild", fmt.Errorf("未找到可重新构建的 build/package job"))
		}
		if _, err := s.gitlab.RetryJob(ctx, item.Project.GitLabProjectID, retryJob.ID); err != nil {
			return s.markActionFailed(tx, item.ID, "rebuild", err)
		}
	}

	refreshedJobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, item.PipelineID)
	if err != nil {
		return s.markActionFailed(tx, item.ID, "rebuild", err)
	}
	return s.updateReleaseProjectFromJobs(tx, item.ID, item.PipelineID, refreshedJobs, "")
}

func (s *Service) createOrResumeTagBuild(ctx context.Context, item model.ReleaseProject) error {
	if item.SourceDirty {
		release, err := s.Get(ctx, item.ReleaseID)
		if err != nil {
			return err
		}
		refreshedItem, err := s.resetTagForProject(ctx, release, item)
		if err != nil {
			return err
		}
		return s.createTagAndWaitBuild(ctx, refreshedItem)
	}
	if batchBuildCompleted(item) {
		return nil
	}
	if item.Status == model.ProjectStatusBuildFailed && item.PipelineID != "" {
		return s.retryBuildAndWait(ctx, item)
	}
	return s.createTagAndWaitBuild(ctx, item)
}

func batchBuildCompleted(item model.ReleaseProject) bool {
	switch item.Status {
	case model.ProjectStatusBuildSuccess, model.ProjectStatusDeploying, model.ProjectStatusDeploySuccess:
		return true
	default:
		return false
	}
}

func (s *Service) retryBuildAndWait(ctx context.Context, item model.ReleaseProject) error {
	if err := s.playJobs(ctx, s.db.WithContext(ctx), item, "rebuild"); err != nil {
		return err
	}
	return s.waitForAutoBuild(ctx, item, item.PipelineID)
}

func (s *Service) resetTagsForTarget(ctx context.Context, release model.Release, target string) error {
	selectedProjects := filterByTarget(release.Projects, target)
	return s.resetTagProjects(ctx, release, selectedProjects)
}

func (s *Service) resetTagForProject(ctx context.Context, release model.Release, item model.ReleaseProject) (model.ReleaseProject, error) {
	nextTag, err := nextRestartTag(item.BusinessLine, releaseNoFromBatchNo(release.BatchNo), time.Now(), item.TargetTag)
	if err != nil {
		return item, err
	}
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("release_project_id = ?", item.ID).Delete(&model.ReleasePipelineJob{}).Error; err != nil {
			return err
		}
		updates := map[string]interface{}{
			"target_tag":    nextTag,
			"pipeline_id":   "",
			"build_job_id":  "",
			"deploy_job_id": "",
			"status":        model.ProjectStatusPending,
			"error_message": "",
			"source_dirty":  false,
		}
		if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
			return err
		}
		return s.refreshReleaseStatus(tx, release.ID)
	})
	if err != nil {
		return item, err
	}
	item.TargetTag = nextTag
	item.PipelineID = ""
	item.BuildJobID = ""
	item.DeployJobID = ""
	item.Jobs = nil
	item.Status = model.ProjectStatusPending
	item.ErrorMessage = ""
	item.SourceDirty = false
	return item, nil
}

func (s *Service) resetTagProjects(ctx context.Context, release model.Release, selectedProjects []model.ReleaseProject) error {
	releaseNo := releaseNoFromBatchNo(release.BatchNo)
	tagTime := time.Now()

	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, item := range selectedProjects {
			nextTag, err := nextRestartTag(item.BusinessLine, releaseNo, tagTime, item.TargetTag)
			if err != nil {
				return err
			}
			if err := tx.Where("release_project_id = ?", item.ID).Delete(&model.ReleasePipelineJob{}).Error; err != nil {
				return err
			}
			updates := map[string]interface{}{
				"target_tag":    nextTag,
				"pipeline_id":   "",
				"build_job_id":  "",
				"deploy_job_id": "",
				"status":        model.ProjectStatusPending,
				"error_message": "",
				"source_dirty":  false,
			}
			if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", item.ID).Updates(updates).Error; err != nil {
				return err
			}
		}
		return s.refreshReleaseStatus(tx, release.ID)
	})
}

func (s *Service) createTagAndWaitBuild(ctx context.Context, item model.ReleaseProject) error {
	pipeline := gitlab.PipelineResponse{
		ID: item.PipelineID,
	}
	if item.PipelineID == "" {
		if err := s.gitlab.CreateTag(ctx, item.Project.GitLabProjectID, item.TargetTag, item.SourceRef); err != nil {
			_ = s.recordActionFailure(ctx, item.ReleaseID, item.ID, "package", err)
			return err
		}
		found, err := s.findPipelineAfterTag(ctx, item)
		if err != nil {
			_ = s.recordActionFailure(ctx, item.ReleaseID, item.ID, "package", err)
			return err
		}
		pipeline = found
	}

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.syncPipelineJobs(ctx, tx, item, pipeline); err != nil {
			return err
		}
		return s.refreshReleaseStatus(tx, item.ReleaseID)
	}); err != nil {
		return err
	}

	return s.waitForAutoBuild(ctx, item, pipeline.ID)
}

func (s *Service) findPipelineAfterTag(ctx context.Context, item model.ReleaseProject) (gitlab.PipelineResponse, error) {
	var lastErr error
	for attempt := 0; attempt < 5; attempt++ {
		pipeline, err := s.gitlab.FindPipelineByRef(ctx, item.Project.GitLabProjectID, item.TargetTag)
		if err == nil {
			return pipeline, nil
		}
		lastErr = err
		select {
		case <-ctx.Done():
			return gitlab.PipelineResponse{}, ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 500 * time.Millisecond):
		}
	}
	return gitlab.PipelineResponse{}, lastErr
}

func (s *Service) syncPipelineJobs(ctx context.Context, tx *gorm.DB, item model.ReleaseProject, pipeline gitlab.PipelineResponse) error {
	jobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, pipeline.ID)
	if err != nil {
		return err
	}
	return s.updateReleaseProjectFromJobs(tx, item.ID, pipeline.ID, jobs, "")
}

func (s *Service) waitForAutoBuild(ctx context.Context, item model.ReleaseProject, pipelineID string) error {
	for attempt := 0; attempt < autoBuildPollAttempts; attempt++ {
		jobs, err := s.gitlab.ListPipelineJobs(ctx, item.Project.GitLabProjectID, pipelineID)
		if err != nil {
			_ = s.recordActionFailure(ctx, item.ReleaseID, item.ID, "package", err)
			return err
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := s.updateReleaseProjectFromJobs(tx, item.ID, pipelineID, jobs, ""); err != nil {
				return err
			}
			return s.refreshReleaseStatus(tx, item.ReleaseID)
		}); err != nil {
			return err
		}

		switch actionState(jobs, "package") {
		case "failed":
			err := fmt.Errorf("%s 自动构建失败", item.Project.Name)
			_ = s.recordActionFailure(ctx, item.ReleaseID, item.ID, "package", err)
			return err
		case "running":
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(autoBuildPollInterval):
			}
		default:
			return nil
		}
	}

	err := fmt.Errorf("%s 自动构建等待超时", item.Project.Name)
	_ = s.recordActionFailure(ctx, item.ReleaseID, item.ID, "package", err)
	return err
}

func (s *Service) updateReleaseProjectFromJobs(tx *gorm.DB, releaseProjectID uint, pipelineID string, jobs []gitlab.JobResponse, errorMessage string) error {
	if err := s.replacePipelineJobs(tx, releaseProjectID, jobs); err != nil {
		return err
	}
	updates := map[string]interface{}{
		"pipeline_id":   pipelineID,
		"status":        projectStatusFromJobs(jobs),
		"error_message": errorMessage,
	}
	if buildJobID := firstActionJobID(jobs, "package"); buildJobID != "" {
		updates["build_job_id"] = buildJobID
	}
	if deployJobID := firstActionJobID(jobs, "deploy"); deployJobID != "" {
		updates["deploy_job_id"] = deployJobID
	}
	return tx.Model(&model.ReleaseProject{}).Where("id = ?", releaseProjectID).Updates(updates).Error
}

func (s *Service) replacePipelineJobs(tx *gorm.DB, releaseProjectID uint, jobs []gitlab.JobResponse) error {
	if err := tx.Where("release_project_id = ?", releaseProjectID).Delete(&model.ReleasePipelineJob{}).Error; err != nil {
		return err
	}
	for _, job := range jobs {
		item := model.ReleasePipelineJob{
			ReleaseProjectID: releaseProjectID,
			GitLabJobID:      job.ID,
			Name:             job.Name,
			Stage:            job.Stage,
			Status:           job.Status,
			WebURL:           job.WebURL,
			Manual:           job.Manual || job.Status == "manual",
			AllowFailure:     job.AllowFailure,
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *Service) markActionFailed(tx *gorm.DB, releaseProjectID uint, action string, err error) error {
	failedStatus := model.ProjectStatusBuildFailed
	if action == "deploy" {
		failedStatus = model.ProjectStatusDeployFailed
	}
	tx.Model(&model.ReleaseProject{}).Where("id = ?", releaseProjectID).Updates(map[string]interface{}{
		"status":        failedStatus,
		"error_message": err.Error(),
	})
	return err
}

func (s *Service) recordActionFailure(ctx context.Context, releaseID uint, releaseProjectID uint, action string, cause error) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		failedStatus := model.ProjectStatusBuildFailed
		if action == "deploy" {
			failedStatus = model.ProjectStatusDeployFailed
		}
		if err := tx.Model(&model.ReleaseProject{}).Where("id = ?", releaseProjectID).Updates(map[string]interface{}{
			"status":        failedStatus,
			"error_message": cause.Error(),
		}).Error; err != nil {
			return err
		}
		return s.refreshReleaseStatus(tx, releaseID)
	})
}

func filterActionJobs(jobs []gitlab.JobResponse, action string) []gitlab.JobResponse {
	result := []gitlab.JobResponse{}
	for _, job := range jobs {
		if matchesActionJob(job.Stage, job.Name, action) {
			result = append(result, job)
		}
	}
	return result
}

func filterPlayableJobs(jobs []gitlab.JobResponse) []gitlab.JobResponse {
	result := []gitlab.JobResponse{}
	for _, job := range jobs {
		if job.Status == "manual" {
			result = append(result, job)
		}
	}
	return result
}

func latestRetryableJob(jobs []gitlab.JobResponse) (gitlab.JobResponse, bool) {
	retryable := []gitlab.JobResponse{}
	for _, job := range jobs {
		if isRetryableStatus(job.Status) {
			retryable = append(retryable, job)
		}
	}
	if len(retryable) == 0 {
		return gitlab.JobResponse{}, false
	}
	sort.SliceStable(retryable, func(i, j int) bool {
		left, leftErr := strconv.Atoi(retryable[i].ID)
		right, rightErr := strconv.Atoi(retryable[j].ID)
		if leftErr == nil && rightErr == nil {
			return left > right
		}
		return retryable[i].ID > retryable[j].ID
	})
	return retryable[0], true
}

func isRetryableStatus(status string) bool {
	switch status {
	case "failed", "canceled", "success":
		return true
	default:
		return false
	}
}

func firstActionJobID(jobs []gitlab.JobResponse, action string) string {
	for _, job := range jobs {
		if matchesActionJob(job.Stage, job.Name, action) {
			return job.ID
		}
	}
	return ""
}

func projectStatusFromJobs(jobs []gitlab.JobResponse) string {
	switch {
	case actionState(jobs, "deploy") == "failed":
		return model.ProjectStatusDeployFailed
	case actionState(jobs, "package") == "failed":
		return model.ProjectStatusBuildFailed
	case actionState(jobs, "deploy") == "running":
		return model.ProjectStatusDeploying
	case actionState(jobs, "deploy") == "success":
		return model.ProjectStatusDeploySuccess
	case actionState(jobs, "package") == "running":
		return model.ProjectStatusBuilding
	case actionState(jobs, "package") == "success":
		return model.ProjectStatusBuildSuccess
	default:
		return model.ProjectStatusTagged
	}
}

func actionJobKind(action string) string {
	if action == "rebuild" {
		return "package"
	}
	return action
}

func actionState(jobs []gitlab.JobResponse, action string) string {
	actionJobs := filterActionJobs(jobs, action)
	switch {
	case len(actionJobs) == 0:
		return "missing"
	case anyJobStatus(actionJobs, "failed", "canceled"):
		return "failed"
	case anyJobRunning(actionJobs):
		return "running"
	case allJobsSuccessful(actionJobs):
		return "success"
	default:
		return "waiting"
	}
}

func matchesActionJob(stage string, name string, action string) bool {
	value := strings.ToLower(strings.TrimSpace(stage + " " + name))
	if action == "deploy" {
		return strings.Contains(value, "deploy")
	}
	return strings.Contains(value, "build") || strings.Contains(value, "package")
}

func anyJobStatus(jobs []gitlab.JobResponse, statuses ...string) bool {
	allowed := map[string]bool{}
	for _, status := range statuses {
		allowed[status] = true
	}
	for _, job := range jobs {
		if allowed[job.Status] {
			return true
		}
	}
	return false
}

func anyJobRunning(jobs []gitlab.JobResponse) bool {
	return anyJobStatus(jobs, "created", "pending", "preparing", "running", "scheduled", "waiting_for_resource")
}

func allJobsSuccessful(jobs []gitlab.JobResponse) bool {
	if len(jobs) == 0 {
		return false
	}
	for _, job := range jobs {
		if job.Status != "success" && job.Status != "skipped" {
			return false
		}
	}
	return true
}

func shouldRefreshPipelineJobs(item model.ReleaseProject) bool {
	if item.Status == model.ProjectStatusBuilding || item.Status == model.ProjectStatusDeploying {
		return true
	}
	if len(item.Jobs) == 0 {
		return true
	}
	for _, job := range item.Jobs {
		if isRunningStatus(job.Status) {
			return true
		}
	}
	return false
}

func isRunningStatus(status string) bool {
	switch status {
	case "created", "pending", "preparing", "running", "scheduled", "waiting_for_resource":
		return true
	default:
		return false
	}
}

func selectProjectBusinessLine(project model.Project, requestedCode string) (model.BusinessLine, error) {
	lines := projectBusinessLines(project)
	if len(lines) == 0 {
		return model.BusinessLine{}, fmt.Errorf("project %s has no business line configured", project.Code)
	}

	code := strings.TrimSpace(requestedCode)
	if code == "" {
		code = project.BusinessLine.Code
	}
	if code == "" {
		code = lines[0].Code
	}
	for _, line := range lines {
		if line.Code == code {
			return line, nil
		}
	}
	return model.BusinessLine{}, fmt.Errorf("project %s is not associated with business line %s", project.Code, code)
}

func projectBusinessLines(project model.Project) []model.BusinessLine {
	lines := []model.BusinessLine{}
	seen := map[string]bool{}
	add := func(line model.BusinessLine) {
		if line.ID == 0 || strings.TrimSpace(line.Code) == "" || seen[line.Code] {
			return
		}
		seen[line.Code] = true
		lines = append(lines, line)
	}
	add(project.BusinessLine)
	for _, line := range project.BusinessLines {
		add(line)
	}
	return lines
}

func (s *Service) orderProjectsByDependencies(ctx context.Context, projects []model.Project) ([]model.Project, error) {
	if len(projects) <= 1 {
		return projects, nil
	}

	projectByID := map[uint]model.Project{}
	selectedIDs := make([]uint, 0, len(projects))
	for _, project := range projects {
		projectByID[project.ID] = project
		selectedIDs = append(selectedIDs, project.ID)
	}

	var dependencies []model.ProjectDependency
	if err := s.db.WithContext(ctx).
		Where("project_id IN ? AND depends_on_project_id IN ?", selectedIDs, selectedIDs).
		Find(&dependencies).Error; err != nil {
		return nil, err
	}

	inDegree := map[uint]int{}
	nextByDependency := map[uint][]uint{}
	for _, project := range projects {
		inDegree[project.ID] = 0
	}
	for _, dependency := range dependencies {
		if _, ok := projectByID[dependency.ProjectID]; !ok {
			continue
		}
		if _, ok := projectByID[dependency.DependsOnProjectID]; !ok {
			continue
		}
		inDegree[dependency.ProjectID]++
		nextByDependency[dependency.DependsOnProjectID] = append(nextByDependency[dependency.DependsOnProjectID], dependency.ProjectID)
	}

	ready := make([]model.Project, 0, len(projects))
	for _, project := range projects {
		if inDegree[project.ID] == 0 {
			ready = append(ready, project)
		}
	}
	sortProjectsByConfiguredOrder(ready)

	ordered := make([]model.Project, 0, len(projects))
	for len(ready) > 0 {
		project := ready[0]
		ready = ready[1:]
		ordered = append(ordered, project)

		for _, nextID := range nextByDependency[project.ID] {
			inDegree[nextID]--
			if inDegree[nextID] == 0 {
				ready = append(ready, projectByID[nextID])
				sortProjectsByConfiguredOrder(ready)
			}
		}
	}
	if len(ordered) != len(projects) {
		return nil, fmt.Errorf("项目依赖关系存在循环，无法生成上线顺序")
	}
	return ordered, nil
}

func sortProjectsByConfiguredOrder(projects []model.Project) {
	sort.SliceStable(projects, func(i, j int) bool {
		if projects[i].SortOrder == projects[j].SortOrder {
			return projects[i].Code < projects[j].Code
		}
		return projects[i].SortOrder < projects[j].SortOrder
	})
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

func nextRestartTag(line model.BusinessLine, releaseNo string, tagTime time.Time, currentTag string) (string, error) {
	for offset := 0; offset < 10; offset++ {
		tag := renderTagAt(line, releaseNo, tagTime.Add(time.Duration(offset)*time.Second))
		if tag != "" && tag != currentTag {
			return tag, nil
		}
	}
	return "", fmt.Errorf("无法为业务线 %s 生成新的 tag，请确认 tag 模板包含 {timestamp}、{datetime} 或 {date}", line.Code)
}

func releaseNoFromBatchNo(batchNo string) string {
	parts := strings.Split(strings.TrimSpace(batchNo), "-")
	if len(parts) == 0 {
		return "001"
	}
	releaseNo := strings.TrimSpace(parts[len(parts)-1])
	if releaseNo == "" {
		return "001"
	}
	return releaseNo
}

func findReleaseProject(release model.Release, releaseProjectID uint) (model.ReleaseProject, error) {
	for _, item := range release.Projects {
		if item.ID == releaseProjectID {
			return item, nil
		}
	}
	return model.ReleaseProject{}, fmt.Errorf("release project not found")
}

func normalizeSource(sourceType string, sourceRef string) (string, string, error) {
	sourceType = strings.TrimSpace(sourceType)
	sourceRef = strings.TrimSpace(sourceRef)
	if sourceRef == "" {
		return "", "", fmt.Errorf("来源分支、tag 或 commit 不能为空")
	}
	switch sourceType {
	case "branch", "tag", "commit":
		return sourceType, sourceRef, nil
	default:
		return "", "", fmt.Errorf("来源类型只能是 branch、tag 或 commit")
	}
}

func sourceLabel(sourceType string) string {
	switch sourceType {
	case "tag":
		return "Tag"
	case "commit":
		return "Commit"
	default:
		return "分支"
	}
}

func normalizeTarget(target string) string {
	target = strings.TrimSpace(target)
	switch target {
	case model.ProjectKindBackend, model.ProjectKindFrontend:
		return target
	default:
		return "all"
	}
}

func targetLabel(target string) string {
	switch target {
	case model.ProjectKindBackend:
		return "后端"
	case model.ProjectKindFrontend:
		return "前端"
	default:
		return "全量"
	}
}

func filterByTarget(projects []model.ReleaseProject, target string) []model.ReleaseProject {
	target = normalizeTarget(target)
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
	if action == "rebuild" {
		return "重新构建"
	}
	return "构建"
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
		sort.Slice(release.Projects[i].Jobs, func(j, k int) bool {
			left := release.Projects[i].Jobs[j]
			right := release.Projects[i].Jobs[k]
			if left.Stage == right.Stage {
				return left.Name < right.Name
			}
			return left.Stage < right.Stage
		})
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
