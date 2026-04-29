package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

type githubClient interface {
	ListAttentionPRs(context.Context) ([]github.PullRequest, error)
	GetPullRequest(context.Context, string, string, int) (github.PullRequestDetail, error)
	AddComment(context.Context, string, string, int, string) error
	SubmitReview(context.Context, string, string, int, github.ReviewEvent, string) error
	Merge(context.Context, string, string, int) error
}

type viewMode int

const (
	modeList viewMode = iota
	modeDetail
	modeCompose
)

type composeAction int

const (
	composeComment composeAction = iota
	composeApprove
	composeRequestChanges
)

type Model struct {
	client githubClient

	mode       viewMode
	prs        []github.PullRequest
	selected   int
	detail     *github.PullRequestDetail
	width      int
	height     int
	loading    bool
	status     string
	err        error
	textarea   textarea.Model
	composing  composeAction
	confirming bool
}

type listLoadedMsg struct {
	prs []github.PullRequest
	err error
}

type detailLoadedMsg struct {
	detail github.PullRequestDetail
	err    error
}

type actionDoneMsg struct {
	message string
	err     error
	refresh bool
}

func New(client githubClient) Model {
	input := textarea.New()
	input.Placeholder = "Write your response..."
	input.CharLimit = 8000
	input.SetHeight(8)
	input.ShowLineNumbers = false

	return Model{
		client:   client,
		mode:     modeList,
		loading:  true,
		status:   "Loading pull requests...",
		textarea: input,
	}
}

func (m Model) Init() tea.Cmd {
	return m.loadList()
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.textarea.SetWidth(max(20, msg.Width-6))
		return m, nil
	case listLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.status = "Refresh failed"
			return m, nil
		}
		m.prs = msg.prs
		if m.selected >= len(m.prs) {
			m.selected = max(0, len(m.prs)-1)
		}
		m.status = fmt.Sprintf("%d PRs need attention", len(m.prs))
		return m, nil
	case detailLoadedMsg:
		m.loading = false
		m.err = msg.err
		if msg.err != nil {
			m.status = "Could not load PR detail"
			return m, nil
		}
		m.detail = &msg.detail
		m.mode = modeDetail
		m.status = "Detail loaded"
		return m, nil
	case actionDoneMsg:
		m.loading = false
		m.err = msg.err
		m.confirming = false
		if msg.err != nil {
			m.status = "Action failed"
			return m, nil
		}
		m.status = msg.message
		if msg.refresh {
			m.loading = true
			return m, m.loadList()
		}
		return m, nil
	case tea.KeyMsg:
		return m.updateKey(msg)
	}

	if m.mode == modeCompose {
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m Model) updateKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.mode == modeCompose {
		switch msg.String() {
		case "esc":
			m.mode = modeDetail
			m.status = "Compose cancelled"
			m.textarea.Reset()
			return m, nil
		case "ctrl+s":
			return m.submitCompose()
		}
		var cmd tea.Cmd
		m.textarea, cmd = m.textarea.Update(msg)
		return m, cmd
	}

	switch msg.String() {
	case "q", "ctrl+c":
		return m, tea.Quit
	case "r":
		m.loading = true
		m.status = "Refreshing..."
		return m, m.loadList()
	case "j", "down":
		if m.selected < len(m.prs)-1 {
			m.selected++
		}
	case "k", "up":
		if m.selected > 0 {
			m.selected--
		}
	case "enter":
		if pr, ok := m.currentPR(); ok {
			m.loading = true
			m.status = "Loading detail..."
			return m, m.loadDetail(pr)
		}
	case "esc":
		if m.mode == modeDetail {
			m.mode = modeList
			m.detail = nil
			m.status = fmt.Sprintf("%d PRs need attention", len(m.prs))
		}
	case "o":
		if pr, ok := m.activePR(); ok {
			return m, openBrowser(pr.URL)
		}
	case "c":
		if _, ok := m.activePR(); ok {
			return m.startCompose(composeComment, "Comment")
		}
	case "a":
		if _, ok := m.activePR(); ok {
			return m.startCompose(composeApprove, "Approve")
		}
	case "x":
		if _, ok := m.activePR(); ok {
			return m.startCompose(composeRequestChanges, "Request changes")
		}
	case "m":
		if pr, ok := m.activePR(); ok {
			if !m.confirming {
				m.confirming = true
				m.status = "Press m again to squash merge"
				return m, nil
			}
			m.loading = true
			m.status = "Merging..."
			return m, m.merge(pr)
		}
	default:
		m.confirming = false
	}
	return m, nil
}

func (m Model) View() string {
	if m.width == 0 {
		return "Loading..."
	}

	header := titleStyle.Render("GitHub PR Attention") + " " + statusStyle.Render(m.status)
	if m.err != nil {
		header += "\n" + errorStyle.Render(m.err.Error())
	}

	body := ""
	switch m.mode {
	case modeList:
		body = m.listView()
	case modeDetail:
		body = m.detailView()
	case modeCompose:
		body = m.composeView()
	}

	footer := helpStyle.Render("j/k move  enter detail  r refresh  o open  c comment  a approve  x changes  m merge  q quit")
	if m.mode == modeCompose {
		footer = helpStyle.Render("ctrl+s submit  esc cancel")
	}

	return lipgloss.JoinVertical(lipgloss.Left, header, body, footer)
}

func (m Model) listView() string {
	if m.loading && len(m.prs) == 0 {
		return "\n  Loading..."
	}
	if len(m.prs) == 0 {
		return "\n  No open pull requests currently need your attention."
	}

	available := max(4, m.height-5)
	start := clamp(m.selected-available/2, 0, max(0, len(m.prs)-available))
	end := min(len(m.prs), start+available)

	lines := make([]string, 0, end-start)
	for i := start; i < end; i++ {
		pr := m.prs[i]
		prefix := "  "
		style := itemStyle
		if i == m.selected {
			prefix = "> "
			style = selectedStyle
		}
		line := fmt.Sprintf("%s%s #%d  %s  [%s]", prefix, pr.FullName(), pr.Number, pr.Title, reasons(pr.Reasons))
		lines = append(lines, style.Width(max(20, m.width-2)).Render(truncate(line, max(20, m.width-2))))
	}
	return "\n" + strings.Join(lines, "\n")
}

func (m Model) detailView() string {
	if m.loading && m.detail == nil {
		return "\n  Loading detail..."
	}
	if m.detail == nil {
		return "\n  No PR selected."
	}

	d := m.detail
	lines := []string{
		"",
		selectedStyle.Render(fmt.Sprintf("%s #%d", d.FullName(), d.Number)),
		d.Title,
		fmt.Sprintf("author: %s  base: %s  head: %s", d.Author, d.BaseRef, d.HeadRef),
		fmt.Sprintf("state: %s  draft: %t  review: %s", d.State, d.Draft, emptyDash(d.ReviewDecision)),
		fmt.Sprintf("changes: +%d -%d in %d files", d.Additions, d.Deletions, d.ChangedFiles),
		d.URL,
		"",
		wrapText(d.Body, max(30, m.width-4), max(6, m.height-12)),
	}
	return strings.Join(lines, "\n")
}

func (m Model) composeView() string {
	label := "Comment"
	if m.composing == composeApprove {
		label = "Approve"
	}
	if m.composing == composeRequestChanges {
		label = "Request changes"
	}
	return "\n" + selectedStyle.Render(label) + "\n" + m.textarea.View()
}

func (m Model) startCompose(action composeAction, label string) (tea.Model, tea.Cmd) {
	m.mode = modeCompose
	m.composing = action
	m.status = label + " compose"
	m.textarea.Reset()
	m.textarea.Focus()
	return m, textarea.Blink
}

func (m Model) submitCompose() (tea.Model, tea.Cmd) {
	pr, ok := m.activePR()
	if !ok {
		return m, nil
	}
	body := m.textarea.Value()
	m.mode = modeDetail
	m.textarea.Reset()
	m.loading = true

	switch m.composing {
	case composeComment:
		m.status = "Posting comment..."
		return m, m.comment(pr, body)
	case composeApprove:
		m.status = "Submitting approval..."
		return m, m.review(pr, github.ReviewApprove, body)
	case composeRequestChanges:
		m.status = "Requesting changes..."
		return m, m.review(pr, github.ReviewRequestChanges, body)
	default:
		m.loading = false
		return m, nil
	}
}

func (m Model) loadList() tea.Cmd {
	return func() tea.Msg {
		prs, err := m.client.ListAttentionPRs(context.Background())
		return listLoadedMsg{prs: prs, err: err}
	}
}

func (m Model) loadDetail(pr github.PullRequest) tea.Cmd {
	return func() tea.Msg {
		detail, err := m.client.GetPullRequest(context.Background(), pr.Owner, pr.Repo, pr.Number)
		detail.Reasons = pr.Reasons
		return detailLoadedMsg{detail: detail, err: err}
	}
}

func (m Model) comment(pr github.PullRequest, body string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.AddComment(context.Background(), pr.Owner, pr.Repo, pr.Number, body)
		return actionDoneMsg{message: "Comment posted", err: err}
	}
}

func (m Model) review(pr github.PullRequest, event github.ReviewEvent, body string) tea.Cmd {
	return func() tea.Msg {
		err := m.client.SubmitReview(context.Background(), pr.Owner, pr.Repo, pr.Number, event, body)
		return actionDoneMsg{message: "Review submitted", err: err, refresh: true}
	}
}

func (m Model) merge(pr github.PullRequest) tea.Cmd {
	return func() tea.Msg {
		err := m.client.Merge(context.Background(), pr.Owner, pr.Repo, pr.Number)
		return actionDoneMsg{message: "PR merged", err: err, refresh: true}
	}
}

func (m Model) currentPR() (github.PullRequest, bool) {
	if m.selected < 0 || m.selected >= len(m.prs) {
		return github.PullRequest{}, false
	}
	return m.prs[m.selected], true
}

func (m Model) activePR() (github.PullRequest, bool) {
	if m.detail != nil {
		return m.detail.PullRequest, true
	}
	return m.currentPR()
}

func openBrowser(target string) tea.Cmd {
	return func() tea.Msg {
		var command string
		var args []string
		switch runtime.GOOS {
		case "darwin":
			command = "open"
			args = []string{target}
		case "windows":
			command = "rundll32"
			args = []string{"url.dll,FileProtocolHandler", target}
		default:
			command = "xdg-open"
			args = []string{target}
		}
		err := exec.Command(command, args...).Start()
		return actionDoneMsg{message: "Opened browser", err: err}
	}
}

func reasons(input []github.AttentionReason) string {
	parts := make([]string, len(input))
	for i, reason := range input {
		parts[i] = string(reason)
	}
	return strings.Join(parts, ", ")
}

func emptyDash(value string) string {
	if value == "" {
		return "-"
	}
	return value
}

func wrapText(value string, width int, maxLines int) string {
	words := strings.Fields(value)
	if len(words) == 0 {
		return ""
	}
	lines := []string{}
	line := ""
	for _, word := range words {
		if len(line)+len(word)+1 > width {
			lines = append(lines, line)
			line = word
			if len(lines) >= maxLines {
				return strings.Join(lines, "\n")
			}
			continue
		}
		if line == "" {
			line = word
		} else {
			line += " " + word
		}
	}
	if line != "" && len(lines) < maxLines {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func truncate(value string, width int) string {
	if len(value) <= width {
		return value
	}
	if width <= 3 {
		return value[:width]
	}
	return value[:width-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(value, low, high int) int {
	return max(low, min(value, high))
}

var (
	titleStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle   = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	itemStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	helpStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
)
