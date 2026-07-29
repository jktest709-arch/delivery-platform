package release

import (
	"context"
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

	tagged, err := service.CreateTags(context.Background(), created.ID, applicant)
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

	tagged, err := service.CreateTags(context.Background(), created.ID, applicant)
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

	tagged, err := service.CreateTags(context.Background(), created.ID, applicant)
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
	taggedProjects []string
}

func (f *fakeGitLabClient) CreateTag(_ context.Context, projectID string, _ string, _ string) error {
	f.taggedProjects = append(f.taggedProjects, projectID)
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

func cloneJobs(jobs []gitlab.JobResponse) []gitlab.JobResponse {
	return append([]gitlab.JobResponse(nil), jobs...)
}
