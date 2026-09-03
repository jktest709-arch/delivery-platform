package release

import (
	"context"
	"strings"
	"testing"
	"time"

	"delivery-platform/backend/internal/bootstrap"
	"delivery-platform/backend/internal/database"
	"delivery-platform/backend/internal/gitlab"
	"delivery-platform/backend/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestRenderTagUsesSecondPrecisionTimestamp(t *testing.T) {
	line := model.BusinessLine{
		TagPrefix:   "aaprd",
		TagTemplate: "{prefix}-{timestamp}-{releaseNo}",
	}
	tag := renderTagAt(line, "042", time.Date(2026, 7, 29, 15, 4, 5, 0, time.UTC))

	if tag != "aaprd-20260729150405-042" {
		t.Fatalf("tag = %s, want aaprd-20260729150405-042", tag)
	}
}

func TestRenderTagKeepsOldDatePlaceholderSecondPrecision(t *testing.T) {
	line := model.BusinessLine{
		TagPrefix:   "bbprd",
		TagTemplate: "{prefix}-{date}-{releaseNo}",
	}
	tag := renderTagAt(line, "007", time.Date(2026, 7, 29, 9, 8, 7, 0, time.UTC))

	if tag != "bbprd-20260729090807-007" {
		t.Fatalf("tag = %s, want bbprd-20260729090807-007", tag)
	}
}

func TestReleaseOperationLockRequiresOwnerToken(t *testing.T) {
	db := newServiceTestDB(t)
	service := NewService(db, nil)
	lock, err := service.acquireReleaseOperationLock(context.Background(), 999, "tag_resume", "all", 1)
	if err != nil {
		t.Fatalf("acquire operation lock: %v", err)
	}

	var row model.ReleaseOperationLock
	if err := db.First(&row, 999).Error; err != nil {
		t.Fatalf("read operation lock: %v", err)
	}
	previousExpiry := row.ExpiresAt
	(&operationLock{db: db, releaseID: 999, token: "not-the-owner"}).Unlock()
	if err := db.First(&row, 999).Error; err != nil {
		t.Fatalf("wrong owner removed operation lock: %v", err)
	}
	if err := lock.Renew(context.Background()); err != nil {
		t.Fatalf("renew operation lock: %v", err)
	}
	if err := db.First(&row, 999).Error; err != nil {
		t.Fatalf("read renewed operation lock: %v", err)
	}
	if !row.ExpiresAt.After(previousExpiry) {
		t.Fatal("operation lock expiry was not renewed")
	}
	lock.Unlock()
	if err := db.First(&row, 999).Error; err == nil {
		t.Fatal("operation lock still exists after owner released it")
	}
}

func TestTagSyncsPipelineJobsAndPackagePlaysBuildJob(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "manual", WebURL: "https://gitlab/jobs/201", Manual: true},
			{ID: "301", Name: "deploy-prod", Stage: "deploy", Status: "manual", WebURL: "https://gitlab/jobs/301", Manual: true},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)

	tagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	project := tagged.Projects[0]
	if project.PipelineID != "101" {
		t.Fatalf("pipelineID = %s, want 101", project.PipelineID)
	}
	if project.BuildJobID != "201" || project.DeployJobID != "301" {
		t.Fatalf("job ids = %s/%s, want 201/301", project.BuildJobID, project.DeployJobID)
	}
	if len(project.Jobs) != 2 {
		t.Fatalf("jobs len = %d, want 2", len(project.Jobs))
	}

	updated, err := service.PackageOne(context.Background(), tagged.ID, project.ID, applicant)
	if err != nil {
		t.Fatalf("package one: %v", err)
	}
	if len(client.played) != 1 || client.played[0] != "201" {
		t.Fatalf("played jobs = %v, want [201]", client.played)
	}
	if updated.Projects[0].Status != model.ProjectStatusBuilding {
		t.Fatalf("project status = %s, want %s", updated.Projects[0].Status, model.ProjectStatusBuilding)
	}
}

func TestReleaseOrdersSelectedProjectsByDependencies(t *testing.T) {
	db := newServiceTestDB(t)
	if err := db.Model(&model.Project{}).Where("code = ?", "base-auth").Update("sort_order", 90).Error; err != nil {
		t.Fatalf("update base-auth order: %v", err)
	}
	if err := db.Model(&model.Project{}).Where("code = ?", "order-core").Update("sort_order", 10).Error; err != nil {
		t.Fatalf("update order-core order: %v", err)
	}
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}

	service := NewService(db, &fakeGitLabClient{})
	created, err := service.Create(context.Background(), CreateRequest{
		ReleaseWindow: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "order-core", BusinessLineCode: "aa", SourceType: "branch", SourceRef: "master"},
			{ProjectCode: "base-auth", BusinessLineCode: "ops", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}

	if created.Projects[0].Project.Code != "base-auth" || created.Projects[1].Project.Code != "order-core" {
		t.Fatalf("release order = %s, %s; want base-auth, order-core", created.Projects[0].Project.Code, created.Projects[1].Project.Code)
	}
}

func TestCreateTagsRequiresApproval(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}

	_, err = service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err == nil || !strings.Contains(err.Error(), "未审批") {
		t.Fatalf("CreateTags err = %v, want approval gate error", err)
	}
	if len(client.taggedProjects) != 0 {
		t.Fatalf("tagged projects = %v, want none before approval", client.taggedProjects)
	}
}

func TestDeployRequiresBuildSuccess(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"},
			{ID: "301", Name: "deploy-prod", Stage: "deploy", Status: "manual", WebURL: "https://gitlab/jobs/301", Manual: true},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)
	projectID := created.Projects[0].ID
	if err := db.Model(&model.ReleaseProject{}).Where("id = ?", projectID).Updates(map[string]interface{}{
		"pipeline_id":   "101",
		"build_job_id":  "201",
		"deploy_job_id": "301",
	}).Error; err != nil {
		t.Fatalf("seed release project pipeline ids: %v", err)
	}
	if err := db.Create(&model.ReleasePipelineJob{
		ReleaseProjectID: projectID,
		GitLabJobID:      "301",
		Name:             "deploy-prod",
		Stage:            "deploy",
		Status:           "manual",
		Manual:           true,
	}).Error; err != nil {
		t.Fatalf("seed deploy job: %v", err)
	}

	if _, err := service.Deploy(context.Background(), created.ID, "all", applicant); err == nil || !strings.Contains(err.Error(), "构建未完成") {
		t.Fatalf("Deploy err = %v, want build gate error", err)
	}
	if len(client.played) != 0 {
		t.Fatalf("played deploy jobs = %v, want none before build success", client.played)
	}

	if err := db.Model(&model.ReleaseProject{}).
		Where("id = ?", projectID).
		Update("status", model.ProjectStatusBuildSuccess).Error; err != nil {
		t.Fatalf("mark build success: %v", err)
	}
	updated, err := service.Deploy(context.Background(), created.ID, "all", applicant)
	if err != nil {
		t.Fatalf("deploy after build success: %v", err)
	}
	if len(client.played) != 1 || client.played[0] != "301" {
		t.Fatalf("played deploy jobs = %v, want [301]", client.played)
	}
	if updated.Projects[0].Status != model.ProjectStatusDeploying {
		t.Fatalf("project status = %s, want deploying", updated.Projects[0].Status)
	}
}

func TestCreateTagsWaitsForAutoBuildBeforeNextProject(t *testing.T) {
	oldInterval := autoBuildPollInterval
	oldAttempts := autoBuildPollAttempts
	autoBuildPollInterval = time.Millisecond
	autoBuildPollAttempts = 5
	defer func() {
		autoBuildPollInterval = oldInterval
		autoBuildPollAttempts = oldAttempts
	}()

	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobsByProject: map[string][][]gitlab.JobResponse{
			"delivery/base-auth": {
				{{ID: "201", Name: "build-image", Stage: "build", Status: "running", WebURL: "https://gitlab/jobs/201"}},
				{{ID: "201", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"}},
			},
			"delivery/order-core": {
				{{ID: "202", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/202"}},
			},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		ReleaseWindow: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", BusinessLineCode: "ops", SourceType: "branch", SourceRef: "master"},
			{ProjectCode: "order-core", BusinessLineCode: "aa", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)

	tagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	if len(client.taggedProjects) != 2 || client.taggedProjects[0] != "delivery/base-auth" || client.taggedProjects[1] != "delivery/order-core" {
		t.Fatalf("tagged projects = %v, want dependency order", client.taggedProjects)
	}
	if client.listCalls["delivery/base-auth"] < 2 {
		t.Fatalf("base-auth list calls = %d, want at least 2 to wait for build completion", client.listCalls["delivery/base-auth"])
	}
	for _, project := range tagged.Projects {
		if project.Status != model.ProjectStatusBuildSuccess {
			t.Fatalf("%s status = %s, want build_success", project.Project.Code, project.Status)
		}
	}
}

func TestCreateTagsCanTargetFrontendProjectsOnly(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobsByProject: map[string][][]gitlab.JobResponse{
			"delivery/reporting": {
				{{ID: "701", Name: "build-web", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/701"}},
			},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		ReleaseWindow: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", BusinessLineCode: "ops", SourceType: "branch", SourceRef: "master"},
			{ProjectCode: "reporting", BusinessLineCode: "ops", SourceType: "branch", SourceRef: "main"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)

	updated, err := service.CreateTags(context.Background(), created.ID, model.ProjectKindFrontend, "resume", applicant)
	if err != nil {
		t.Fatalf("create frontend tags: %v", err)
	}
	if len(client.taggedProjects) != 1 || client.taggedProjects[0] != "delivery/reporting" {
		t.Fatalf("tagged projects = %v, want frontend project only", client.taggedProjects)
	}
	for _, project := range updated.Projects {
		if project.Project.Code == "base-auth" && project.Status != model.ProjectStatusPending {
			t.Fatalf("base-auth status = %s, want pending", project.Status)
		}
		if project.Project.Code == "reporting" && project.Status != model.ProjectStatusBuildSuccess {
			t.Fatalf("reporting status = %s, want build_success", project.Status)
		}
	}
}

func TestCreateTagsResumeRetriesFailedProjectThenContinues(t *testing.T) {
	oldInterval := autoBuildPollInterval
	oldAttempts := autoBuildPollAttempts
	autoBuildPollInterval = time.Millisecond
	autoBuildPollAttempts = 5
	defer func() {
		autoBuildPollInterval = oldInterval
		autoBuildPollAttempts = oldAttempts
	}()

	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobsByProject: map[string][][]gitlab.JobResponse{
			"delivery/base-auth": {
				{{ID: "201", Name: "build-image", Stage: "build", Status: "failed", WebURL: "https://gitlab/jobs/201"}},
				{{ID: "201", Name: "build-image", Stage: "build", Status: "failed", WebURL: "https://gitlab/jobs/201"}},
				{{ID: "201", Name: "build-image", Stage: "build", Status: "failed", WebURL: "https://gitlab/jobs/201"}},
				{{ID: "202", Name: "build-image", Stage: "build", Status: "running", WebURL: "https://gitlab/jobs/202"}},
				{{ID: "202", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/202"}},
			},
			"delivery/order-core": {
				{{ID: "301", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/301"}},
			},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		ReleaseWindow: time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", BusinessLineCode: "ops", SourceType: "branch", SourceRef: "master"},
			{ProjectCode: "order-core", BusinessLineCode: "aa", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)

	if _, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant); err == nil {
		t.Fatal("create tags succeeded unexpectedly with failed base-auth build")
	}
	if len(client.taggedProjects) != 1 || client.taggedProjects[0] != "delivery/base-auth" {
		t.Fatalf("first tagged projects = %v, want base-auth only", client.taggedProjects)
	}

	updated, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("resume tags: %v", err)
	}
	if len(client.retried) != 1 || client.retried[0] != "201" {
		t.Fatalf("retried jobs = %v, want [201]", client.retried)
	}
	if len(client.taggedProjects) != 2 || client.taggedProjects[1] != "delivery/order-core" {
		t.Fatalf("tagged projects after resume = %v, want order-core continuation", client.taggedProjects)
	}
	for _, project := range updated.Projects {
		if project.Status != model.ProjectStatusBuildSuccess {
			t.Fatalf("%s status = %s, want build_success", project.Project.Code, project.Status)
		}
	}
}

func TestCreateTagsRestartGeneratesNewTag(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)
	first, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	firstTag := first.Projects[0].TargetTag

	restarted, err := service.CreateTags(context.Background(), created.ID, "all", "restart", applicant)
	if err != nil {
		t.Fatalf("restart tags: %v", err)
	}
	if len(client.taggedNames) != 2 {
		t.Fatalf("tagged names = %v, want two tag creations", client.taggedNames)
	}
	if restarted.Projects[0].TargetTag == firstTag {
		t.Fatalf("restart target tag = %s, want a new tag", restarted.Projects[0].TargetTag)
	}
	if client.taggedNames[0] == client.taggedNames[1] {
		t.Fatalf("created tag names are equal: %v", client.taggedNames)
	}
}

func TestUpdateProjectSourceMakesBatchTagUseLatestRef(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)
	first, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	firstProject := first.Projects[0]
	firstTag := firstProject.TargetTag

	updated, err := service.UpdateProjectSource(context.Background(), first.ID, firstProject.ID, "commit", "abc123", applicant)
	if err != nil {
		t.Fatalf("update source: %v", err)
	}
	if !updated.Projects[0].SourceDirty {
		t.Fatal("sourceDirty = false, want true after source update")
	}
	if updated.Projects[0].PipelineID == "" {
		t.Fatal("pipelineID was cleared after source update; old pipeline should remain retryable")
	}

	retagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("resume after source update: %v", err)
	}
	if len(client.taggedRefs) != 2 || client.taggedRefs[1] != "abc123" {
		t.Fatalf("tagged refs = %v, want second ref abc123", client.taggedRefs)
	}
	if retagged.Projects[0].TargetTag == firstTag {
		t.Fatalf("target tag = %s, want new tag after source update", retagged.Projects[0].TargetTag)
	}
	if retagged.Projects[0].SourceDirty {
		t.Fatal("sourceDirty = true after retag, want false")
	}
}

func TestTagOneUsesLatestProjectSource(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)
	tagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	project := tagged.Projects[0]
	if _, err := service.UpdateProjectSource(context.Background(), tagged.ID, project.ID, "branch", "hotfix/source", applicant); err != nil {
		t.Fatalf("update source: %v", err)
	}

	retagged, err := service.TagOne(context.Background(), tagged.ID, project.ID, applicant)
	if err != nil {
		t.Fatalf("tag one: %v", err)
	}
	if len(client.taggedRefs) != 2 || client.taggedRefs[1] != "hotfix/source" {
		t.Fatalf("tagged refs = %v, want second ref hotfix/source", client.taggedRefs)
	}
	if retagged.Projects[0].SourceDirty {
		t.Fatal("sourceDirty = true after tag one, want false")
	}
}

func TestBuildOnlyProjectDoesNotRequireDeployJob(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-lib", Stage: "build", Status: "success", WebURL: "https://gitlab/jobs/201"},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)

	tagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err != nil {
		t.Fatalf("create tags: %v", err)
	}
	project := tagged.Projects[0]
	if project.Status != model.ProjectStatusBuildSuccess {
		t.Fatalf("project status = %s, want build_success", project.Status)
	}
	if project.DeployJobID != "" {
		t.Fatalf("deploy job id = %s, want empty", project.DeployJobID)
	}
}

func TestPackageOneRetriesLatestBuildJob(t *testing.T) {
	db := newServiceTestDB(t)
	var applicant model.User
	if err := db.Where("username = ?", "admin").First(&applicant).Error; err != nil {
		t.Fatalf("find applicant: %v", err)
	}
	client := &fakeGitLabClient{
		jobs: []gitlab.JobResponse{
			{ID: "201", Name: "build-image", Stage: "build", Status: "failed", WebURL: "https://gitlab/jobs/201"},
			{ID: "301", Name: "deploy-prod", Stage: "deploy", Status: "manual", WebURL: "https://gitlab/jobs/301", Manual: true},
		},
	}
	service := NewService(db, client)

	created, err := service.Create(context.Background(), CreateRequest{
		BusinessLineCode: "ops",
		ReleaseWindow:    time.Date(2026, 7, 29, 10, 0, 0, 0, time.UTC),
		Projects: []CreateProjectRequest{
			{ProjectCode: "base-auth", SourceType: "branch", SourceRef: "master"},
		},
	}, applicant)
	if err != nil {
		t.Fatalf("create release: %v", err)
	}
	created = approveReleaseForTest(t, service, created, applicant)
	tagged, err := service.CreateTags(context.Background(), created.ID, "all", "resume", applicant)
	if err == nil {
		t.Fatal("create tags succeeded unexpectedly with failed auto build")
	}
	if len(client.retried) != 0 {
		t.Fatalf("retried during tag = %v, want none", client.retried)
	}

	updated, err := service.PackageOne(context.Background(), tagged.ID, tagged.Projects[0].ID, applicant)
	if err != nil {
		t.Fatalf("package one retry: %v", err)
	}
	if len(client.retried) != 1 || client.retried[0] != "201" {
		t.Fatalf("retried jobs = %v, want [201]", client.retried)
	}
	if updated.Projects[0].Status != model.ProjectStatusBuilding {
		t.Fatalf("project status = %s, want building", updated.Projects[0].Status)
	}
}

func approveReleaseForTest(t *testing.T, service *Service, release model.Release, operator model.User) model.Release {
	t.Helper()
	var approver model.User
	if err := service.db.Where("id <> ? AND role IN ?", operator.ID, []string{model.RoleReleaseManager, model.RoleAdmin}).First(&approver).Error; err != nil {
		t.Fatalf("find independent approver: %v", err)
	}
	approved, err := service.Approve(context.Background(), release.ID, approver)
	if err != nil {
		t.Fatalf("approve release: %v", err)
	}
	if approved.Status != model.ReleaseStatusApproved {
		t.Fatalf("approved status = %s, want %s", approved.Status, model.ReleaseStatusApproved)
	}
	if approved.ApproverID == nil {
		t.Fatal("approver id is nil after approval")
	}
	return approved
}

func newServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := bootstrap.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

type fakeGitLabClient struct {
	jobs           []gitlab.JobResponse
	jobsByProject  map[string][][]gitlab.JobResponse
	listCalls      map[string]int
	played         []string
	retried        []string
	taggedProjects []string
	taggedNames    []string
	taggedRefs     []string
	trace          string
}

func (f *fakeGitLabClient) CreateTag(_ context.Context, projectID string, tagName string, ref string) error {
	f.taggedProjects = append(f.taggedProjects, projectID)
	f.taggedNames = append(f.taggedNames, tagName)
	f.taggedRefs = append(f.taggedRefs, ref)
	return nil
}

func (f *fakeGitLabClient) FindPipelineByRef(context.Context, string, string) (gitlab.PipelineResponse, error) {
	return gitlab.PipelineResponse{ID: "101", WebURL: "https://gitlab/pipelines/101"}, nil
}

func (f *fakeGitLabClient) ListPipelineJobs(_ context.Context, projectID string, _ string) ([]gitlab.JobResponse, error) {
	if f.listCalls == nil {
		f.listCalls = map[string]int{}
	}
	call := f.listCalls[projectID]
	f.listCalls[projectID] = call + 1
	if sequences := f.jobsByProject[projectID]; len(sequences) > 0 {
		index := call
		if index >= len(sequences) {
			index = len(sequences) - 1
		}
		return cloneJobs(sequences[index]), nil
	}
	return append([]gitlab.JobResponse(nil), f.jobs...), nil
}

func (f *fakeGitLabClient) PlayJob(_ context.Context, _ string, jobID string) (gitlab.JobResponse, error) {
	f.played = append(f.played, jobID)
	for index := range f.jobs {
		if f.jobs[index].ID == jobID {
			f.jobs[index].Status = "running"
			f.jobs[index].Manual = false
			return f.jobs[index], nil
		}
	}
	return gitlab.JobResponse{ID: jobID, Status: "running"}, nil
}

func (f *fakeGitLabClient) RetryJob(_ context.Context, _ string, jobID string) (gitlab.JobResponse, error) {
	f.retried = append(f.retried, jobID)
	for index := range f.jobs {
		if f.jobs[index].ID == jobID {
			f.jobs[index].ID = jobID + "1"
			f.jobs[index].Status = "running"
			f.jobs[index].Manual = false
			return f.jobs[index], nil
		}
	}
	return gitlab.JobResponse{ID: jobID, Status: "running"}, nil
}

func (f *fakeGitLabClient) GetJobTrace(context.Context, string, string) (string, error) {
	if f.trace != "" {
		return f.trace, nil
	}
	return "fake trace\n", nil
}

func cloneJobs(jobs []gitlab.JobResponse) []gitlab.JobResponse {
	return append([]gitlab.JobResponse(nil), jobs...)
}
