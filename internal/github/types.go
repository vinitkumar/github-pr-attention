package github

import "time"

type AttentionReason string

const (
	ReasonReviewRequested AttentionReason = "review requested"
	ReasonAssigned        AttentionReason = "assigned"
	ReasonMentioned       AttentionReason = "mentioned"
	ReasonAuthored        AttentionReason = "authored"
)

type PullRequest struct {
	ID        int64
	Owner     string
	Repo      string
	Number    int
	Title     string
	URL       string
	Author    string
	Reasons   []AttentionReason
	UpdatedAt time.Time
	CreatedAt time.Time
}

func (p PullRequest) FullName() string {
	return p.Owner + "/" + p.Repo
}

type PullRequestDetail struct {
	PullRequest
	State          string
	Draft          bool
	Mergeable      *bool
	ReviewDecision string
	HeadRef        string
	BaseRef        string
	Additions      int
	Deletions      int
	ChangedFiles   int
	Body           string
}

type PullRequestFile struct {
	Filename  string
	Status    string
	Additions int
	Deletions int
	Changes   int
	Patch     string
}

type ReviewEvent string

const (
	ReviewApprove        ReviewEvent = "APPROVE"
	ReviewRequestChanges ReviewEvent = "REQUEST_CHANGES"
	ReviewComment        ReviewEvent = "COMMENT"
)
