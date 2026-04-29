package github

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"sort"
	"strings"
	"time"
)

const defaultBaseURL = "https://api.github.com"
const searchPageLimit = 10
const pullFilesPageLimit = 10

type Client struct {
	baseURL    string
	httpClient *http.Client
	token      string
}

func NewClient(token string) *Client {
	return &Client{
		baseURL:    defaultBaseURL,
		httpClient: &http.Client{Timeout: 20 * time.Second},
		token:      token,
	}
}

func NewClientWithBaseURL(token, baseURL string, httpClient *http.Client) *Client {
	if httpClient == nil {
		httpClient = &http.Client{Timeout: 20 * time.Second}
	}
	return &Client{baseURL: strings.TrimRight(baseURL, "/"), httpClient: httpClient, token: token}
}

type searchSpec struct {
	Query  string
	Reason AttentionReason
}

func attentionSearches() []searchSpec {
	return []searchSpec{
		{Query: "is:pr is:open archived:false review-requested:@me", Reason: ReasonReviewRequested},
		{Query: "is:pr is:open archived:false assignee:@me", Reason: ReasonAssigned},
		{Query: "is:pr is:open archived:false mentions:@me", Reason: ReasonMentioned},
		{Query: "is:pr is:open archived:false author:@me", Reason: ReasonAuthored},
	}
}

func (c *Client) ListAttentionPRs(ctx context.Context) ([]PullRequest, error) {
	byKey := map[string]*PullRequest{}

	for _, spec := range attentionSearches() {
		items, err := c.searchPRs(ctx, spec)
		if err != nil {
			return nil, err
		}
		for _, item := range items {
			key := fmt.Sprintf("%s/%s#%d", item.Owner, item.Repo, item.Number)
			existing, ok := byKey[key]
			if !ok {
				pr := item
				byKey[key] = &pr
				continue
			}
			existing.Reasons = mergeReasons(existing.Reasons, item.Reasons)
		}
	}

	prs := make([]PullRequest, 0, len(byKey))
	for _, pr := range byKey {
		prs = append(prs, *pr)
	}
	sort.Slice(prs, func(i, j int) bool {
		return prs[i].UpdatedAt.After(prs[j].UpdatedAt)
	})
	return prs, nil
}

func mergeReasons(current []AttentionReason, next []AttentionReason) []AttentionReason {
	seen := map[AttentionReason]bool{}
	out := make([]AttentionReason, 0, len(current)+len(next))
	for _, reason := range append(current, next...) {
		if seen[reason] {
			continue
		}
		seen[reason] = true
		out = append(out, reason)
	}
	return out
}

func (c *Client) searchPRs(ctx context.Context, spec searchSpec) ([]PullRequest, error) {
	prs := []PullRequest{}
	for page := 1; page <= searchPageLimit; page++ {
		endpoint, err := url.Parse(c.baseURL + "/search/issues")
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("q", spec.Query)
		query.Set("sort", "updated")
		query.Set("order", "desc")
		query.Set("per_page", "100")
		query.Set("page", fmt.Sprint(page))
		endpoint.RawQuery = query.Encode()

		var response searchIssuesResponse
		if err := c.do(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
			return nil, fmt.Errorf("search %q: %w", spec.Query, err)
		}

		for _, item := range response.Items {
			if item.PullRequest == nil {
				continue
			}
			owner, repo := parseRepoURL(item.RepositoryURL)
			prs = append(prs, PullRequest{
				ID:        item.ID,
				Owner:     owner,
				Repo:      repo,
				Number:    item.Number,
				Title:     item.Title,
				URL:       item.HTMLURL,
				Author:    item.User.Login,
				Reasons:   []AttentionReason{spec.Reason},
				UpdatedAt: item.UpdatedAt,
				CreatedAt: item.CreatedAt,
			})
		}
		if len(response.Items) < 100 {
			break
		}
	}
	return prs, nil
}

func (c *Client) GetPullRequest(ctx context.Context, owner, repo string, number int) (PullRequestDetail, error) {
	endpoint := c.repoEndpoint(owner, repo, "pulls", fmt.Sprint(number))
	var response pullResponse
	if err := c.do(ctx, http.MethodGet, endpoint, nil, &response); err != nil {
		return PullRequestDetail{}, err
	}
	ciStatus, err := c.GetCIStatus(ctx, owner, repo, response.Head.SHA)
	if err != nil {
		ciStatus = CIStatus{State: CIStateUnknown, Summary: "CI unavailable: " + err.Error()}
	}

	return PullRequestDetail{
		PullRequest: PullRequest{
			ID:        response.ID,
			Owner:     owner,
			Repo:      repo,
			Number:    response.Number,
			Title:     response.Title,
			URL:       response.HTMLURL,
			Author:    response.User.Login,
			UpdatedAt: response.UpdatedAt,
			CreatedAt: response.CreatedAt,
		},
		State:          response.State,
		Draft:          response.Draft,
		Mergeable:      response.Mergeable,
		ReviewDecision: response.ReviewDecision,
		HeadSHA:        response.Head.SHA,
		HeadRef:        response.Head.Ref,
		BaseRef:        response.Base.Ref,
		Additions:      response.Additions,
		Deletions:      response.Deletions,
		ChangedFiles:   response.ChangedFiles,
		Body:           response.Body,
		CIStatus:       ciStatus,
	}, nil
}

func (c *Client) GetCIStatus(ctx context.Context, owner, repo, sha string) (CIStatus, error) {
	if sha == "" {
		return CIStatus{State: CIStateUnknown, Summary: "No head SHA"}, nil
	}

	var statuses combinedStatusResponse
	if err := c.do(ctx, http.MethodGet, c.repoEndpoint(owner, repo, "commits", sha, "status"), nil, &statuses); err != nil {
		return CIStatus{}, err
	}

	var checks checkRunsResponse
	checkRunsEndpoint, err := url.Parse(c.repoEndpoint(owner, repo, "commits", sha, "check-runs"))
	if err != nil {
		return CIStatus{}, err
	}
	query := checkRunsEndpoint.Query()
	query.Set("per_page", "100")
	checkRunsEndpoint.RawQuery = query.Encode()
	if err := c.do(ctx, http.MethodGet, checkRunsEndpoint.String(), nil, &checks); err != nil {
		return CIStatus{}, err
	}

	return summarizeCI(statuses, checks), nil
}

func summarizeCI(statuses combinedStatusResponse, checks checkRunsResponse) CIStatus {
	items := make([]CICheck, 0, len(statuses.Statuses)+len(checks.CheckRuns))
	for _, status := range statuses.Statuses {
		items = append(items, CICheck{
			Name:       status.Context,
			Status:     status.State,
			Conclusion: status.State,
			DetailsURL: status.TargetURL,
		})
	}
	for _, check := range checks.CheckRuns {
		items = append(items, CICheck{
			Name:        check.Name,
			Status:      check.Status,
			Conclusion:  check.Conclusion,
			DetailsURL:  check.DetailsURL,
			CompletedAt: check.CompletedAt,
		})
	}

	if len(items) == 0 {
		return CIStatus{State: CIStateUnknown, Summary: "No checks reported", Checks: items}
	}

	counts := map[CIState]int{}
	for _, item := range items {
		counts[ciStateForCheck(item)]++
	}

	state := CIStateSuccess
	if counts[CIStateFailure] > 0 {
		state = CIStateFailure
	} else if counts[CIStatePending] > 0 {
		state = CIStatePending
	} else if counts[CIStateUnknown] > 0 {
		state = CIStateUnknown
	}

	summaryParts := []string{}
	if counts[CIStateSuccess] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d passed", counts[CIStateSuccess]))
	}
	if counts[CIStatePending] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d pending", counts[CIStatePending]))
	}
	if counts[CIStateFailure] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d failed", counts[CIStateFailure]))
	}
	if counts[CIStateUnknown] > 0 {
		summaryParts = append(summaryParts, fmt.Sprintf("%d unknown", counts[CIStateUnknown]))
	}

	return CIStatus{State: state, Summary: strings.Join(summaryParts, ", "), Checks: items}
}

func ciStateForCheck(check CICheck) CIState {
	status := strings.ToLower(check.Status)
	conclusion := strings.ToLower(check.Conclusion)
	switch {
	case status == "pending" || status == "queued" || status == "in_progress":
		return CIStatePending
	case conclusion == "success" || conclusion == "neutral" || conclusion == "skipped":
		return CIStateSuccess
	case conclusion == "pending":
		return CIStatePending
	case conclusion == "failure" || conclusion == "timed_out" || conclusion == "cancelled" || conclusion == "action_required" || conclusion == "startup_failure" || status == "failure" || status == "error":
		return CIStateFailure
	default:
		return CIStateUnknown
	}
}

func (c *Client) GetPullRequestFiles(ctx context.Context, owner, repo string, number int) ([]PullRequestFile, error) {
	files := []PullRequestFile{}
	for page := 1; page <= pullFilesPageLimit; page++ {
		endpoint, err := url.Parse(c.repoEndpoint(owner, repo, "pulls", fmt.Sprint(number), "files"))
		if err != nil {
			return nil, err
		}
		query := endpoint.Query()
		query.Set("per_page", "100")
		query.Set("page", fmt.Sprint(page))
		endpoint.RawQuery = query.Encode()

		var response []pullFileResponse
		if err := c.do(ctx, http.MethodGet, endpoint.String(), nil, &response); err != nil {
			return nil, err
		}
		for _, item := range response {
			files = append(files, PullRequestFile{
				Filename:  item.Filename,
				Status:    item.Status,
				Additions: item.Additions,
				Deletions: item.Deletions,
				Changes:   item.Changes,
				Patch:     item.Patch,
			})
		}
		if len(response) < 100 {
			break
		}
	}
	return files, nil
}

func (c *Client) AddComment(ctx context.Context, owner, repo string, number int, body string) error {
	body = strings.TrimSpace(body)
	if body == "" {
		return errors.New("comment body cannot be empty")
	}
	payload := map[string]string{"body": body}
	return c.do(ctx, http.MethodPost, c.repoEndpoint(owner, repo, "issues", fmt.Sprint(number), "comments"), payload, nil)
}

func (c *Client) SubmitReview(ctx context.Context, owner, repo string, number int, event ReviewEvent, body string) error {
	if event != ReviewApprove && event != ReviewRequestChanges && event != ReviewComment {
		return fmt.Errorf("unsupported review event %q", event)
	}
	body = strings.TrimSpace(body)
	if event == ReviewRequestChanges && body == "" {
		return errors.New("requesting changes requires a body")
	}
	payload := map[string]string{"event": string(event)}
	if body != "" {
		payload["body"] = body
	}
	return c.do(ctx, http.MethodPost, c.repoEndpoint(owner, repo, "pulls", fmt.Sprint(number), "reviews"), payload, nil)
}

func (c *Client) Merge(ctx context.Context, owner, repo string, number int) error {
	payload := map[string]string{"merge_method": "squash"}
	return c.do(ctx, http.MethodPut, c.repoEndpoint(owner, repo, "pulls", fmt.Sprint(number), "merge"), payload, nil)
}

func (c *Client) ClosePullRequest(ctx context.Context, owner, repo string, number int) error {
	payload := map[string]string{"state": "closed"}
	return c.do(ctx, http.MethodPatch, c.repoEndpoint(owner, repo, "pulls", fmt.Sprint(number)), payload, nil)
}

func (c *Client) repoEndpoint(owner, repo string, parts ...string) string {
	all := append([]string{"repos", owner, repo}, parts...)
	return c.baseURL + "/" + path.Join(all...)
}

func (c *Client) do(ctx context.Context, method, endpoint string, payload any, output any) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}

	request, err := http.NewRequestWithContext(ctx, method, endpoint, body)
	if err != nil {
		return err
	}
	request.Header.Set("Accept", "application/vnd.github+json")
	request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if payload != nil {
		request.Header.Set("Content-Type", "application/json")
	}
	if c.token != "" {
		request.Header.Set("Authorization", "Bearer "+c.token)
	}

	response, err := c.httpClient.Do(request)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode < 200 || response.StatusCode > 299 {
		data, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		responseBody := strings.TrimSpace(string(data))
		if responseBody == "" {
			responseBody = "<empty>"
		}
		return fmt.Errorf("github api %s %s failed: %s\nresponse: %s", method, endpoint, response.Status, responseBody)
	}

	if output == nil {
		_, _ = io.Copy(io.Discard, response.Body)
		return nil
	}
	return json.NewDecoder(response.Body).Decode(output)
}

func parseRepoURL(repoURL string) (string, string) {
	parsed, err := url.Parse(repoURL)
	if err != nil {
		return "", ""
	}
	parts := strings.Split(strings.Trim(parsed.Path, "/"), "/")
	if len(parts) < 3 {
		return "", ""
	}
	return parts[len(parts)-2], parts[len(parts)-1]
}

type searchIssuesResponse struct {
	Items []searchIssue `json:"items"`
}

type searchIssue struct {
	ID            int64      `json:"id"`
	Number        int        `json:"number"`
	Title         string     `json:"title"`
	HTMLURL       string     `json:"html_url"`
	RepositoryURL string     `json:"repository_url"`
	User          searchUser `json:"user"`
	PullRequest   *struct{}  `json:"pull_request"`
	UpdatedAt     time.Time  `json:"updated_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

type searchUser struct {
	Login string `json:"login"`
}

type pullResponse struct {
	ID             int64      `json:"id"`
	Number         int        `json:"number"`
	Title          string     `json:"title"`
	HTMLURL        string     `json:"html_url"`
	User           searchUser `json:"user"`
	State          string     `json:"state"`
	Draft          bool       `json:"draft"`
	Mergeable      *bool      `json:"mergeable"`
	ReviewDecision string     `json:"review_decision"`
	Head           refObject  `json:"head"`
	Base           refObject  `json:"base"`
	Additions      int        `json:"additions"`
	Deletions      int        `json:"deletions"`
	ChangedFiles   int        `json:"changed_files"`
	Body           string     `json:"body"`
	UpdatedAt      time.Time  `json:"updated_at"`
	CreatedAt      time.Time  `json:"created_at"`
}

type refObject struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

type pullFileResponse struct {
	Filename  string `json:"filename"`
	Status    string `json:"status"`
	Additions int    `json:"additions"`
	Deletions int    `json:"deletions"`
	Changes   int    `json:"changes"`
	Patch     string `json:"patch"`
}

type combinedStatusResponse struct {
	State    string           `json:"state"`
	Statuses []statusResponse `json:"statuses"`
}

type statusResponse struct {
	Context   string `json:"context"`
	State     string `json:"state"`
	TargetURL string `json:"target_url"`
}

type checkRunsResponse struct {
	CheckRuns []checkRunResponse `json:"check_runs"`
}

type checkRunResponse struct {
	Name        string     `json:"name"`
	Status      string     `json:"status"`
	Conclusion  string     `json:"conclusion"`
	DetailsURL  string     `json:"details_url"`
	CompletedAt *time.Time `json:"completed_at"`
}
