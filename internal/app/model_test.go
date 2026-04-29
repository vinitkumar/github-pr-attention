package app

import (
	"context"
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

type fakeClient struct{}

func (fakeClient) ListAttentionPRs(context.Context) ([]github.PullRequest, error) {
	return nil, nil
}

func (fakeClient) GetPullRequest(context.Context, string, string, int) (github.PullRequestDetail, error) {
	return github.PullRequestDetail{}, nil
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
