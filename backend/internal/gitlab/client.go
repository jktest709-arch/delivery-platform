package gitlab

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	BaseURL string
	Token   string
	DryRun  bool
}

type Client struct {
	baseURL string
	token   string
	dryRun  bool
	http    *http.Client
}

type PipelineResponse struct {
	ID     string
	WebURL string
}

type JobResponse struct {
	ID           string
	Name         string
	Stage        string
	Status       string
	WebURL       string
	AllowFailure bool
	Manual       bool
}

func NewClient(cfg Config) *Client {
	return &Client{
		baseURL: normalizeBaseURL(cfg.BaseURL),
		token:   cfg.Token,
		dryRun:  cfg.DryRun,
		http: &http.Client{
			Timeout: 30 * time.Second,
		},
	}
}

func (c *Client) CreateTag(ctx context.Context, projectID, tagName, ref string) error {
	if c.dryRun {
		return nil
	}
	if c.token == "" {
		return fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	values := url.Values{}
	values.Set("tag_name", tagName)
	values.Set("ref", ref)

	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/repository/tags", c.baseURL, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(values.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("PRIVATE-TOKEN", c.token)
	if err := c.do(req, nil); err != nil {
		if isForbiddenError(err) {
			return fmt.Errorf(
				"create tag %q from ref %q for GitLab project %q failed: %w；请检查 GITLAB_TOKEN 对该项目是否具备创建 tag 权限，以及 GitLab Protected Tags 是否允许当前用户创建 %q 这类 tag",
				tagName,
				ref,
				projectID,
				err,
				tagName,
			)
		}
		return fmt.Errorf("create tag %q from ref %q for GitLab project %q: %w", tagName, ref, projectID, err)
	}
	return nil
}

func (c *Client) FindPipelineByRef(ctx context.Context, projectID, ref string) (PipelineResponse, error) {
	if c.dryRun {
		return PipelineResponse{
			ID:     fmt.Sprintf("dryrun-%d", time.Now().UnixNano()),
			WebURL: fmt.Sprintf("%s/%s/-/pipelines/dryrun", c.baseURL, projectID),
		}, nil
	}
	if c.token == "" {
		return PipelineResponse{}, fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	query := url.Values{}
	query.Set("ref", ref)
	query.Set("order_by", "id")
	query.Set("sort", "desc")
	query.Set("per_page", "1")
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipelines?%s", c.baseURL, url.PathEscape(projectID), query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return PipelineResponse{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	var payload []struct {
		ID     int    `json:"id"`
		WebURL string `json:"web_url"`
	}
	if err := c.do(req, &payload); err != nil {
		return PipelineResponse{}, fmt.Errorf("query pipeline for ref %q in GitLab project %q: %w", ref, projectID, err)
	}
	if len(payload) == 0 {
		return PipelineResponse{}, fmt.Errorf("gitlab pipeline for ref %q in project %q not found", ref, projectID)
	}
	return PipelineResponse{
		ID:     strconv.Itoa(payload[0].ID),
		WebURL: payload[0].WebURL,
	}, nil
}

func (c *Client) ListPipelineJobs(ctx context.Context, projectID, pipelineID string) ([]JobResponse, error) {
	if c.dryRun {
		return []JobResponse{
			{ID: "dryrun-build", Name: "build", Stage: "build", Status: "manual", WebURL: fmt.Sprintf("%s/%s/-/jobs/dryrun-build", c.baseURL, projectID), Manual: true},
			{ID: "dryrun-deploy", Name: "deploy", Stage: "deploy", Status: "manual", WebURL: fmt.Sprintf("%s/%s/-/jobs/dryrun-deploy", c.baseURL, projectID), Manual: true},
		}, nil
	}
	if c.token == "" {
		return nil, fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	query := url.Values{}
	query.Set("include_retried", "true")
	query.Set("per_page", "100")
	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipelines/%s/jobs?%s", c.baseURL, url.PathEscape(projectID), url.PathEscape(pipelineID), query.Encode())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	var payload []struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Stage        string `json:"stage"`
		Status       string `json:"status"`
		WebURL       string `json:"web_url"`
		AllowFailure bool   `json:"allow_failure"`
	}
	if err := c.do(req, &payload); err != nil {
		return nil, fmt.Errorf("list jobs for pipeline %q in GitLab project %q: %w", pipelineID, projectID, err)
	}
	jobs := make([]JobResponse, 0, len(payload))
	for _, item := range payload {
		jobs = append(jobs, JobResponse{
			ID:           strconv.Itoa(item.ID),
			Name:         item.Name,
			Stage:        item.Stage,
			Status:       item.Status,
			WebURL:       item.WebURL,
			AllowFailure: item.AllowFailure,
			Manual:       item.Status == "manual",
		})
	}
	return jobs, nil
}

func (c *Client) PlayJob(ctx context.Context, projectID, jobID string) (JobResponse, error) {
	if c.dryRun {
		return JobResponse{
			ID:     jobID,
			Name:   jobID,
			Stage:  jobID,
			Status: "running",
			WebURL: fmt.Sprintf("%s/%s/-/jobs/%s", c.baseURL, projectID, jobID),
		}, nil
	}
	if c.token == "" {
		return JobResponse{}, fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/jobs/%s/play", c.baseURL, url.PathEscape(projectID), url.PathEscape(jobID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return JobResponse{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	var payload jobPayload
	if err := c.do(req, &payload); err != nil {
		return JobResponse{}, fmt.Errorf("play job %q in GitLab project %q: %w", jobID, projectID, err)
	}
	return jobFromPayload(payload), nil
}

func (c *Client) RetryJob(ctx context.Context, projectID, jobID string) (JobResponse, error) {
	if c.dryRun {
		return JobResponse{
			ID:     jobID,
			Name:   jobID,
			Stage:  jobID,
			Status: "running",
			WebURL: fmt.Sprintf("%s/%s/-/jobs/%s", c.baseURL, projectID, jobID),
		}, nil
	}
	if c.token == "" {
		return JobResponse{}, fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/jobs/%s/retry", c.baseURL, url.PathEscape(projectID), url.PathEscape(jobID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		return JobResponse{}, err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	var payload jobPayload
	if err := c.do(req, &payload); err != nil {
		return JobResponse{}, fmt.Errorf("retry job %q in GitLab project %q: %w", jobID, projectID, err)
	}
	return jobFromPayload(payload), nil
}

func (c *Client) GetJobTrace(ctx context.Context, projectID, jobID string) (string, error) {
	if c.dryRun {
		return fmt.Sprintf("dry-run trace for GitLab project %s job %s\n", projectID, jobID), nil
	}
	if c.token == "" {
		return "", fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/jobs/%s/trace", c.baseURL, url.PathEscape(projectID), url.PathEscape(jobID))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("PRIVATE-TOKEN", c.token)

	data, err := c.doBytes(req)
	if err != nil {
		return "", fmt.Errorf("get trace for job %q in GitLab project %q: %w", jobID, projectID, err)
	}
	return string(data), nil
}

func normalizeBaseURL(baseURL string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if strings.HasSuffix(strings.ToLower(baseURL), "/api/v4") {
		baseURL = strings.TrimRight(baseURL[:len(baseURL)-len("/api/v4")], "/")
	}
	return baseURL
}

func isForbiddenError(err error) bool {
	return strings.Contains(err.Error(), "403 Forbidden")
}

func (c *Client) do(req *http.Request, out interface{}) error {
	data, err := c.doBytes(req)
	if err != nil {
		return err
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}

func (c *Client) doBytes(req *http.Request) ([]byte, error) {
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("gitlab api %s %s returned %s: %s", req.Method, req.URL.Redacted(), resp.Status, gitLabErrorMessage(data, resp.StatusCode))
	}
	return data, nil
}

func gitLabErrorMessage(data []byte, statusCode int) string {
	body := strings.TrimSpace(string(data))
	if body == "" {
		return http.StatusText(statusCode)
	}
	var payload struct {
		Message          interface{} `json:"message"`
		Error            string      `json:"error"`
		ErrorDescription string      `json:"error_description"`
	}
	if err := json.Unmarshal(data, &payload); err == nil {
		switch message := payload.Message.(type) {
		case string:
			if strings.TrimSpace(message) != "" {
				return message
			}
		case nil:
		default:
			encoded, _ := json.Marshal(message)
			if len(encoded) > 0 && string(encoded) != "null" {
				return string(encoded)
			}
		}
		if strings.TrimSpace(payload.ErrorDescription) != "" {
			return payload.ErrorDescription
		}
		if strings.TrimSpace(payload.Error) != "" {
			return payload.Error
		}
	}
	return body
}

type jobPayload struct {
	ID           int    `json:"id"`
	Name         string `json:"name"`
	Stage        string `json:"stage"`
	Status       string `json:"status"`
	WebURL       string `json:"web_url"`
	AllowFailure bool   `json:"allow_failure"`
}

func jobFromPayload(payload jobPayload) JobResponse {
	return JobResponse{
		ID:           strconv.Itoa(payload.ID),
		Name:         payload.Name,
		Stage:        payload.Stage,
		Status:       payload.Status,
		WebURL:       payload.WebURL,
		AllowFailure: payload.AllowFailure,
		Manual:       payload.Status == "manual",
	}
}
