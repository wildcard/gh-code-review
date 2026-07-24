package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/cli/go-gh/v2/pkg/auth"
	"github.com/wildcard/gh-code-review/internal/diff"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

const APIVersion = "2026-03-10"

type Client struct {
	Host    string
	REST    *api.RESTClient
	GraphQL *api.GraphQLClient
}

type PullRequest struct {
	Number  int    `json:"number"`
	NodeID  string `json:"node_id"`
	Title   string `json:"title"`
	State   string `json:"state"`
	Draft   bool   `json:"draft"`
	HTMLURL string `json:"html_url"`
	Head    struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"head"`
	Base struct {
		SHA string `json:"sha"`
		Ref string `json:"ref"`
	} `json:"base"`
	User struct {
		Login string `json:"login"`
	} `json:"user"`
}

type User struct {
	Login string `json:"login"`
}

type Repository struct {
	FullName    string `json:"full_name"`
	Permissions struct {
		Admin    bool `json:"admin"`
		Maintain bool `json:"maintain"`
		Push     bool `json:"push"`
		Triage   bool `json:"triage"`
		Pull     bool `json:"pull"`
	} `json:"permissions"`
}

type Review struct {
	ID          int64  `json:"id"`
	NodeID      string `json:"node_id"`
	State       string `json:"state"`
	Body        string `json:"body"`
	CommitID    string `json:"commit_id"`
	HTMLURL     string `json:"html_url"`
	SubmittedAt string `json:"submitted_at"`
	User        User   `json:"user"`
}

type ReviewComment struct {
	ID        int64  `json:"id"`
	NodeID    string `json:"node_id"`
	Path      string `json:"path"`
	Body      string `json:"body"`
	CommitID  string `json:"commit_id"`
	Line      int    `json:"line"`
	Side      string `json:"side"`
	StartLine int    `json:"start_line"`
	StartSide string `json:"start_side"`
	InReplyTo int64  `json:"in_reply_to_id"`
	HTMLURL   string `json:"html_url"`
	Position  *int   `json:"position"`
	User      User   `json:"user"`
}

type CheckRun struct {
	ID         int64  `json:"id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Conclusion string `json:"conclusion"`
	HTMLURL    string `json:"html_url"`
}

type CheckRuns struct {
	TotalCount int        `json:"total_count"`
	CheckRuns  []CheckRun `json:"check_runs"`
}

type Status struct {
	State    string `json:"state"`
	Contexts []struct {
		Context string `json:"context"`
		State   string `json:"state"`
		Target  string `json:"target_url"`
	} `json:"statuses"`
}

type Thread struct {
	ID         string          `json:"id"`
	Path       string          `json:"path"`
	Line       int             `json:"line,omitempty"`
	StartLine  int             `json:"start_line,omitempty"`
	Side       string          `json:"side,omitempty"`
	IsResolved bool            `json:"is_resolved"`
	IsOutdated bool            `json:"is_outdated"`
	Subject    string          `json:"subject_type"`
	Comments   []ThreadComment `json:"comments"`
}

type ThreadComment struct {
	ID         string `json:"id"`
	DatabaseID int64  `json:"database_id"`
	Body       string `json:"body"`
	URL        string `json:"url"`
	Author     string `json:"author"`
}

func New(host string) (*Client, error) {
	if host == "" {
		host, _ = auth.DefaultHost()
	}
	token, source := auth.TokenForHost(host)
	if token == "" {
		return nil, output.NewError(output.ExitAuth, "AUTHENTICATION", fmt.Sprintf("no GitHub token found for %s (%s)", host, source), false, nil)
	}
	options := api.ClientOptions{
		Host:      host,
		AuthToken: token,
		Timeout:   30 * time.Second,
		Headers: map[string]string{
			"X-GitHub-Api-Version": APIVersion,
			"Accept":               "application/vnd.github+json",
			"User-Agent":           "gh-code-review",
		},
	}
	return NewWithOptions(options)
}

func NewWithOptions(options api.ClientOptions) (*Client, error) {
	rest, err := api.NewRESTClient(options)
	if err != nil {
		return nil, err
	}
	graphQL, err := api.NewGraphQLClient(options)
	if err != nil {
		return nil, err
	}
	return &Client{Host: options.Host, REST: rest, GraphQL: graphQL}, nil
}

func SplitRepository(repository string) (string, string, error) {
	parts := strings.Split(repository, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("repository must be owner/name")
	}
	return parts[0], parts[1], nil
}

func (c *Client) GetPull(repository string, number int) (*PullRequest, error) {
	var result PullRequest
	if err := c.get(fmt.Sprintf("repos/%s/pulls/%d", repository, number), &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) GetRepository(repository string) (*Repository, error) {
	var result Repository
	if err := c.get("repos/"+repository, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) CurrentUser() (*User, error) {
	var result User
	if err := c.get("user", &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) ListFiles(repository string, number int, includePatch bool) ([]diff.File, error) {
	var raw []struct {
		Filename         string `json:"filename"`
		PreviousFilename string `json:"previous_filename"`
		Status           string `json:"status"`
		Additions        int    `json:"additions"`
		Deletions        int    `json:"deletions"`
		Changes          int    `json:"changes"`
		Patch            string `json:"patch"`
	}
	if err := c.paginate(fmt.Sprintf("repos/%s/pulls/%d/files?per_page=100", repository, number), &raw); err != nil {
		return nil, classifyAPIError(err)
	}
	files := make([]diff.File, 0, len(raw))
	for _, file := range raw {
		patch := file.Patch
		binary := patch == "" && file.Changes > 0 && file.Additions == 0 && file.Deletions == 0
		truncated := (!binary && patch == "" && file.Changes > 0) || strings.HasSuffix(patch, "\n...") || len(patch) >= 65535
		if !includePatch {
			patch = ""
		}
		files = append(files, diff.File{
			Path: file.Filename, PreviousPath: file.PreviousFilename, Status: file.Status,
			Additions: file.Additions, Deletions: file.Deletions, Changes: file.Changes,
			Patch: patch, Binary: binary, Truncated: truncated,
		})
	}
	return files, nil
}

func (c *Client) ListReviews(repository string, number int) ([]Review, error) {
	result := []Review{}
	if err := c.paginate(fmt.Sprintf("repos/%s/pulls/%d/reviews?per_page=100", repository, number), &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return result, nil
}

func (c *Client) GetReview(repository string, number int, reviewID int64) (*Review, error) {
	var result Review
	if err := c.get(fmt.Sprintf("repos/%s/pulls/%d/reviews/%d", repository, number, reviewID), &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) ListReviewComments(repository string, number int) ([]ReviewComment, error) {
	result := []ReviewComment{}
	if err := c.paginate(fmt.Sprintf("repos/%s/pulls/%d/comments?per_page=100", repository, number), &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return result, nil
}

func (c *Client) Checks(repository, sha string) (*CheckRuns, *Status, error) {
	checks := CheckRuns{CheckRuns: []CheckRun{}}
	for page := 1; ; page++ {
		var batch CheckRuns
		if err := c.get(fmt.Sprintf("repos/%s/commits/%s/check-runs?per_page=100&page=%d", repository, sha, page), &batch); err != nil {
			return nil, nil, classifyAPIError(err)
		}
		checks.TotalCount = batch.TotalCount
		checks.CheckRuns = append(checks.CheckRuns, batch.CheckRuns...)
		if len(batch.CheckRuns) < 100 {
			break
		}
	}
	var status Status
	if err := c.get(fmt.Sprintf("repos/%s/commits/%s/status", repository, sha), &status); err != nil {
		return nil, nil, classifyAPIError(err)
	}
	return &checks, &status, nil
}

func (c *Client) CreatePendingReview(repository string, number int, sha string, comments []model.APIComment) (*Review, error) {
	payload := map[string]interface{}{"commit_id": sha}
	if len(comments) > 0 {
		payload["comments"] = comments
	}
	var result Review
	if err := c.write(http.MethodPost, fmt.Sprintf("repos/%s/pulls/%d/reviews", repository, number), payload, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) SubmitReview(repository string, number int, reviewID int64, event model.Event, body string) (*Review, error) {
	var result Review
	payload := map[string]interface{}{"event": event, "body": body}
	if err := c.write(http.MethodPost, fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/events", repository, number, reviewID), payload, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) DeletePendingReview(repository string, number int, reviewID int64) error {
	if err := c.write(http.MethodDelete, fmt.Sprintf("repos/%s/pulls/%d/reviews/%d", repository, number, reviewID), nil, nil); err != nil {
		return classifyAPIError(err)
	}
	return nil
}

func (c *Client) UpdateReview(repository string, number int, reviewID int64, body string) (*Review, error) {
	var result Review
	if err := c.write(http.MethodPut, fmt.Sprintf("repos/%s/pulls/%d/reviews/%d", repository, number, reviewID), map[string]string{"body": body}, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) DismissReview(repository string, number int, reviewID int64, message string) (*Review, error) {
	var result Review
	if err := c.write(http.MethodPut, fmt.Sprintf("repos/%s/pulls/%d/reviews/%d/dismissals", repository, number, reviewID), map[string]string{"message": message}, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) AddReviewComment(repository string, number int, sha string, comment model.Comment) (*ReviewComment, error) {
	payload := map[string]interface{}{
		"commit_id": sha, "path": comment.Path, "body": comment.GitHubBody(),
	}
	if comment.Subject == "file" {
		payload["subject_type"] = "file"
	} else {
		payload["subject_type"] = "line"
		payload["line"] = comment.Line
		payload["side"] = strings.ToUpper(comment.Side)
		if comment.StartLine != 0 {
			payload["start_line"] = comment.StartLine
			payload["start_side"] = strings.ToUpper(comment.StartSide)
		}
	}
	var result ReviewComment
	if err := c.write(http.MethodPost, fmt.Sprintf("repos/%s/pulls/%d/comments", repository, number), payload, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) ReplyToComment(repository string, number int, commentID int64, body string) (*ReviewComment, error) {
	var result ReviewComment
	payload := map[string]interface{}{"body": body, "in_reply_to": commentID}
	if err := c.write(http.MethodPost, fmt.Sprintf("repos/%s/pulls/%d/comments", repository, number), payload, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) EditReviewComment(repository string, commentID int64, body string) (*ReviewComment, error) {
	var result ReviewComment
	if err := c.write(http.MethodPatch, fmt.Sprintf("repos/%s/pulls/comments/%d", repository, commentID), map[string]string{"body": body}, &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) DeleteReviewComment(repository string, commentID int64) error {
	if err := c.write(http.MethodDelete, fmt.Sprintf("repos/%s/pulls/comments/%d", repository, commentID), nil, nil); err != nil {
		return classifyAPIError(err)
	}
	return nil
}

func (c *Client) GetReviewComment(repository string, commentID int64) (*ReviewComment, error) {
	var result ReviewComment
	if err := c.get(fmt.Sprintf("repos/%s/pulls/comments/%d", repository, commentID), &result); err != nil {
		return nil, classifyAPIError(err)
	}
	return &result, nil
}

func (c *Client) get(path string, out interface{}) error {
	return c.request(http.MethodGet, path, nil, out)
}

func (c *Client) write(method, path string, payload, out interface{}) error {
	var body io.Reader
	if payload != nil {
		data, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(data)
	}
	return c.request(method, path, body, out)
}

func (c *Client) request(method, path string, body io.Reader, out interface{}) error {
	var bodyBytes []byte
	var err error
	if body != nil {
		bodyBytes, err = io.ReadAll(body)
		if err != nil {
			return err
		}
	}
	for attempt := 0; attempt < 3; attempt++ {
		var requestBody io.Reader
		if bodyBytes != nil {
			requestBody = bytes.NewReader(bodyBytes)
		}
		response, requestErr := c.REST.Request(method, path, requestBody)
		if requestErr != nil {
			if shouldRetryError(requestErr) && attempt < 2 {
				time.Sleep(retryDelayError(requestErr, attempt))
				continue
			}
			return requestErr
		}
		if shouldRetry(response) && attempt < 2 {
			delay := retryDelay(response, attempt)
			_, _ = io.Copy(io.Discard, response.Body)
			_ = response.Body.Close()
			time.Sleep(delay)
			continue
		}
		defer func() {
			_ = response.Body.Close()
		}()
		if response.StatusCode >= http.StatusBadRequest {
			return api.HandleHTTPError(response)
		}
		if out == nil || response.StatusCode == http.StatusNoContent {
			_, _ = io.Copy(io.Discard, response.Body)
			return nil
		}
		if err := json.NewDecoder(response.Body).Decode(out); err != nil {
			return fmt.Errorf("decode GitHub response: %w", err)
		}
		return nil
	}
	return fmt.Errorf("GitHub API retry limit exceeded")
}

func (c *Client) paginate(path string, target interface{}) error {
	next := path
	for next != "" {
		var response *http.Response
		for attempt := 0; attempt < 3; attempt++ {
			var err error
			response, err = c.REST.Request(http.MethodGet, next, nil)
			if err != nil {
				if shouldRetryError(err) && attempt < 2 {
					time.Sleep(retryDelayError(err, attempt))
					continue
				}
				return err
			}
			if shouldRetry(response) && attempt < 2 {
				delay := retryDelay(response, attempt)
				_, _ = io.Copy(io.Discard, response.Body)
				_ = response.Body.Close()
				time.Sleep(delay)
				continue
			}
			break
		}
		if response.StatusCode >= http.StatusBadRequest {
			err := api.HandleHTTPError(response)
			_ = response.Body.Close()
			return err
		}
		if err := appendPage(response.Body, target); err != nil {
			_ = response.Body.Close()
			return err
		}
		next = nextLink(response.Header.Get("Link"))
		if err := response.Body.Close(); err != nil {
			return fmt.Errorf("close GitHub response: %w", err)
		}
	}
	return nil
}

func shouldRetry(response *http.Response) bool {
	if response.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return response.StatusCode == http.StatusForbidden &&
		(response.Header.Get("Retry-After") != "" || response.Header.Get("X-RateLimit-Remaining") == "0")
}

func shouldRetryError(err error) bool {
	var httpErr *api.HTTPError
	if !errors.As(err, &httpErr) {
		return false
	}
	if httpErr.StatusCode == http.StatusTooManyRequests {
		return true
	}
	return httpErr.StatusCode == http.StatusForbidden &&
		(httpErr.Headers.Get("Retry-After") != "" || httpErr.Headers.Get("X-RateLimit-Remaining") == "0")
}

func retryDelay(response *http.Response, attempt int) time.Duration {
	return retryDelayHeaders(response.Header, attempt)
}

func retryDelayError(err error, attempt int) time.Duration {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		return retryDelayHeaders(httpErr.Headers, attempt)
	}
	return time.Duration(attempt+1) * time.Second
}

func retryDelayHeaders(headers http.Header, attempt int) time.Duration {
	if raw := headers.Get("Retry-After"); raw != "" {
		if seconds, err := strconv.Atoi(raw); err == nil {
			delay := time.Duration(seconds) * time.Second
			if delay > 5*time.Second {
				return 5 * time.Second
			}
			return delay
		}
	}
	return time.Duration(attempt+1) * time.Second
}

func appendPage(reader io.Reader, target interface{}) error {
	data, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	switch typed := target.(type) {
	case *[]diff.File:
		var page []diff.File
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		*typed = append(*typed, page...)
	case *[]Review:
		var page []Review
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		*typed = append(*typed, page...)
	case *[]ReviewComment:
		var page []ReviewComment
		if err := json.Unmarshal(data, &page); err != nil {
			return err
		}
		*typed = append(*typed, page...)
	default:
		// Decode any slice without reflection by wrapping pages as raw JSON and
		// re-decoding their concatenation.
		var incoming []json.RawMessage
		if err := json.Unmarshal(data, &incoming); err != nil {
			return err
		}
		existing, err := json.Marshal(target)
		if err != nil {
			return err
		}
		var current []json.RawMessage
		if err := json.Unmarshal(existing, &current); err != nil {
			return err
		}
		combined, _ := json.Marshal(append(current, incoming...))
		if err := json.Unmarshal(combined, target); err != nil {
			return err
		}
	}
	return nil
}

func nextLink(header string) string {
	for _, part := range strings.Split(header, ",") {
		sections := strings.Split(part, ";")
		if len(sections) < 2 || !strings.Contains(sections[1], `rel="next"`) {
			continue
		}
		raw := strings.Trim(strings.TrimSpace(sections[0]), "<>")
		parsed, err := url.Parse(raw)
		if err != nil {
			return ""
		}
		next := strings.TrimPrefix(parsed.Path, "/")
		if parsed.RawQuery != "" {
			next += "?" + parsed.RawQuery
		}
		return next
	}
	return ""
}

func classifyAPIError(err error) error {
	var httpErr *api.HTTPError
	if errors.As(err, &httpErr) {
		status := httpErr.StatusCode
		message := httpErr.Message
		if message == "" {
			message = err.Error()
		}
		switch status {
		case http.StatusUnauthorized, http.StatusForbidden:
			return output.NewError(output.ExitAuth, "AUTHORIZATION", message, false, map[string]int{"status": status})
		case http.StatusConflict, http.StatusUnprocessableEntity:
			return output.NewError(output.ExitConflict, "GITHUB_CONFLICT", message, false, map[string]int{"status": status})
		case http.StatusNotFound:
			return output.NewError(output.ExitAPI, "NOT_FOUND", message, false, map[string]int{"status": status})
		case http.StatusTooManyRequests:
			return output.NewError(output.ExitAPI, "RATE_LIMITED", message, true, map[string]int{"status": status})
		default:
			return output.NewError(output.ExitAPI, "GITHUB_API", message, status >= 500, map[string]int{"status": status})
		}
	}
	return output.NewError(output.ExitAPI, "GITHUB_API", err.Error(), true, nil)
}

func ParseInt(value string, field string) (int64, error) {
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil || number <= 0 {
		return 0, fmt.Errorf("%s must be a positive integer", field)
	}
	return number, nil
}
