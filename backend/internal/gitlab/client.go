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
		baseURL: strings.TrimRight(cfg.BaseURL, "/"),
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
	return c.do(req, nil)
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
		return PipelineResponse{}, err
	}
	if len(payload) == 0 {
		return PipelineResponse{}, fmt.Errorf("gitlab pipeline for ref %s not found", ref)
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
		return nil, err
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

	var payload struct {
		ID           int    `json:"id"`
		Name         string `json:"name"`
		Stage        string `json:"stage"`
		Status       string `json:"status"`
		WebURL       string `json:"web_url"`
		AllowFailure bool   `json:"allow_failure"`
	}
	if err := c.do(req, &payload); err != nil {
		return JobResponse{}, err
	}
	return JobResponse{
		ID:           strconv.Itoa(payload.ID),
		Name:         payload.Name,
		Stage:        payload.Stage,
		Status:       payload.Status,
		WebURL:       payload.WebURL,
		AllowFailure: payload.AllowFailure,
		Manual:       payload.Status == "manual",
	}, nil
}

func (c *Client) do(req *http.Request, out interface{}) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("gitlab api %s: %s", resp.Status, string(data))
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, out)
}
