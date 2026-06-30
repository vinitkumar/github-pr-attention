# GitHub PR Attention

A terminal inbox and report CLI for GitHub pull requests that need your attention.

`pr-attention` collects open pull requests where you are requested for review,
assigned, mentioned, or the author. Use the TUI for hands-on review, or emit a
text/JSON report for scripts, status checks, and daily planning.

## Screenshots

### Inbox

![Inbox view](screenshots/Screenshot%202026-05-08%20at%2010.58.43.png)

### PR detail

![PR detail view](screenshots/Screenshot%202026-05-08%20at%2010.49.27.png)

### Changed files

![Changed files view](screenshots/Screenshot%202026-05-08%20at%2010.49.35.png)

## Requirements

- Go 1.26+
- A GitHub token with access to the repositories you want to inspect

## Usage

```sh
export GITHUB_TOKEN=...
go run ./cmd/pr-attention
```

`GH_TOKEN` is also accepted.

### Report Mode

Use `--format text` when you want a quick inbox snapshot without opening the
interactive TUI:

```sh
go run ./cmd/pr-attention --format text --limit 10
```

Use `--format json` when another tool should consume the review inbox:

```sh
go run ./cmd/pr-attention --format json
```

## Build

```sh
make build
```

The build copies the self-contained `pr-attention` binary to `~/.local/bin`.
Override the install location with `LOCAL_BIN=/path/to/bin make build`.

## Keys

- `j` / `k` or arrow keys: move selection
- `/`: filter the PR list; press `enter` to apply, `ctrl+u` to clear while filtering
- `enter`: open PR details
- `esc`, `b`, `backspace`, or left arrow in detail view: return to the list
- `tab` in detail view: switch between PR description and changed files
- `j` / `k` in detail view: scroll PR description
- `r`: refresh inbox
- `o`: open selected PR in a browser
- `c`: add an issue comment
- `a`: approve the PR
- `x`: request changes
- `m`: merge the PR with squash merge
- `M`: bulk merge all listed PRs with squash merge
- `d`: close the PR without merging
- `esc`: go back or cancel compose mode
- `ctrl+s`: submit compose mode
- `q`: quit
