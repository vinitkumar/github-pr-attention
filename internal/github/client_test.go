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
