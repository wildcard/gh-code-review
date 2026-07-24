package review

import (
	"fmt"
	"strings"

	gh "github.com/wildcard/gh-code-review/internal/github"
	"github.com/wildcard/gh-code-review/internal/journal"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

type SubmitOptions struct {
	DeriveEvent     bool
	ConfirmDecision bool
	ResumeReview    int64
	DryRun          bool
	Journal         *journal.Store
}

type Receipt struct {
	DryRun           bool        `json:"dry_run"`
	Event            model.Event `json:"event"`
	HeadSHA          string      `json:"head_sha"`
	ReviewID         int64       `json:"review_id,omitempty"`
	ReviewNodeID     string      `json:"review_node_id,omitempty"`
	ReviewURL        string      `json:"review_url,omitempty"`
	CommentsCreated  int         `json:"comments_created"`
	Transport        string      `json:"transport"`
	IdempotentReplay bool        `json:"idempotent_replay"`
	CleanupAttempted bool        `json:"cleanup_attempted"`
	CleanupSucceeded bool        `json:"cleanup_succeeded"`
}

func Submit(client *gh.Client, manifest *model.Manifest, options SubmitOptions) (*Receipt, error) {
	var err error
	if manifest.Event == "" && options.DeriveEvent {
		manifest.Event = model.DeriveEvent(manifest.Comments)
	}
	if manifest.Event != model.EventComment && !options.ConfirmDecision {
		return nil, output.NewError(output.ExitValidation, "DECISION_CONFIRMATION_REQUIRED",
			fmt.Sprintf("%s requires --confirm-decision", manifest.Event), false,
			map[string]string{"event": string(manifest.Event)})
	}

	store := options.Journal
	if store == nil {
		store, err = journal.DefaultStore()
		if err != nil {
			return nil, output.NewError(output.ExitInternal, "JOURNAL", err.Error(), false, nil)
		}
	}
	var existingEntry *journal.Entry
	if manifest.IdempotencyKey != "" {
		existingEntry, err = store.Load(manifest.IdempotencyKey)
		if err != nil {
			return nil, output.NewError(output.ExitInternal, "JOURNAL", err.Error(), false, nil)
		}
		if existingEntry != nil {
			if existingEntry.Repository != manifest.Repository || existingEntry.PullRequest != manifest.PullRequest || !strings.EqualFold(existingEntry.HeadSHA, manifest.ExpectedHeadSHA) {
				return nil, output.NewError(output.ExitConflict, "IDEMPOTENCY_CONFLICT",
					"idempotency key was already used for a different review transaction", false, existingEntry)
			}
			if existingEntry.State == "submitted" {
				review, reviewErr := client.GetReview(manifest.Repository, manifest.PullRequest, existingEntry.ReviewID)
				if reviewErr != nil {
					return nil, output.NewError(output.ExitConflict, "IDEMPOTENCY_RECONCILIATION",
						"the local journal says this review was submitted, but GitHub no longer confirms it", false,
						map[string]interface{}{"journal": existingEntry, "github_error": reviewErr.Error()})
				}
				comments, commentsErr := client.ListReviewComments(manifest.Repository, manifest.PullRequest)
				if commentsErr != nil {
					return nil, commentsErr
				}
				for _, proposed := range manifest.Comments {
					found := false
					for _, comment := range comments {
						if comment.PullRequestReviewID == existingEntry.ReviewID && sameFinding(proposed, comment) {
							found = true
							break
						}
					}
					if !found {
						return nil, output.NewError(output.ExitConflict, "IDEMPOTENCY_RECONCILIATION",
							"the submitted review no longer contains every normalized finding", false,
							map[string]interface{}{"review_id": existingEntry.ReviewID, "client_id": proposed.ClientID})
					}
				}
				return &Receipt{
					Event: manifest.Event, HeadSHA: existingEntry.HeadSHA, ReviewID: review.ID,
					ReviewNodeID: existingEntry.ReviewNodeID, ReviewURL: existingEntry.ReviewURL,
					CommentsCreated: len(manifest.Comments), Transport: "journal",
					IdempotentReplay: true,
				}, nil
			}
			if options.ResumeReview == 0 && existingEntry.ReviewID != 0 {
				options.ResumeReview = existingEntry.ReviewID
			}
		}
	}

	validation, err := Validate(client, manifest, ValidateOptions{
		DeriveEvent: options.DeriveEvent, ResumeReview: options.ResumeReview,
	})
	if err != nil {
		return nil, err
	}
	if err := ValidationError(validation); err != nil {
		return nil, err
	}

	lineOnly := options.ResumeReview == 0
	for _, comment := range manifest.Comments {
		if comment.Subject == "file" {
			lineOnly = false
			break
		}
	}
	transport := "rest-batch"
	if !lineOnly {
		transport = "graphql-pending"
	}
	receipt := &Receipt{
		DryRun: options.DryRun, Event: manifest.Event, HeadSHA: manifest.ExpectedHeadSHA,
		CommentsCreated: len(manifest.Comments), Transport: transport,
	}
	if options.DryRun {
		return receipt, nil
	}

	entry := &journal.Entry{
		IdempotencyKey: manifest.IdempotencyKey, Repository: manifest.Repository,
		PullRequest: manifest.PullRequest, HeadSHA: manifest.ExpectedHeadSHA, State: "starting",
	}
	_ = store.Save(entry)

	var pending *gh.Review
	if options.ResumeReview != 0 {
		reviews, err := client.ListReviews(manifest.Repository, manifest.PullRequest)
		if err != nil {
			return nil, err
		}
		for i := range reviews {
			if reviews[i].ID == options.ResumeReview && reviews[i].State == "PENDING" {
				pending = &reviews[i]
				break
			}
		}
		if pending == nil {
			return nil, output.NewError(output.ExitConflict, "PENDING_REVIEW_NOT_FOUND",
				fmt.Sprintf("pending review %d was not found", options.ResumeReview), false, nil)
		}
	} else if lineOnly {
		apiComments := make([]model.APIComment, 0, len(manifest.Comments))
		for _, comment := range manifest.Comments {
			apiComments = append(apiComments, comment.APIComment())
		}
		pending, err = client.CreatePendingReview(manifest.Repository, manifest.PullRequest, manifest.ExpectedHeadSHA, apiComments)
		if err != nil {
			return nil, err
		}
	} else {
		pending, err = client.CreatePendingReview(manifest.Repository, manifest.PullRequest, manifest.ExpectedHeadSHA, nil)
		if err != nil {
			return nil, err
		}
	}

	receipt.ReviewID = pending.ID
	receipt.ReviewNodeID = pending.NodeID
	entry.ReviewID = pending.ID
	entry.ReviewNodeID = pending.NodeID
	entry.State = "pending"
	_ = store.Save(entry)

	cleanup := func(cause error) (*Receipt, error) {
		receipt.CleanupAttempted = true
		if cleanupErr := client.DeletePendingReview(manifest.Repository, manifest.PullRequest, pending.ID); cleanupErr != nil {
			entry.State = "cleanup_failed"
			entry.Error = cause.Error() + "; cleanup: " + cleanupErr.Error()
			_ = store.Save(entry)
			return receipt, output.NewError(output.ExitPartial, "PARTIAL_TRANSACTION",
				"review submission failed and the pending review could not be deleted", true,
				map[string]interface{}{"review_id": pending.ID, "cause": cause.Error(), "cleanup_error": cleanupErr.Error()})
		}
		receipt.CleanupSucceeded = true
		entry.State = "cleaned"
		entry.Error = cause.Error()
		_ = store.Save(entry)
		return receipt, cause
	}

	if !lineOnly {
		for _, comment := range manifest.Comments {
			thread, addErr := client.AddReviewThread(pending.NodeID, comment)
			if addErr != nil {
				return cleanup(addErr)
			}
			entry.CommentIDs = append(entry.CommentIDs, thread.ID)
			entry.State = "comments_added"
			_ = store.Save(entry)
		}
	} else {
		for _, comment := range manifest.Comments {
			entry.CommentIDs = append(entry.CommentIDs, comment.ClientID)
		}
		entry.State = "comments_added"
		_ = store.Save(entry)
	}

	current, err := client.GetPull(manifest.Repository, manifest.PullRequest)
	if err != nil {
		return cleanup(err)
	}
	if !strings.EqualFold(current.Head.SHA, manifest.ExpectedHeadSHA) {
		return cleanup(output.NewError(output.ExitHeadChanged, "HEAD_CHANGED",
			fmt.Sprintf("pull request head changed from %s to %s before submission", manifest.ExpectedHeadSHA, current.Head.SHA),
			true, map[string]string{"expected_head_sha": manifest.ExpectedHeadSHA, "actual_head_sha": current.Head.SHA}))
	}

	submitted, err := client.SubmitReview(manifest.Repository, manifest.PullRequest, pending.ID, manifest.Event, manifest.Summary)
	if err != nil {
		return cleanup(err)
	}
	receipt.ReviewURL = submitted.HTMLURL
	entry.State = "submitted"
	entry.ReviewURL = submitted.HTMLURL
	entry.Error = ""
	_ = store.Save(entry)
	return receipt, nil
}
