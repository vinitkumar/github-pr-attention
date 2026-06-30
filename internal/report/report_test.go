package report

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

func TestWriteTextRendersAttentionTable(t *testing.T) {
	updated := time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC)
	prs := []github.PullRequest{{
		Owner:     "acme",
		Repo:      "tool",
		Number:    42,
		Title:     "Fix review queue",
		Author:    "octocat",
		Reasons:   []github.AttentionReason{github.ReasonReviewRequested, github.ReasonAssigned},
		UpdatedAt: updated,
	}}

	var out bytes.Buffer
	if err := WriteText(&out, prs); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}

	text := out.String()
	for _, want := range []string{"REPOSITORY", "acme/tool", "#42", "review requested, assigned", "2026-06-30", "Fix review queue"} {
		if !strings.Contains(text, want) {
			t.Fatalf("expected %q in text report:\n%s", want, text)
		}
	}
}

func TestWriteTextRendersClearInbox(t *testing.T) {
	var out bytes.Buffer
	if err := WriteText(&out, nil); err != nil {
		t.Fatalf("WriteText returned error: %v", err)
	}
	if !strings.Contains(out.String(), "Inbox clear") {
		t.Fatalf("expected clear inbox message, got %q", out.String())
	}
}

func TestWriteJSONRendersStableShape(t *testing.T) {
	created := time.Date(2026, 6, 29, 8, 0, 0, 0, time.UTC)
	updated := time.Date(2026, 6, 30, 8, 0, 0, 0, time.UTC)
	prs := []github.PullRequest{{
		Owner:     "acme",
		Repo:      "tool",
		Number:    42,
		Title:     "Fix review queue",
		URL:       "https://github.com/acme/tool/pull/42",
		Author:    "octocat",
		Reasons:   []github.AttentionReason{github.ReasonMentioned},
		CreatedAt: created,
		UpdatedAt: updated,
	}}

	var out bytes.Buffer
	if err := WriteJSON(&out, prs); err != nil {
		t.Fatalf("WriteJSON returned error: %v", err)
	}

	var decoded []JSONPullRequest
	if err := json.Unmarshal(out.Bytes(), &decoded); err != nil {
		t.Fatalf("decode json report: %v", err)
	}
	if len(decoded) != 1 {
		t.Fatalf("decoded PRs = %d", len(decoded))
	}
	if decoded[0].Owner != "acme" || decoded[0].Repo != "tool" || decoded[0].Number != 42 {
		t.Fatalf("unexpected decoded PR: %#v", decoded[0])
	}
	if strings.Join(decoded[0].Reasons, ",") != "mentioned" {
		t.Fatalf("reasons = %#v", decoded[0].Reasons)
	}
	if decoded[0].UpdatedAt != "2026-06-30T08:00:00Z" {
		t.Fatalf("updated_at = %q", decoded[0].UpdatedAt)
	}
}
