package gitlab

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCreateTagAcceptsBaseURLWithAPIV4Suffix(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/api/v4/projects/delivery%2Fbase-auth/repository/tags" {
			t.Fatalf("request path = %s, want /api/v4/projects/delivery%%2Fbase-auth/repository/tags", r.URL.EscapedPath())
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.Header.Get("PRIVATE-TOKEN") != "token" {
			t.Fatal("PRIVATE-TOKEN header was not sent")
		}
		if err := r.ParseForm(); err != nil {
			t.Fatalf("parse form: %v", err)
		}
		if r.Form.Get("tag_name") != "aaprd-20260729160201-0001" || r.Form.Get("ref") != "master" {
			t.Fatalf("form = %v", r.Form)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL + "/api/v4",
		Token:   "token",
		DryRun:  false,
	})

	if err := client.CreateTag(context.Background(), "delivery/base-auth", "aaprd-20260729160201-0001", "master"); err != nil {
		t.Fatalf("create tag: %v", err)
	}
}

func TestCreateTagErrorIncludesOperationAndEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"message":"404 Project Not Found"}`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "token",
		DryRun:  false,
	})

	err := client.CreateTag(context.Background(), "delivery/missing", "aaprd-20260729160201-0001", "master")
	if err == nil {
		t.Fatal("create tag succeeded unexpectedly")
	}
	message := err.Error()
	for _, want := range []string{
		`create tag "aaprd-20260729160201-0001"`,
		"GitLab project \"delivery/missing\"",
		"POST " + server.URL + "/api/v4/projects/delivery%2Fmissing/repository/tags",
		"404 Project Not Found",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}

func TestCreateTagForbiddenErrorIncludesPermissionHint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`403 Forbidden`))
	}))
	defer server.Close()

	client := NewClient(Config{
		BaseURL: server.URL,
		Token:   "token",
		DryRun:  false,
	})

	err := client.CreateTag(context.Background(), "143", "ftprd-20260729160201-0001", "master")
	if err == nil {
		t.Fatal("create tag succeeded unexpectedly")
	}
	message := err.Error()
	for _, want := range []string{
		"GITLAB_TOKEN",
		"创建 tag 权限",
		"Protected Tags",
		"ftprd-20260729160201-0001",
	} {
		if !strings.Contains(message, want) {
			t.Fatalf("error %q does not contain %q", message, want)
		}
	}
}
