package main

import (
	"fmt"
	"os"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/vinitkumar/github-pr-attention/internal/app"
	"github.com/vinitkumar/github-pr-attention/internal/github"
)

func main() {
	token := os.Getenv("GITHUB_TOKEN")
	if token == "" {
		token = os.Getenv("GH_TOKEN")
	}
	if token == "" {
		fmt.Fprintln(os.Stderr, "set GITHUB_TOKEN or GH_TOKEN before running pr-attention")
		os.Exit(1)
	}

	client := github.NewClient(token)
	program := tea.NewProgram(app.New(client), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "pr-attention failed: %v\n", err)
		os.Exit(1)
	}
}
