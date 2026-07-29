package release

import (
	"context"
	"testing"
	"time"

	"delivery-platform/backend/internal/bootstrap"
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

func newServiceTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.BusinessLine{},
		&model.Project{},
		&model.ProjectBusinessLine{},
		&model.ProjectDependency{},
		&model.Release{},
		&model.ReleaseProject{},
		&model.ReleasePipelineJob{},
		&model.ReleaseEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := bootstrap.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return db
}

type fakeGitLabClient struct {
	jobs   []gitlab.JobResponse
	played []string
}

func (f *fakeGitLabClient) CreateTag(context.Context, string, string, string) error {
	return nil
}

func (f *fakeGitLabClient) FindPipelineByRef(context.Context, string, string) (gitlab.PipelineResponse, error) {
	return gitlab.PipelineResponse{ID: "101", WebURL: "https://gitlab/pipelines/101"}, nil
}

func (f *fakeGitLabClient) ListPipelineJobs(context.Context, string, string) ([]gitlab.JobResponse, error) {
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
