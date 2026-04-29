package app

import (
	"context"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/lipgloss"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

type githubClient interface {
	ListAttentionPRs(context.Context) ([]github.PullRequest, error)
	GetPullRequest(context.Context, string, string, int) (github.PullRequestDetail, error)
	GetPullRequestFiles(context.Context, string, string, int) ([]github.PullRequestFile, error)
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

type detailTab int

const (
	tabDescription detailTab = iota
	tabChanges
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
	files      []github.PullRequestFile
	detailTab  detailTab
	width      int
	height     int
	loading    bool
	status     string
	err        error
	textarea   textarea.Model
	viewport   viewport.Model
	composing  composeAction
	confirming bool
}

type listLoadedMsg struct {
	prs []github.PullRequest
	err error
}

type detailLoadedMsg struct {
	detail github.PullRequestDetail
	files  []github.PullRequestFile
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
		m.viewport.Width = max(20, msg.Width-2)
		m.viewport.Height = max(4, msg.Height-4)
		if m.detail != nil {
			m.viewport.SetContent(m.renderDetailContent())
		}
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
		m.files = msg.files
		m.detailTab = tabDescription
		m.mode = modeDetail
		m.viewport.Width = max(20, m.width-2)
		m.viewport.Height = max(4, m.height-4)
		m.viewport.SetContent(m.renderDetailContent())
		m.viewport.GotoTop()
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
	if m.mode == modeDetail {
		var cmd tea.Cmd
		m.viewport, cmd = m.viewport.Update(msg)
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

	if m.mode == modeDetail {
		switch msg.String() {
		case "tab":
			m.toggleDetailTab()
			return m, nil
		case "esc":
			m.mode = modeList
			m.detail = nil
			m.files = nil
			m.status = fmt.Sprintf("%d PRs need attention", len(m.prs))
			return m, nil
		case "j", "down", "k", "up", "pgdown", "pgup", "home", "end":
			var cmd tea.Cmd
			m.viewport, cmd = m.viewport.Update(msg)
			return m, cmd
		}
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
	if m.mode == modeDetail {
		footer = helpStyle.Render("tab description/changes  j/k scroll  esc back  o open  c comment  a approve  x changes  m merge  q quit")
	}
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

	return "\n" + m.viewport.View()
}

func (m Model) renderDetailContent() string {
	if m.detail == nil {
		return ""
	}

	d := m.detail
	bodyWidth := max(32, m.viewport.Width-4)
	body := renderMarkdown(d.Body, bodyWidth)
	if strings.TrimSpace(body) == "" {
		body = mutedStyle.Render("No pull request description.")
	}

	header := lipgloss.JoinVertical(
		lipgloss.Left,
		detailRepoStyle.Render(fmt.Sprintf("%s #%d", d.FullName(), d.Number)),
		detailTitleStyle.Width(bodyWidth).Render(d.Title),
	)

	meta := lipgloss.JoinVertical(
		lipgloss.Left,
		metaLine("Author", d.Author, "State", d.State),
		metaLine("Base", d.BaseRef, "Head", d.HeadRef),
		metaLine("Review", emptyDash(d.ReviewDecision), "Draft", fmt.Sprint(d.Draft)),
		metaLine("CI", formatCIStatus(d.CIStatus), "Head SHA", shortSHA(d.HeadSHA)),
		metaLine("Changes", fmt.Sprintf("+%d -%d", d.Additions, d.Deletions), "Files", fmt.Sprint(d.ChangedFiles)),
	)

	content := body
	section := "Description"
	if m.detailTab == tabChanges {
		section = "Changes"
		content = renderFiles(m.files, bodyWidth)
	}

	return lipgloss.JoinVertical(
		lipgloss.Left,
		header,
		"",
		metaBoxStyle.Width(bodyWidth).Render(meta),
		"",
		linkStyle.Render(d.URL),
		"",
		renderTabs(m.detailTab),
		sectionStyle.Render(section),
		content,
	)
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
		if err != nil {
			return detailLoadedMsg{err: err}
		}
		files, err := m.client.GetPullRequestFiles(context.Background(), pr.Owner, pr.Repo, pr.Number)
		detail.Reasons = pr.Reasons
		return detailLoadedMsg{detail: detail, files: files, err: err}
	}
}

func (m *Model) toggleDetailTab() {
	if m.detailTab == tabDescription {
		m.detailTab = tabChanges
		m.status = fmt.Sprintf("%d changed files", len(m.files))
	} else {
		m.detailTab = tabDescription
		m.status = "Detail loaded"
	}
	m.viewport.SetContent(m.renderDetailContent())
	m.viewport.GotoTop()
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

func renderMarkdown(value string, width int) string {
	renderer, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(width),
	)
	if err != nil {
		return wrapText(value, width, 200)
	}
	rendered, err := renderer.Render(value)
	if err != nil {
		return wrapText(value, width, 200)
	}
	return strings.TrimRight(rendered, "\n")
}

func renderTabs(active detailTab) string {
	description := tabStyle.Render("Description")
	changes := tabStyle.Render("Changes")
	if active == tabDescription {
		description = activeTabStyle.Render("Description")
	} else {
		changes = activeTabStyle.Render("Changes")
	}
	return lipgloss.JoinHorizontal(lipgloss.Top, description, " ", changes)
}

func renderFiles(files []github.PullRequestFile, width int) string {
	if len(files) == 0 {
		return mutedStyle.Render("No changed files returned by GitHub.")
	}

	sections := make([]string, 0, len(files))
	for _, file := range files {
		header := fileHeaderStyle.Render(
			fmt.Sprintf("%s  %s  +%d -%d", file.Filename, file.Status, file.Additions, file.Deletions),
		)
		patch := renderPatch(file.Patch, width)
		if strings.TrimSpace(patch) == "" {
			patch = mutedStyle.Render("Binary file or patch too large to display.")
		}
		sections = append(sections, lipgloss.JoinVertical(lipgloss.Left, header, patch))
	}
	return strings.Join(sections, "\n\n")
}

func formatCIStatus(status github.CIStatus) string {
	state := string(status.State)
	if status.State == "" {
		state = string(github.CIStateUnknown)
	}
	if status.Summary == "" {
		return state
	}
	return state + " (" + status.Summary + ")"
}

func shortSHA(sha string) string {
	if sha == "" {
		return "-"
	}
	return truncate(sha, 7)
}

func renderPatch(patch string, width int) string {
	lines := strings.Split(strings.TrimRight(patch, "\n"), "\n")
	rendered := make([]string, 0, len(lines))
	for _, line := range lines {
		style := patchContextStyle
		switch {
		case strings.HasPrefix(line, "@@"):
			style = patchHunkStyle
		case strings.HasPrefix(line, "+"):
			style = patchAddStyle
		case strings.HasPrefix(line, "-"):
			style = patchDeleteStyle
		}
		rendered = append(rendered, style.Render(truncate(line, width)))
	}
	return strings.Join(rendered, "\n")
}

func metaLine(leftLabel, leftValue, rightLabel, rightValue string) string {
	left := labelStyle.Render(leftLabel+":") + " " + valueStyle.Render(leftValue)
	right := labelStyle.Render(rightLabel+":") + " " + valueStyle.Render(rightValue)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, strings.Repeat(" ", 4), right)
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
	titleStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	statusStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	errorStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	itemStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	selectedStyle     = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62"))
	helpStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	detailRepoStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("39"))
	detailTitleStyle  = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230"))
	metaBoxStyle      = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("238")).Padding(0, 1)
	labelStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	valueStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("252"))
	linkStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("33")).Underline(true)
	sectionStyle      = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("214"))
	mutedStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("244"))
	tabStyle          = lipgloss.NewStyle().Foreground(lipgloss.Color("245")).Padding(0, 1)
	activeTabStyle    = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("230")).Background(lipgloss.Color("62")).Padding(0, 1)
	fileHeaderStyle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("81"))
	patchHunkStyle    = lipgloss.NewStyle().Foreground(lipgloss.Color("183"))
	patchAddStyle     = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	patchDeleteStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
	patchContextStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
)
