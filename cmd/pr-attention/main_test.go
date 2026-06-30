package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

func TestRunReportWritesText(t *testing.T) {
	client := fakeReportClient{prs: []github.PullRequest{{
		Owner:  "acme",
		Repo:   "tool",
		Number: 42,
		Title:  "Fix review queue",
	}}}

	var out bytes.Buffer
	if err := runReport(context.Background(), &out, client, "text", 0); err != nil {
		t.Fatalf("runReport returned error: %v", err)
	}

	if !strings.Contains(out.String(), "acme/tool") {
		t.Fatalf("expected text report, got:\n%s", out.String())
	}
}

func TestRunReportWritesJSONWithLimit(t *testing.T) {
	client := fakeReportClient{prs: []github.PullRequest{
		{Owner: "acme", Repo: "one", Number: 1},
		{Owner: "acme", Repo: "two", Number: 2},
	}}

	var out bytes.Buffer
	if err := runReport(context.Background(), &out, client, "json", 1); err != nil {
		t.Fatalf("runReport returned error: %v", err)
	}

	text := out.String()
	if !strings.Contains(text, `"repo": "one"`) {
		t.Fatalf("expected first PR in json report, got:\n%s", text)
	}
	if strings.Contains(text, `"repo": "two"`) {
		t.Fatalf("expected limited json report, got:\n%s", text)
	}
}

func TestRunReportRejectsUnsupportedFormat(t *testing.T) {
	err := runReport(context.Background(), &bytes.Buffer{}, fakeReportClient{}, "xml", 0)
	if err == nil || !strings.Contains(err.Error(), "unsupported output format") {
		t.Fatalf("expected unsupported format error, got %v", err)
	}
}

func TestRunReportReturnsListError(t *testing.T) {
	err := runReport(context.Background(), &bytes.Buffer{}, fakeReportClient{err: errors.New("github unavailable")}, "text", 0)
	if err == nil || !strings.Contains(err.Error(), "github unavailable") {
		t.Fatalf("expected list error, got %v", err)
	}
}

type fakeReportClient struct {
	prs []github.PullRequest
	err error
}

func (c fakeReportClient) ListAttentionPRs(context.Context) ([]github.PullRequest, error) {
	return c.prs, c.err
}
