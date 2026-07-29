package database

import (
	"strings"
	"testing"

	"delivery-platform/backend/internal/model"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestMigrateDropsLegacyProjectJobColumns(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file:"+strings.ReplaceAll(t.Name(), "/", "_")+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.Exec(`
		CREATE TABLE projects (
			id integer primary key autoincrement,
			code text not null,
			name text not null,
			kind text not null,
			owner text not null,
			business_line_id integer not null,
			git_lab_url text not null,
			git_lab_project_id text not null,
			default_branch text not null,
			sort_order integer not null,
			enabled numeric,
			package_job text not null,
			deploy_job text not null,
			created_at datetime,
			updated_at datetime
		)
	`).Error; err != nil {
		t.Fatalf("create legacy projects table: %v", err)
	}

	if err := Migrate(db); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	for _, column := range []string{"package_job", "deploy_job"} {
		if db.Migrator().HasColumn(&model.Project{}, column) {
			t.Fatalf("legacy column %s still exists", column)
		}
	}

	line := model.BusinessLine{
		Code:        "ops",
		Name:        "OPS 平台业务线",
		Platform:    "OPSPRD",
		TagPrefix:   "opsprd",
		TagTemplate: "{prefix}-{timestamp}-{releaseNo}",
		Approver:    "平台 SRE",
	}
	if err := db.Create(&line).Error; err != nil {
		t.Fatalf("create business line: %v", err)
	}
	project := model.Project{
		Code:            "search-api",
		Name:            "搜索服务",
		Kind:            model.ProjectKindBackend,
		Owner:           "搜索组",
		BusinessLineID:  line.ID,
		GitLabURL:       "https://gitlab.corp/delivery/search-api",
		GitLabProjectID: "delivery/search-api",
		DefaultBranch:   "master",
		SortOrder:       10,
		Enabled:         true,
	}
	if err := db.Create(&project).Error; err != nil {
		t.Fatalf("create project after migration: %v", err)
	}
}
