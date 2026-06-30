package report

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"

	"github.com/vinitkumar/github-pr-attention/internal/github"
)

type JSONPullRequest struct {
	Owner     string   `json:"owner"`
	Repo      string   `json:"repo"`
	Number    int      `json:"number"`
	Title     string   `json:"title"`
	URL       string   `json:"url"`
	Author    string   `json:"author"`
	Reasons   []string `json:"reasons"`
	UpdatedAt string   `json:"updated_at"`
	CreatedAt string   `json:"created_at"`
}

func WriteText(w io.Writer, prs []github.PullRequest) error {
	if len(prs) == 0 {
		_, err := fmt.Fprintln(w, "Inbox clear. No open pull requests currently need your attention.")
		return err
	}

	table := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	if _, err := fmt.Fprintln(table, "REPOSITORY\tPR\tREASONS\tUPDATED\tTITLE"); err != nil {
		return err
	}
	for _, pr := range prs {
		if _, err := fmt.Fprintf(
			table,
			"%s\t#%d\t%s\t%s\t%s\n",
			pr.FullName(),
			pr.Number,
			reasons(pr.Reasons),
			pr.UpdatedAt.Format("2006-01-02"),
			pr.Title,
		); err != nil {
			return err
		}
	}
	return table.Flush()
}

func WriteJSON(w io.Writer, prs []github.PullRequest) error {
	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(toJSON(prs))
}

func toJSON(prs []github.PullRequest) []JSONPullRequest {
	out := make([]JSONPullRequest, 0, len(prs))
	for _, pr := range prs {
		out = append(out, JSONPullRequest{
			Owner:     pr.Owner,
			Repo:      pr.Repo,
			Number:    pr.Number,
			Title:     pr.Title,
			URL:       pr.URL,
			Author:    pr.Author,
			Reasons:   reasonStrings(pr.Reasons),
			UpdatedAt: pr.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
			CreatedAt: pr.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		})
	}
	return out
}

func reasons(input []github.AttentionReason) string {
	if len(input) == 0 {
		return "attention"
	}
	return strings.Join(reasonStrings(input), ", ")
}

func reasonStrings(input []github.AttentionReason) []string {
	out := make([]string, 0, len(input))
	for _, reason := range input {
		out = append(out, string(reason))
	}
	return out
}
