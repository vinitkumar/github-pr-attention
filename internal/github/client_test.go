package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestListAttentionPRsDeduplicatesAndMergesReasons(t *testing.T) {
	seenQueries := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Fatalf("missing auth header: %q", r.Header.Get("Authorization"))
		}
		seenQueries = append(seenQueries, r.URL.Query().Get("q"))
		writeJSON(t, w, searchIssuesResponse{Items: []searchIssue{{
			ID:            10,
			Number:        42,
			Title:         "Fix attention",
			HTMLURL:       "https://github.com/acme/tool/pull/42",
			RepositoryURL: serverRepoURL(r, "acme", "tool"),
			User:          searchUser{Login: "octocat"},
			PullRequest:   &struct{}{},
		}}})
	}))
	defer server.Close()

	client := NewClientWithBaseURL("test-token", server.URL, server.Client())
	prs, err := client.ListAttentionPRs(context.Background())
	if err != nil {
		t.Fatalf("ListAttentionPRs returned error: %v", err)
	}

	if len(seenQueries) != 4 {
		t.Fatalf("expected 4 searches, got %d", len(seenQueries))
	}
	if len(prs) != 1 {
		t.Fatalf("expected one deduped PR, got %d", len(prs))
	}
	if prs[0].FullName() != "acme/tool" {
		t.Fatalf("unexpected full name: %s", prs[0].FullName())
	}
	if len(prs[0].Reasons) != 4 {
		t.Fatalf("expected merged reasons, got %#v", prs[0].Reasons)
	}
}

func TestAddCommentPostsIssueComment(t *testing.T) {
	var method, requestPath string
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()

	client := NewClientWithBaseURL("token", server.URL, server.Client())
	err := client.AddComment(context.Background(), "acme", "tool", 42, "  looks good  ")
	if err != nil {
		t.Fatalf("AddComment returned error: %v", err)
	}
	if method != http.MethodPost {
		t.Fatalf("method = %s", method)
	}
	if requestPath != "/repos/acme/tool/issues/42/comments" {
		t.Fatalf("path = %s", requestPath)
	}
	if payload["body"] != "looks good" {
		t.Fatalf("body = %q", payload["body"])
	}
}

func TestClosePullRequestPatchesStateClosed(t *testing.T) {
	var method, requestPath string
	var payload map[string]string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		requestPath = r.URL.Path
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client := NewClientWithBaseURL("token", server.URL, server.Client())
	err := client.ClosePullRequest(context.Background(), "acme", "tool", 42)
	if err != nil {
		t.Fatalf("ClosePullRequest returned error: %v", err)
	}
	if method != http.MethodPatch {
		t.Fatalf("method = %s", method)
	}
	if requestPath != "/repos/acme/tool/pulls/42" {
		t.Fatalf("path = %s", requestPath)
	}
	if payload["state"] != "closed" {
		t.Fatalf("state = %q", payload["state"])
	}
}

func TestMergeFailureIncludesAPIResponseBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte(`{"message":"Pull Request is not mergeable","documentation_url":"https://docs.github.com/rest/pulls/pulls#merge-a-pull-request"}`))
	}))
	defer server.Close()

	client := NewClientWithBaseURL("token", server.URL, server.Client())
	err := client.Merge(context.Background(), "acme", "tool", 42)
	if err == nil {
		t.Fatal("expected merge error")
	}

	text := err.Error()
	if !strings.Contains(text, "405 Method Not Allowed") {
		t.Fatalf("expected status in error, got %q", text)
	}
	if !strings.Contains(text, "response:") {
		t.Fatalf("expected response label in error, got %q", text)
	}
	if !strings.Contains(text, "Pull Request is not mergeable") {
		t.Fatalf("expected API response body in error, got %q", text)
	}
}

func TestGetPullRequestFilesFetchesChangedFiles(t *testing.T) {
	var requestPath, page string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestPath = r.URL.Path
		page = r.URL.Query().Get("page")
		writeJSON(t, w, []pullFileResponse{{
			Filename:  "internal/app/model.go",
			Status:    "modified",
			Additions: 12,
			Deletions: 3,
			Changes:   15,
			Patch:     "@@ -1 +1 @@\n-old\n+new",
		}})
	}))
	defer server.Close()

	client := NewClientWithBaseURL("token", server.URL, server.Client())
	files, err := client.GetPullRequestFiles(context.Background(), "acme", "tool", 42)
	if err != nil {
		t.Fatalf("GetPullRequestFiles returned error: %v", err)
	}
	if requestPath != "/repos/acme/tool/pulls/42/files" {
		t.Fatalf("path = %s", requestPath)
	}
	if page != "1" {
		t.Fatalf("page = %s", page)
	}
	if len(files) != 1 {
		t.Fatalf("files = %d", len(files))
	}
	if files[0].Patch == "" || files[0].Filename != "internal/app/model.go" {
		t.Fatalf("unexpected file: %#v", files[0])
	}
}

func TestGetPullRequestIncludesCIStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/acme/tool/pulls/42":
			writeJSON(t, w, pullResponse{
				ID:     1,
				Number: 42,
				Title:  "Add CI",
				Head:   refObject{Ref: "feature", SHA: "abc123"},
				Base:   refObject{Ref: "main"},
				User:   searchUser{Login: "octocat"},
			})
		case "/repos/acme/tool/commits/abc123/status":
			writeJSON(t, w, combinedStatusResponse{Statuses: []statusResponse{{
				Context: "legacy/status",
				State:   "success",
			}}})
		case "/repos/acme/tool/commits/abc123/check-runs":
			writeJSON(t, w, checkRunsResponse{CheckRuns: []checkRunResponse{{
				Name:       "test",
				Status:     "completed",
				Conclusion: "failure",
			}}})
		default:
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	client := NewClientWithBaseURL("token", server.URL, server.Client())
	detail, err := client.GetPullRequest(context.Background(), "acme", "tool", 42)
	if err != nil {
		t.Fatalf("GetPullRequest returned error: %v", err)
	}
	if detail.HeadSHA != "abc123" {
		t.Fatalf("HeadSHA = %q", detail.HeadSHA)
	}
	if detail.CIStatus.State != CIStateFailure {
		t.Fatalf("CI state = %q", detail.CIStatus.State)
	}
	if !strings.Contains(detail.CIStatus.Summary, "1 failed") {
		t.Fatalf("CI summary = %q", detail.CIStatus.Summary)
	}
}

func TestSummarizeCIReportsPendingChecks(t *testing.T) {
	status := summarizeCI(combinedStatusResponse{}, checkRunsResponse{CheckRuns: []checkRunResponse{{
		Name:   "build",
		Status: "in_progress",
	}}})

	if status.State != CIStatePending {
		t.Fatalf("state = %q", status.State)
	}
	if status.Summary != "1 pending" {
		t.Fatalf("summary = %q", status.Summary)
	}
}

func TestSubmitReviewValidatesRequestChangesBody(t *testing.T) {
	client := NewClientWithBaseURL("token", "http://example.test", nil)
	err := client.SubmitReview(context.Background(), "acme", "tool", 42, ReviewRequestChanges, " ")
	if err == nil || !strings.Contains(err.Error(), "requires a body") {
		t.Fatalf("expected body validation error, got %v", err)
	}
}

func TestParseRepoURL(t *testing.T) {
	owner, repo := parseRepoURL("https://api.github.com/repos/acme/tool")
	if owner != "acme" || repo != "tool" {
		t.Fatalf("owner/repo = %s/%s", owner, repo)
	}
}

func writeJSON(t *testing.T, w http.ResponseWriter, value any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(value); err != nil {
		t.Fatalf("write json: %v", err)
	}
}

func serverRepoURL(r *http.Request, owner, repo string) string {
	return (&url.URL{Scheme: "http", Host: r.Host, Path: "/repos/" + owner + "/" + repo}).String()
}
