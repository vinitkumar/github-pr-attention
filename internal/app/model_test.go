package app

import (
	"context"
	"errors"
	"strconv"
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

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	if cmd == nil {
		t.Fatal("expected detail load command")
	}
	m := updated.(Model)
	if m.mode != modeDetail {
		t.Fatal("expected detail mode while detail loads")
	}
	if !m.detailBusy || !m.filesBusy {
		t.Fatalf("expected detail and file loads to be busy, detailBusy=%v filesBusy=%v", m.detailBusy, m.filesBusy)
	}
	if m.detail != nil {
		t.Fatal("expected detail to be empty until the detail request completes")
	}
}

func TestEscCancelsPendingDetailLoad(t *testing.T) {
	model := New(fakeClient{})
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := updated.(Model)
	requestID := m.detailReq

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = updated.(Model)
	if m.mode != modeList {
		t.Fatal("expected esc to return to list while detail loads")
	}
	if m.loading || m.detailBusy || m.filesBusy {
		t.Fatalf("expected pending load state to be cleared, loading=%v detailBusy=%v filesBusy=%v", m.loading, m.detailBusy, m.filesBusy)
	}

	updated, _ = m.Update(detailLoadedMsg{
		requestID: requestID,
		detail: github.PullRequestDetail{PullRequest: github.PullRequest{
			Owner: "acme", Repo: "tool", Number: 42, Title: "Late detail",
		}},
	})
	m = updated.(Model)
	if m.mode != modeList {
		t.Fatal("late detail response should not reopen detail mode")
	}
	if m.detail != nil {
		t.Fatal("late detail response should be ignored")
	}
}

func TestDetailBackKeysReturnToList(t *testing.T) {
	tests := []struct {
		name string
		key  tea.KeyMsg
	}{
		{name: "b", key: tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("b")}},
		{name: "backspace", key: tea.KeyMsg{Type: tea.KeyBackspace}},
		{name: "left", key: tea.KeyMsg{Type: tea.KeyLeft}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			model := New(fakeClient{})
			model.mode = modeDetail
			model.detail = &github.PullRequestDetail{PullRequest: github.PullRequest{
				Owner: "acme", Repo: "tool", Number: 42,
			}}
			model.files = []github.PullRequestFile{{Filename: "internal/app/model.go"}}
			model.detailBusy = true
			model.filesBusy = true
			model.loading = true

			updated, cmd := model.Update(tt.key)
			if cmd != nil {
				t.Fatal("back key should not start a command")
			}
			m := updated.(Model)
			if m.mode != modeList {
				t.Fatal("expected back key to return to list")
			}
			if m.detail != nil || m.files != nil {
				t.Fatal("expected detail state to be cleared")
			}
			if m.loading || m.detailBusy || m.filesBusy {
				t.Fatalf("expected load state to be cleared, loading=%v detailBusy=%v filesBusy=%v", m.loading, m.detailBusy, m.filesBusy)
			}
		})
	}
}

func TestDetailMetadataCanRenderBeforeFilesLoad(t *testing.T) {
	model := New(fakeClient{})
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.width = 100
	model.height = 30
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := updated.(Model)
	requestID := m.detailReq

	updated, _ = m.Update(detailLoadedMsg{
		requestID: requestID,
		detail: github.PullRequestDetail{PullRequest: github.PullRequest{
			Owner: "acme", Repo: "tool", Number: 42, Title: "Render early",
		}},
	})
	m = updated.(Model)
	if m.detail == nil {
		t.Fatal("expected detail metadata to be visible before files complete")
	}
	if !m.filesBusy {
		t.Fatal("expected file load to continue in the background")
	}
	if m.status != "Detail loaded; loading changes..." {
		t.Fatalf("status = %q", m.status)
	}

	updated, _ = m.Update(filesLoadedMsg{
		requestID: requestID,
		files: []github.PullRequestFile{{
			Filename: "internal/app/model.go",
			Status:   "modified",
		}},
	})
	m = updated.(Model)
	if m.loading || m.filesBusy {
		t.Fatalf("expected loading to finish, loading=%v filesBusy=%v", m.loading, m.filesBusy)
	}
	if len(m.files) != 1 {
		t.Fatalf("files = %d", len(m.files))
	}
}

func TestDetailErrorClearsBackgroundFileLoad(t *testing.T) {
	model := New(fakeClient{})
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := updated.(Model)

	updated, _ = m.Update(detailLoadedMsg{
		requestID: m.detailReq,
		err:       errors.New("github unavailable"),
	})
	m = updated.(Model)
	if m.loading || m.detailBusy || m.filesBusy {
		t.Fatalf("expected failed detail load to stop all loading state, loading=%v detailBusy=%v filesBusy=%v", m.loading, m.detailBusy, m.filesBusy)
	}
	if m.status != "Could not load PR detail" {
		t.Fatalf("status = %q", m.status)
	}
}

func TestSlashFiltersListAndSelectionUsesFilteredPRs(t *testing.T) {
	model := New(fakeClient{})
	model.prs = []github.PullRequest{
		{Owner: "acme", Repo: "tool", Number: 1, Title: "Fix parser"},
		{Owner: "acme", Repo: "tool", Number: 2, Title: "Add billing"},
		{Owner: "other", Repo: "service", Number: 3, Title: "Fix auth"},
	}
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("fix")})
	updated, _ = updated.(Model).Update(tea.KeyMsg{Type: tea.KeyEnter})
	m := updated.(Model)

	if m.filter != "fix" {
		t.Fatalf("filter = %q", m.filter)
	}
	if got := len(m.filteredPRs()); got != 2 {
		t.Fatalf("filtered PR count = %d", got)
	}

	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(Model)
	pr, ok := m.currentPR()
	if !ok {
		t.Fatal("expected current filtered PR")
	}
	if pr.Number != 3 {
		t.Fatalf("selected PR number = %d", pr.Number)
	}
}

func TestClosePullRequestRequiresSecondKeyPress(t *testing.T) {
	client := &recordingClient{}
	model := New(client)
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd != nil {
		t.Fatal("first d should only ask for confirmation")
	}
	m := updated.(Model)
	if m.status != "Press d again to close without merging" {
		t.Fatalf("status = %q", m.status)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd == nil {
		t.Fatal("second d should close the pull request")
	}
	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("message = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("close command returned error: %v", done.err)
	}
	if !done.refresh {
		t.Fatal("close should refresh the list")
	}
	if client.closed != "acme/tool#42" {
		t.Fatalf("closed = %q", client.closed)
	}
}

func TestBulkMergeRequiresConfirmationAndMergesFilteredList(t *testing.T) {
	client := &recordingClient{}
	model := New(client)
	model.prs = []github.PullRequest{
		{Owner: "acme", Repo: "tool", Number: 1, Title: "Fix parser"},
		{Owner: "acme", Repo: "tool", Number: 2, Title: "Add billing"},
		{Owner: "other", Repo: "service", Number: 3, Title: "Fix auth"},
	}
	model.filter = "fix"
	model.loading = false

	updated, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if cmd != nil {
		t.Fatal("first M should only ask for confirmation")
	}
	m := updated.(Model)
	if m.status != "Press M again to squash merge 2 listed PRs" {
		t.Fatalf("status = %q", m.status)
	}

	_, cmd = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("M")})
	if cmd == nil {
		t.Fatal("second M should bulk merge visible pull requests")
	}
	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("message = %#v", msg)
	}
	if done.err != nil {
		t.Fatalf("bulk merge command returned error: %v", done.err)
	}
	if !done.refresh {
		t.Fatal("bulk merge should refresh the list")
	}
	if strings.Join(client.merged, ",") != "acme/tool#1,other/service#3" {
		t.Fatalf("merged = %#v", client.merged)
	}
}

func TestBulkMergeContinuesAfterIndividualMergeFailure(t *testing.T) {
	client := &recordingClient{
		mergeErrors: map[string]error{
			"acme/tool#2": errors.New("merge blocked"),
		},
	}
	model := New(client)
	model.prs = []github.PullRequest{
		{Owner: "acme", Repo: "tool", Number: 1, Title: "Ready"},
		{Owner: "acme", Repo: "tool", Number: 2, Title: "Blocked"},
		{Owner: "acme", Repo: "tool", Number: 3, Title: "Also ready"},
	}
	model.loading = false

	cmd := model.mergePullRequests(model.prs)
	msg := cmd()
	done, ok := msg.(actionDoneMsg)
	if !ok {
		t.Fatalf("message = %#v", msg)
	}
	if done.refresh {
		t.Fatal("partial failure should keep the error visible instead of refreshing immediately")
	}
	if !strings.Contains(done.message, "Merged 2/3 PRs") {
		t.Fatalf("message = %q", done.message)
	}
	if done.err == nil {
		t.Fatal("expected partial failure details in error")
	}
	if !strings.Contains(done.err.Error(), "acme/tool #2") {
		t.Fatalf("err = %v", done.err)
	}
	if !strings.Contains(done.err.Error(), "merge blocked") {
		t.Fatalf("err = %v", done.err)
	}
	if strings.Join(client.mergeAttempts, ",") != "acme/tool#1,acme/tool#2,acme/tool#3" {
		t.Fatalf("merge attempts = %#v", client.mergeAttempts)
	}
	if strings.Join(client.merged, ",") != "acme/tool#1,acme/tool#3" {
		t.Fatalf("merged = %#v", client.merged)
	}
}

func TestCloseConfirmationDoesNotReuseMergeConfirmation(t *testing.T) {
	client := &recordingClient{}
	model := New(client)
	model.prs = []github.PullRequest{{Owner: "acme", Repo: "tool", Number: 42}}
	model.loading = false

	updated, _ := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("m")})
	_, cmd := updated.(Model).Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("d")})
	if cmd != nil {
		t.Fatal("d after m should ask for close confirmation, not close immediately")
	}
	if client.closed != "" {
		t.Fatalf("closed = %q", client.closed)
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

func (fakeClient) ClosePullRequest(context.Context, string, string, int) error {
	return nil
}

type recordingClient struct {
	fakeClient
	closed        string
	merged        []string
	mergeAttempts []string
	mergeErrors   map[string]error
}

func (c *recordingClient) Merge(_ context.Context, owner, repo string, number int) error {
	key := owner + "/" + repo + "#" + strconv.Itoa(number)
	c.mergeAttempts = append(c.mergeAttempts, key)
	if err := c.mergeErrors[key]; err != nil {
		return err
	}
	c.merged = append(c.merged, key)
	return nil
}

func (c *recordingClient) ClosePullRequest(_ context.Context, owner, repo string, number int) error {
	c.closed = owner + "/" + repo + "#" + strconv.Itoa(number)
	return nil
}
