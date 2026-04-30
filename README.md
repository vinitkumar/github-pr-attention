# GitHub PR Attention

A terminal inbox for GitHub pull requests that need your attention.

[![asciicast](https://asciinema.org/a/yc2GopjUrsbSgD3b.svg)](https://asciinema.org/a/yc2GopjUrsbSgD3b)

## Requirements

- Go 1.26+
- A GitHub token with access to the repositories you want to inspect

## Usage

```sh
export GITHUB_TOKEN=...
go run ./cmd/pr-attention
```

`GH_TOKEN` is also accepted.

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
