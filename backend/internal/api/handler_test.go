package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"delivery-platform/backend/internal/api"
	"delivery-platform/backend/internal/bootstrap"
	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/model"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

func TestProjectsEndpointReturnsArrayDependencies(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if bytes.Contains(recorder.Body.Bytes(), []byte(`"dependencies":null`)) {
		t.Fatalf("dependencies must be encoded as [] rather than null: %s", recorder.Body.String())
	}

	var payload []map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if len(payload) == 0 {
		t.Fatal("expected seeded projects")
	}
	for _, project := range payload {
		dependencies, ok := project["dependencies"].([]any)
		if !ok {
			t.Fatalf("project %v dependencies type = %T, want JSON array", project["code"], project["dependencies"])
		}
		if project["code"] == "base-auth" && len(dependencies) != 0 {
			t.Fatalf("base-auth dependencies = %v, want empty array", dependencies)
		}
	}
}

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(
		&model.User{},
		&model.BusinessLine{},
		&model.Project{},
		&model.ProjectDependency{},
		&model.Release{},
		&model.ReleaseProject{},
		&model.ReleaseEvent{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := bootstrap.Seed(db); err != nil {
		t.Fatalf("seed: %v", err)
	}

	cfg := config.Config{
		Env:          "test",
		JWTSecret:    "test-secret",
		CORSAllowAll: true,
	}
	return api.NewRouter(cfg, db, nil)
}

func login(t *testing.T, router *gin.Engine) string {
	t.Helper()
	body := bytes.NewBufferString(`{"username":"admin","password":"admin123"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/login status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var payload struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("decode login response: %v", err)
	}
	if payload.Token == "" {
		t.Fatal("login response token is empty")
	}
	return payload.Token
}
