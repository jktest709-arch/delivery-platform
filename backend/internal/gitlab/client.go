package gitlab

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
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

func (c *Client) TriggerPipeline(ctx context.Context, projectID, ref string, variables map[string]string) (PipelineResponse, error) {
	if c.dryRun {
		return PipelineResponse{
			ID:     fmt.Sprintf("dryrun-%d", time.Now().UnixNano()),
			WebURL: fmt.Sprintf("%s/%s/-/pipelines/dryrun", c.baseURL, projectID),
		}, nil
	}
	if c.token == "" {
		return PipelineResponse{}, fmt.Errorf("GITLAB_TOKEN is required when GITLAB_DRY_RUN=false")
	}

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err := writer.WriteField("ref", ref); err != nil {
		return PipelineResponse{}, err
	}
	i := 0
	for key, value := range variables {
		if err := writer.WriteField(fmt.Sprintf("variables[%d][key]", i), key); err != nil {
			return PipelineResponse{}, err
		}
		if err := writer.WriteField(fmt.Sprintf("variables[%d][value]", i), value); err != nil {
			return PipelineResponse{}, err
		}
		i++
	}
	if err := writer.Close(); err != nil {
		return PipelineResponse{}, err
	}

	endpoint := fmt.Sprintf("%s/api/v4/projects/%s/pipeline", c.baseURL, url.PathEscape(projectID))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return PipelineResponse{}, err
	}
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("PRIVATE-TOKEN", c.token)

	var payload struct {
		ID     int    `json:"id"`
		WebURL string `json:"web_url"`
	}
	if err := c.do(req, &payload); err != nil {
		return PipelineResponse{}, err
	}
	return PipelineResponse{
		ID:     strconv.Itoa(payload.ID),
		WebURL: payload.WebURL,
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
