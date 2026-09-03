package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"delivery-platform/backend/internal/api"
	"delivery-platform/backend/internal/bootstrap"
	"delivery-platform/backend/internal/config"
	"delivery-platform/backend/internal/database"
	"delivery-platform/backend/internal/release"
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
		"businessLineCodes":["ops","aa"],
		"gitlabUrl":"https://gitlab.corp/delivery/search-api",
		"gitlabProjectId":"delivery/search-api",
		"defaultBranch":"master",
		"sortOrder":35,
		"enabled":true
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/projects", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/projects status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectName(t, recorder.Body.Bytes(), "search-api", "搜索服务")
	assertProjectBusinessLineCodes(t, recorder.Body.Bytes(), "search-api", []string{"ops", "aa"})

	updateBody := strings.Replace(body, "搜索服务", "搜索服务 V2", 1)
	recorder = authedRequest(t, router, token, http.MethodPut, "/api/projects/search-api", updateBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/projects/search-api status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectName(t, recorder.Body.Bytes(), "search-api", "搜索服务 V2")
	assertProjectBusinessLineCodes(t, recorder.Body.Bytes(), "search-api", []string{"ops", "aa"})

	recorder = authedRequest(t, router, token, http.MethodDelete, "/api/projects/search-api", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/projects/search-api status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectMissing(t, recorder.Body.Bytes(), "search-api")
}

func TestProjectOrderCanBeUpdated(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	recorder := authedRequest(t, router, token, http.MethodGet, "/api/projects", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	codes := projectCodesFromResponse(t, recorder.Body.Bytes())
	if len(codes) < 2 {
		t.Fatalf("expected at least two seeded projects, got %v", codes)
	}
	codes[0], codes[1] = codes[1], codes[0]

	body, err := json.Marshal(struct {
		Codes []string `json:"codes"`
	}{Codes: codes})
	if err != nil {
		t.Fatalf("encode project order request: %v", err)
	}
	recorder = authedRequest(t, router, token, http.MethodPut, "/api/projects/order", string(body))
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/projects/order status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectOrder(t, recorder.Body.Bytes(), codes)

	recorder = authedRequest(t, router, token, http.MethodGet, "/api/projects", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/projects after order update status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectOrder(t, recorder.Body.Bytes(), codes)
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

func TestBusinessLineDeleteCanMigrateUsedProjects(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	recorder := authedRequest(t, router, token, http.MethodDelete, "/api/business-lines/ops?replacementCode=aa", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/business-lines/ops status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertBusinessLineMissing(t, recorder.Body.Bytes(), "ops")

	recorder = authedRequest(t, router, token, http.MethodGet, "/api/projects", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/projects status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertProjectBusinessLine(t, recorder.Body.Bytes(), "base-auth", "aa")
	assertProjectBusinessLine(t, recorder.Body.Bytes(), "reporting", "aa")
	assertProjectBusinessLineCodes(t, recorder.Body.Bytes(), "base-auth", []string{"aa"})
	assertProjectBusinessLineCodes(t, recorder.Body.Bytes(), "reporting", []string{"aa"})
}

func TestReleaseUsesSelectedBusinessLineForTag(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	projectBody := `{
		"code":"base-auth",
		"name":"统一认证中心",
		"kind":"backend",
		"owner":"平台组",
		"businessLineCode":"ops",
		"businessLineCodes":["ops","aa"],
		"gitlabUrl":"https://gitlab.corp/delivery/base-auth",
		"gitlabProjectId":"delivery/base-auth",
		"defaultBranch":"master",
		"sortOrder":10,
		"enabled":true
	}`
	recorder := authedRequest(t, router, token, http.MethodPut, "/api/projects/base-auth", projectBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/projects/base-auth status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	releaseBody := `{
		"businessLineCode":"aa",
		"releaseWindow":"2026-07-29T10:00:00Z",
		"remark":"multi line tag",
		"projects":[{"projectCode":"base-auth","sourceType":"branch","sourceRef":"master"}]
	}`
	recorder = authedRequest(t, router, token, http.MethodPost, "/api/releases", releaseBody)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /api/releases status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	assertReleaseBusinessLine(t, recorder.Body.Bytes(), "aa")
	assertReleaseProjectTagPrefix(t, recorder.Body.Bytes(), "base-auth", "aaprd-")
	assertReleaseProjectBusinessLine(t, recorder.Body.Bytes(), "base-auth", "aa")
}

func TestUserManagementCanCreateUpdateAndDeleteUser(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	body := `{
		"username":"qa",
		"displayName":"测试同学",
		"role":"developer",
		"status":"enabled",
		"password":"qa1234567890"
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/users", body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/users status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	userID := assertUserRole(t, recorder.Body.Bytes(), "qa", "developer")

	updateBody := `{
		"username":"qa",
		"displayName":"测试同学",
		"role":"release_manager",
		"status":"disabled",
		"password":""
	}`
	recorder = authedRequest(t, router, token, http.MethodPut, "/api/users/"+strconv.Itoa(int(userID)), updateBody)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/users/%d status = %d, body = %s", userID, recorder.Code, recorder.Body.String())
	}
	assertUserRole(t, recorder.Body.Bytes(), "qa", "release_manager")

	recorder = authedRequest(t, router, token, http.MethodDelete, "/api/users/"+strconv.Itoa(int(userID)), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/users/%d status = %d, body = %s", userID, recorder.Code, recorder.Body.String())
	}
	assertUserMissing(t, recorder.Body.Bytes(), "qa")
}

func TestUserManagementRequiresAdmin(t *testing.T) {
	router := newTestRouter(t)
	token := loginAs(t, router, "release", "release123")

	recorder := authedRequest(t, router, token, http.MethodGet, "/api/users", "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("GET /api/users as release manager status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestConfigManagementRequiresAdmin(t *testing.T) {
	router := newTestRouter(t)
	token := loginAs(t, router, "release", "release123")

	projectBody := `{
		"code":"search-api",
		"name":"搜索服务",
		"kind":"backend",
		"owner":"搜索组",
		"businessLineCode":"ops",
		"gitlabUrl":"https://gitlab.corp/delivery/search-api",
		"gitlabProjectId":"delivery/search-api",
		"defaultBranch":"master",
		"sortOrder":35,
		"enabled":true
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/projects", projectBody)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("POST /api/projects as release manager status = %d, body = %s", recorder.Code, recorder.Body.String())
	}

	recorder = authedRequest(t, router, token, http.MethodPut, "/api/dependencies/base-auth", `{"dependencies":[]}`)
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("PUT /api/dependencies/base-auth as release manager status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReleaseApprovalRequiresOperatorRole(t *testing.T) {
	router := newTestRouter(t)
	devToken := loginAs(t, router, "dev", "dev123")
	releaseToken := loginAs(t, router, "release", "release123")

	body := `{
		"releaseWindow":"2026-07-29T10:00:00Z",
		"remark":"approval test",
		"projects":[{"projectCode":"base-auth","sourceType":"branch","sourceRef":"master"}]
	}`
	recorder := authedRequest(t, router, devToken, http.MethodPost, "/api/releases", body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /api/releases status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		ID uint `json:"id"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create release response: %v", err)
	}

	recorder = authedRequest(t, router, devToken, http.MethodPost, "/api/releases/"+strconv.Itoa(int(created.ID))+"/approve", "")
	if recorder.Code != http.StatusForbidden {
		t.Fatalf("POST /api/releases/%d/approve as developer status = %d, body = %s", created.ID, recorder.Code, recorder.Body.String())
	}

	recorder = authedRequest(t, router, releaseToken, http.MethodPost, "/api/releases/"+strconv.Itoa(int(created.ID))+"/approve", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/releases/%d/approve status = %d, body = %s", created.ID, recorder.Code, recorder.Body.String())
	}
	var approved struct {
		Status     string `json:"status"`
		ApproverID *uint `json:"approverId"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &approved); err != nil {
		t.Fatalf("decode approve release response: %v", err)
	}
	if approved.Status != "approved" || approved.ApproverID == nil {
		t.Fatalf("approved payload = %+v, want approved status and approver", approved)
	}
}

func TestDisabledUserTokenIsRejected(t *testing.T) {
	router := newTestRouter(t)
	adminToken := login(t, router)
	devToken := loginAs(t, router, "dev", "dev123")

	recorder := authedRequest(t, router, adminToken, http.MethodGet, "/api/users", "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("GET /api/users status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	devID := assertUserRole(t, recorder.Body.Bytes(), "dev", "developer")

	body := `{
		"username":"dev",
		"displayName":"林辰",
		"role":"developer",
		"status":"disabled",
		"password":""
	}`
	recorder = authedRequest(t, router, adminToken, http.MethodPut, "/api/users/"+strconv.Itoa(int(devID)), body)
	if recorder.Code != http.StatusOK {
		t.Fatalf("PUT /api/users/%d status = %d, body = %s", devID, recorder.Code, recorder.Body.String())
	}

	recorder = authedRequest(t, router, devToken, http.MethodGet, "/api/projects", "")
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/projects with disabled user token status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestReleaseCanBeDeletedWithProjectsAndEvents(t *testing.T) {
	router := newTestRouter(t)
	token := login(t, router)

	body := `{
		"releaseWindow":"2026-07-29T10:00:00Z",
		"remark":"delete test",
		"projects":[{"projectCode":"base-auth","sourceType":"branch","sourceRef":"master"}]
	}`
	recorder := authedRequest(t, router, token, http.MethodPost, "/api/releases", body)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("POST /api/releases status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	var created struct {
		ID      uint   `json:"id"`
		BatchNo string `json:"batchNo"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create release response: %v", err)
	}
	if created.ID == 0 {
		t.Fatal("created release id is empty")
	}

	recorder = authedRequest(t, router, token, http.MethodDelete, "/api/releases/"+strconv.Itoa(int(created.ID)), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("DELETE /api/releases/%d status = %d, body = %s", created.ID, recorder.Code, recorder.Body.String())
	}
	assertReleaseMissing(t, recorder.Body.Bytes(), created.BatchNo)

	recorder = authedRequest(t, router, token, http.MethodGet, "/api/releases/"+strconv.Itoa(int(created.ID)), "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("GET deleted /api/releases/%d status = %d, body = %s", created.ID, recorder.Code, recorder.Body.String())
	}
}

func newTestRouter(t *testing.T) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dbName := strings.ReplaceAll(t.Name(), "/", "_")
	db, err := gorm.Open(sqlite.Open("file:"+dbName+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := database.Migrate(db); err != nil {
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
	return api.NewRouter(cfg, db, release.NewService(db, nil))
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
	return loginAs(t, router, "admin", "admin123")
}

func loginAs(t *testing.T, router *gin.Engine, username string, password string) string {
	t.Helper()
	body := bytes.NewBufferString(`{"username":"` + username + `","password":"` + password + `"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/auth/login", body)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()

	router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("POST /api/auth/login as %s status = %d, body = %s", username, recorder.Code, recorder.Body.String())
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

func projectCodesFromResponse(t *testing.T, body []byte) []string {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	codes := make([]string, 0, len(payload))
	for _, project := range payload {
		code, ok := project["code"].(string)
		if !ok || code == "" {
			t.Fatalf("project code = %v, want string", project["code"])
		}
		codes = append(codes, code)
	}
	return codes
}

func assertProjectOrder(t *testing.T, body []byte, codes []string) {
	t.Helper()
	actual := projectCodesFromResponse(t, body)
	if len(actual) != len(codes) {
		t.Fatalf("project order length = %d, want %d: %v", len(actual), len(codes), actual)
	}
	for index, expected := range codes {
		if actual[index] != expected {
			t.Fatalf("project order[%d] = %s, want %s; actual order = %v", index, actual[index], expected, actual)
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

func assertProjectBusinessLine(t *testing.T, body []byte, code string, businessLineCode string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	for _, project := range payload {
		if project["code"] == code {
			if project["businessLineCode"] != businessLineCode {
				t.Fatalf("project %s businessLineCode = %v, want %s", code, project["businessLineCode"], businessLineCode)
			}
			return
		}
	}
	t.Fatalf("project %s not found in response: %s", code, string(body))
}

func assertProjectBusinessLineCodes(t *testing.T, body []byte, code string, businessLineCodes []string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode projects response: %v", err)
	}
	for _, project := range payload {
		if project["code"] != code {
			continue
		}
		values, ok := project["businessLineCodes"].([]any)
		if !ok {
			t.Fatalf("project %s businessLineCodes type = %T, want array", code, project["businessLineCodes"])
		}
		actual := map[string]bool{}
		for _, value := range values {
			actual[value.(string)] = true
		}
		for _, expected := range businessLineCodes {
			if !actual[expected] {
				t.Fatalf("project %s businessLineCodes = %v, missing %s", code, values, expected)
			}
		}
		if len(values) != len(businessLineCodes) {
			t.Fatalf("project %s businessLineCodes = %v, want %v", code, values, businessLineCodes)
		}
		return
	}
	t.Fatalf("project %s not found in response: %s", code, string(body))
}

func assertReleaseProjectTagPrefix(t *testing.T, body []byte, projectCode string, prefix string) {
	t.Helper()
	var payload struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode release response: %v", err)
	}
	for _, item := range payload.Projects {
		project, ok := item["project"].(map[string]any)
		if !ok || project["code"] != projectCode {
			continue
		}
		tag, _ := item["targetTag"].(string)
		if !strings.HasPrefix(tag, prefix) {
			t.Fatalf("project %s targetTag = %s, want prefix %s", projectCode, tag, prefix)
		}
		return
	}
	t.Fatalf("project %s not found in release response: %s", projectCode, string(body))
}

func assertReleaseBusinessLine(t *testing.T, body []byte, businessLineCode string) {
	t.Helper()
	var payload struct {
		BusinessLine map[string]any `json:"businessLine"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode release response: %v", err)
	}
	if payload.BusinessLine["code"] != businessLineCode {
		t.Fatalf("release businessLine = %v, want %s", payload.BusinessLine["code"], businessLineCode)
	}
}

func assertReleaseProjectBusinessLine(t *testing.T, body []byte, projectCode string, businessLineCode string) {
	t.Helper()
	var payload struct {
		Projects []map[string]any `json:"projects"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode release response: %v", err)
	}
	for _, item := range payload.Projects {
		project, ok := item["project"].(map[string]any)
		if !ok || project["code"] != projectCode {
			continue
		}
		line, ok := item["businessLine"].(map[string]any)
		if !ok {
			t.Fatalf("project %s businessLine missing in release response", projectCode)
		}
		if line["code"] != businessLineCode {
			t.Fatalf("project %s businessLine = %v, want %s", projectCode, line["code"], businessLineCode)
		}
		return
	}
	t.Fatalf("project %s not found in release response: %s", projectCode, string(body))
}

func assertUserRole(t *testing.T, body []byte, username string, role string) uint {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode users response: %v", err)
	}
	for _, item := range payload {
		if item["username"] == username {
			if item["role"] != role {
				t.Fatalf("user %s role = %v, want %s", username, item["role"], role)
			}
			id, ok := item["id"].(float64)
			if !ok || id == 0 {
				t.Fatalf("user %s id = %v, want numeric id", username, item["id"])
			}
			return uint(id)
		}
	}
	t.Fatalf("user %s not found in response: %s", username, string(body))
	return 0
}

func assertUserMissing(t *testing.T, body []byte, username string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode users response: %v", err)
	}
	for _, item := range payload {
		if item["username"] == username {
			t.Fatalf("user %s should be absent after deletion", username)
		}
	}
}

func assertReleaseMissing(t *testing.T, body []byte, batchNo string) {
	t.Helper()
	var payload []map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("decode releases response: %v", err)
	}
	for _, item := range payload {
		if item["batchNo"] == batchNo {
			t.Fatalf("release %s should be absent after deletion", batchNo)
		}
	}
}
