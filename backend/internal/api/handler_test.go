package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
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

	recorder := authedRequest(t, router, token, http.MethodGet, "/api/projects", "")

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

func TestProjectConfigCanCreateUpdateAndDeleteProject(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	body := `{
		"code":"search-api",
		"name":"搜索服务",
		"kind":"backend",
		"owner":"搜索组",
		"businessLineCode":"ops",
		"gitlabUrl":"https://gitlab.corp/delivery/search-api",
		"gitlabProjectId":"delivery/search-api",
		"defaultBranch":"master",
		"packageJob":"build-search-prd",
		"deployJob":"deploy-search-prd",
		"sortOrder":35,
		"enabled":true
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/projects", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/projects status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectName(t, recorder.Body.Bytes(), "search-api", "搜索服务")

	updateBody := strings.Replace(body, "搜索服务", "搜索服务 V2", 1)
	recorder = authedRequest(t, router, token, http.MethodPut, "/api/projects/search-api", updateBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/projects/search-api status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectName(t, recorder.Body.Bytes(), "search-api", "搜索服务 V2")

	recorder = authedRequest(t, router, token, http.MethodDelete, "/api/projects/search-api", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/projects/search-api status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectMissing(t, recorder.Body.Bytes(), "search-api")
}

func TestBusinessLineConfigCanCreateAndDelete(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	body := `{
		"code":"cc",
		"name":"CC 新业务线",
		"platform":"CCPRD",
		"tagPrefix":"ccprd",
		"tagTemplate":"{prefix}-{timestamp}-{releaseNo}",
		"approver":"CC 发布经理"
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/business-lines", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/business-lines status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertBusinessLineName(t, recorder.Body.Bytes(), "cc", "CC 新业务线")

	recorder = authedRequest(t, router, token, http.MethodDelete, "/api/business-lines/cc", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/business-lines/cc status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertBusinessLineMissing(t, recorder.Body.Bytes(), "cc")
}

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbName := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(sqlite.Open("file:"+dbName+"?mode=memory&cache=shared"), &gorm.Config{})
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

func authedRequest(t *testing.T, router *gin.Engine, token string, method string, path string, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
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

func assertProjectName(t *testing.T, body []byte, code string, name string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	for _, project := range payload {
		if project["code"] == code {
			if project["name"] != name {
				t.Fatalf("project %s name = %v, want %s", code, project["name"], name)
			}
			return
		}
	}
	t.Fatalf("project %s not found in response: %s", code, string(body))
}

func assertProjectMissing(t *testing.T, body []byte, code string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	for _, project := range payload {
		if project["code"] == code {
			t.Fatalf("project %s should be absent after deletion", code)
		}
	}
}

func assertBusinessLineName(t *testing.T, body []byte, code string, name string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode business lines response: %v", err)
	}
	for _, line := range payload {
		if line["code"] == code {
			if line["name"] != name {
				t.Fatalf("business line %s name = %v, want %s", code, line["name"], name)
			}
			return
		}
	}
	t.Fatalf("business line %s not found in response: %s", code, string(body))
}

func assertBusinessLineMissing(t *testing.T, body []byte, code string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode business lines response: %v", err)
	}
	for _, line := range payload {
		if line["code"] == code {
			t.Fatalf("business line %s should be absent after deletion", code)
		}
	}
}
