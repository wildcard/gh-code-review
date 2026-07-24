package review

import (
	"fmt"
	"strings"

	"github.com/wildcard/gh-code-review/internal/diff"
	gh "github.com/wildcard/gh-code-review/internal/github"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

type ValidationResult struct {
	Valid             bool                           `json:"valid"`
	DerivedEvent      model.Event                    `json:"derived_event,omitempty"`
	CurrentHeadSHA    string                         `json:"current_head_sha,omitempty"`
	ExpectedHeadSHA   string                         `json:"expected_head_sha"`
	Issues            []model.Issue                  `json:"issues"`
	Warnings          []model.Issue                  `json:"warnings"`
	CommentableRanges map[string]map[string][][2]int `json:"commentable_ranges,omitempty"`
	PendingReviewID   int64                          `json:"pending_review_id,omitempty"`
	PullRequest       *gh.PullRequest                `json:"pull_request,omitempty"`
	Files             []diff.File                    `json:"files,omitempty"`
	ExistingThreads   int                            `json:"existing_threads"`
}

type ValidateOptions struct {
	DeriveEvent  bool
	ResumeReview int64
	IncludePatch bool
}

func Validate(client *gh.Client, manifest *model.Manifest, options ValidateOptions) (*ValidationResult, error) {
	result := &ValidationResult{
		ExpectedHeadSHA: manifest.ExpectedHeadSHA,
		Issues:          []model.Issue{},
		Warnings:        []model.Issue{},
	}
	result.Issues = append(result.Issues, manifest.Validate(options.DeriveEvent)...)
	if options.DeriveEvent {
		result.DerivedEvent = manifest.Event
	}
	if len(result.Issues) > 0 {
		result.Valid = false
		return result, nil
	}

	pull, err := client.GetPull(manifest.Repository, manifest.PullRequest)
	if err != nil {
		return nil, err
	}
	result.PullRequest = pull
	result.CurrentHeadSHA = pull.Head.SHA
	if !strings.EqualFold(pull.Head.SHA, manifest.ExpectedHeadSHA) {
		return result, output.NewError(output.ExitHeadChanged, "HEAD_CHANGED",
			fmt.Sprintf("pull request head changed from %s to %s", manifest.ExpectedHeadSHA, pull.Head.SHA),
			true, map[string]string{"expected_head_sha": manifest.ExpectedHeadSHA, "actual_head_sha": pull.Head.SHA})
	}
	if manifest.Event == model.EventApprove && strings.EqualFold(pull.User.Login, currentLogin(client)) {
		result.Issues = append(result.Issues, model.Issue{Code: "SELF_APPROVAL", Message: "GitHub does not allow authors to approve their own pull requests"})
	}

	files, err := client.ListFiles(manifest.Repository, manifest.PullRequest, true)
	if err != nil {
		return nil, err
	}
	index := diff.Build(files)
	for _, comment := range manifest.Comments {
		result.Issues = append(result.Issues, index.ValidateComment(comment)...)
	}
	result.CommentableRanges = index.CompactLocations()
	result.Files = stripPatches(files, options.IncludePatch)

	reviews, err := client.ListReviews(manifest.Repository, manifest.PullRequest)
	if err != nil {
		return nil, err
	}
	user, err := client.CurrentUser()
	if err != nil {
		return nil, err
	}
	for _, existing := range reviews {
		if existing.State == "PENDING" && strings.EqualFold(existing.User.Login, user.Login) {
			result.PendingReviewID = existing.ID
			if options.ResumeReview == 0 || options.ResumeReview != existing.ID {
				result.Issues = append(result.Issues, model.Issue{
					Code:    "PENDING_REVIEW_CONFLICT",
					Message: fmt.Sprintf("pending review %d already exists; pass --resume-review %d to use it or abandon it explicitly", existing.ID, existing.ID),
				})
			}
		}
	}

	comments, err := client.ListReviewComments(manifest.Repository, manifest.PullRequest)
	if err != nil {
		return nil, err
	}
	for _, proposed := range manifest.Comments {
		for _, existing := range comments {
			if sameFinding(proposed, existing) {
				result.Issues = append(result.Issues, model.Issue{
					Code:     "DUPLICATE_EXISTING",
					Message:  fmt.Sprintf("an equivalent review comment already exists at %s", existing.HTMLURL),
					ClientID: proposed.ClientID,
					Path:     proposed.Path,
				})
				break
			}
		}
	}

	threads, _, err := client.ListThreads(manifest.Repository, manifest.PullRequest)
	if err != nil {
		return nil, err
	}
	result.ExistingThreads = len(threads)
	result.Valid = len(result.Issues) == 0
	return result, nil
}

func ValidationError(result *ValidationResult) error {
	if result.Valid {
		return nil
	}
	exit := output.ExitValidation
	code := "VALIDATION_FAILED"
	for _, issue := range result.Issues {
		switch issue.Code {
		case "PENDING_REVIEW_CONFLICT", "DUPLICATE_EXISTING", "DUPLICATE_FINDING", "DUPLICATE_CLIENT_ID":
			exit = output.ExitConflict
			code = "CONFLICT"
		}
	}
	return output.NewError(exit, code, "review manifest failed validation", false, result.Issues)
}

func stripPatches(files []diff.File, include bool) []diff.File {
	if include {
		return files
	}
	result := append([]diff.File(nil), files...)
	for i := range result {
		result[i].Patch = ""
	}
	return result
}

func currentLogin(client *gh.Client) string {
	user, err := client.CurrentUser()
	if err != nil {
		return ""
	}
	return user.Login
}

func sameFinding(proposed model.Comment, existing gh.ReviewComment) bool {
	if proposed.Path != existing.Path {
		return false
	}
	if proposed.Subject == "file" {
		if !strings.EqualFold(existing.Subject, "file") {
			return false
		}
		return normalize(proposed.GitHubBody()) == normalize(existing.Body)
	}
	if existing.Subject != "" && !strings.EqualFold(existing.Subject, "line") {
		return false
	}
	if proposed.Line != existing.Line ||
		!strings.EqualFold(proposed.Side, existing.Side) ||
		proposed.StartLine != existing.StartLine ||
		!strings.EqualFold(proposed.StartSide, existing.StartSide) {
		return false
	}
	return normalize(proposed.GitHubBody()) == normalize(existing.Body)
}

func normalize(value string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
}
