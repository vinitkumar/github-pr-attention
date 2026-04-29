package app

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

func TestListLoadedSelectsLatestStatus(t *testing.T) {
	model := New(fakeClient{})
	updated, _ := model.Update(listLoadedMsg{prs: []github.PullRequest{{
		Owner: "acme", Repo: "tool", Number: 1, Title: "Fix bug",
	}}})

	m := updated.(Model)
	if m.status != "1 PRs need attention" {
		t.Fatalf("status = %q", m.status)
	}
	if len(m.prs) != 1 {
		t.Fatalf("prs = %d", len(m.prs))
	}
}

func TestEnterLoadsSelectedDetail(t *testing.T) {
	client := fakeClient{}
	model := New(client)
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.loading = false

	_, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected detail load command")
	}
}

func TestWrapTextLimitsLines(t *testing.T) {
	wrapped := wrapText("one two three four five six", 8, 2)
	if wrapped != "one two\nthree" {
		t.Fatalf("wrapped = %q", wrapped)
	}
}

func TestRenderDetailContentPreservesFencedCode(t *testing.T) {
	model := New(fakeClient{})
	model.width = 100
	model.height = 30
	model.viewport.Width = 98
	model.viewport.Height = 26
	model.detail = &github.PullRequestDetail{
		PullRequest: github.PullRequest{
			Owner:  "acme",
			Repo:   "tool",
			Number: 42,
			Title:  "Render markdown",
			URL:    "https://github.com/acme/tool/pull/42",
			Author: "octocat",
		},
		State:        "open",
		BaseRef:      "main",
		HeadRef:      "feature",
		Additions:    12,
		Deletions:    2,
		ChangedFiles: 3,
		HeadSHA:      "abc123456789",
		CIStatus: github.CIStatus{
			State:   github.CIStateSuccess,
			Summary: "2 passed",
		},
		Body: "## Summary\n\nKeeps code readable:\n\n```go\nfmt.Println(\"ok\")\n```\n",
	}

	content := model.renderDetailContent()
	if !strings.Contains(content, "fmt.Println") {
		t.Fatalf("expected rendered detail to contain code, got:\n%s", content)
	}
	if !strings.Contains(content, "Description") {
		t.Fatalf("expected rendered detail section label, got:\n%s", content)
	}
	if !strings.Contains(content, "success (2 passed)") {
		t.Fatalf("expected CI summary, got:\n%s", content)
	}
}

func TestRenderDetailContentShowsChangedFiles(t *testing.T) {
	model := New(fakeClient{})
	model.width = 100
	model.height = 30
	model.viewport.Width = 98
	model.viewport.Height = 26
	model.detailTab = tabChanges
	model.detail = &github.PullRequestDetail{
		PullRequest: github.PullRequest{
			Owner:  "acme",
			Repo:   "tool",
			Number: 42,
			Title:  "Render changes",
			URL:    "https://github.com/acme/tool/pull/42",
			Author: "octocat",
		},
		State:        "open",
		BaseRef:      "main",
		HeadRef:      "feature",
		Additions:    1,
		Deletions:    1,
		ChangedFiles: 1,
		Body:         "Body",
	}
	model.files = []github.PullRequestFile{{
		Filename:  "internal/app/model.go",
		Status:    "modified",
		Additions: 1,
		Deletions: 1,
		Patch:     "@@ -1 +1 @@\n-old\n+new",
	}}

	content := model.renderDetailContent()
	if !strings.Contains(content, "internal/app/model.go") {
		t.Fatalf("expected changed file path, got:\n%s", content)
	}
	if !strings.Contains(content, "+new") {
		t.Fatalf("expected rendered patch addition, got:\n%s", content)
	}
}

type fakeClient struct{}

func (fakeClient) ListAttentionPRs(context.Context) ([]github.PullRequest, error) {
	return nil, nil
}

func (fakeClient) GetPullRequest(context.Context, string, string, int) (github.PullRequestDetail, error) {
	return github.PullRequestDetail{}, nil
}

func (fakeClient) GetPullRequestFiles(context.Context, string, string, int) ([]github.PullRequestFile, error) {
	return nil, nil
}

func (fakeClient) AddComment(context.Context, string, string, int, string) error {
	return nil
}

func (fakeClient) SubmitReview(context.Context, string, string, int, github.ReviewEvent, string) error {
	return nil
}

func (fakeClient) Merge(context.Context, string, string, int) error {
	return nil
}
