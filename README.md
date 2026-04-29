# GitHub PR Attention

A terminal inbox for GitHub pull requests that need your attention.

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
go build -o pr-attention ./cmd/pr-attention
```

The resulting `pr-attention` binary is self-contained.

## Keys

- `j` / `k` or arrow keys: move selection
- `enter`: open PR details
- `tab` in detail view: switch between PR description and changed files
- `j` / `k` in detail view: scroll PR description
- `r`: refresh inbox
- `o`: open selected PR in a browser
- `c`: add an issue comment
- `a`: approve the PR
- `x`: request changes
- `m`: merge the PR with squash merge
- `esc`: go back or cancel compose mode
- `ctrl+s`: submit compose mode
- `q`: quit
