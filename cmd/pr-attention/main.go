package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vinitkumar/github-pr-attention/internal/app"
	"github.com/vinitkumar/github-pr-attention/internal/github"
	"github.com/vinitkumar/github-pr-attention/internal/report"
)

type pullRequestLister interface {
	ListAttentionPRs(context.Context) ([]github.PullRequest, error)
}

func main() {
	format := flag.String("format", "tui", "output format: tui, text, or json")
	limit := flag.Int("limit", 0, "maximum number of pull requests to show in text or json output")
	flag.Parse()

	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "set GITHUB_TOKEN or GH_TOKEN before running pr-attention")
		os.Exit(1)
	}

	client := github.NewClient(token)
	if *format != "tui" {
		if err := runReport(context.Background(), os.Stdout, client, *format, *limit); err != nil {
			fmt.Fprintf(os.Stderr, "pr-attention report failed: %v\n", err)
			os.Exit(1)
		}
		return
	}

	program := tea.NewProgram(app.New(client), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "pr-attention failed: %v\n", err)
		os.Exit(1)
	}
}

func runReport(ctx context.Context, w io.Writer, client pullRequestLister, format string, limit int) error {
	prs, err := client.ListAttentionPRs(ctx)
	if err != nil {
		return err
	}
	prs = limitPullRequests(prs, limit)

	switch format {
	case "text":
		return report.WriteText(w, prs)
	case "json":
		return report.WriteJSON(w, prs)
	default:
		return fmt.Errorf("unsupported output format %q", format)
	}
}

func limitPullRequests(prs []github.PullRequest, limit int) []github.PullRequest {
	if limit <= 0 || limit >= len(prs) {
		return prs
	}
	return prs[:limit]
}
