package cmd

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	gh "github.com/wildcard/gh-code-review/internal/github"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
	reviewsvc "github.com/wildcard/gh-code-review/internal/review"
)

func (a *app) pendingCommand() *cobra.Command {
	root := &cobra.Command{Use: "pending", Short: "Manage pending reviews"}
	root.AddCommand(a.pendingStartCommand(), a.pendingShowCommand(), a.pendingAbandonCommand())
	return root
}

func (a *app) pendingStartCommand() *cobra.Command {
	var target target
	command := &cobra.Command{
		Use: "start <pull-request>", Short: "Start an empty pending review", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("pending.start", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish("pending.start", target.repository, target.pr, nil, err)
				return
			}
			pull, err := client.GetPull(target.repository, target.pr)
			if err != nil {
				a.finish("pending.start", target.repository, target.pr, nil, err)
				return
			}
			reviews, err := client.ListReviews(target.repository, target.pr)
			if err != nil {
				a.finish("pending.start", target.repository, target.pr, nil, err)
				return
			}
			user, err := client.CurrentUser()
			if err != nil {
				a.finish("pending.start", target.repository, target.pr, nil, err)
				return
			}
			for _, review := range reviews {
				if review.State == "PENDING" && strings.EqualFold(review.User.Login, user.Login) {
					a.finish("pending.start", target.repository, target.pr, nil,
						output.NewError(output.ExitConflict, "PENDING_REVIEW_CONFLICT",
							fmt.Sprintf("pending review %d already exists", review.ID), false, review))
					return
				}
			}
			result, err := client.CreatePendingReview(target.repository, target.pr, pull.Head.SHA, nil)
			a.finish("pending.start", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	return command
}

func (a *app) pendingShowCommand() *cobra.Command {
	var target target
	command := &cobra.Command{
		Use: "show <pull-request>", Short: "Show the current user's pending review", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("pending.show", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish("pending.show", target.repository, target.pr, nil, err)
				return
			}
			reviews, err := client.ListReviews(target.repository, target.pr)
			if err != nil {
				a.finish("pending.show", target.repository, target.pr, nil, err)
				return
			}
			user, err := client.CurrentUser()
			if err != nil {
				a.finish("pending.show", target.repository, target.pr, nil, err)
				return
			}
			for _, review := range reviews {
				if review.State == "PENDING" && strings.EqualFold(review.User.Login, user.Login) {
					a.finish("pending.show", target.repository, target.pr, review, nil)
					return
				}
			}
			a.finish("pending.show", target.repository, target.pr, nil,
				output.NewError(output.ExitConflict, "PENDING_REVIEW_NOT_FOUND", "no pending review exists for the current user", false, nil))
		},
	}
	bindTarget(command, &target)
	return command
}

func (a *app) pendingAbandonCommand() *cobra.Command {
	var target target
	var reviewID int64
	var confirm bool
	command := &cobra.Command{
		Use: "abandon <pull-request>", Short: "Delete a pending review", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("pending.abandon", target.repository, target.pr, nil, err)
				return
			}
			if err := positiveID(reviewID, "review-id"); err != nil {
				a.finish("pending.abandon", target.repository, target.pr, nil, err)
				return
			}
			if !confirm {
				a.finish("pending.abandon", target.repository, target.pr, nil,
					output.NewError(output.ExitValidation, "CONFIRMATION_REQUIRED", "abandoning a pending review requires --confirm", false, nil))
				return
			}
			client, err := a.client()
			if err == nil {
				err = client.DeletePendingReview(target.repository, target.pr, reviewID)
			}
			a.finish("pending.abandon", target.repository, target.pr, map[string]interface{}{"review_id": reviewID, "abandoned": err == nil}, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Pending review database ID")
	command.Flags().BoolVar(&confirm, "confirm", false, "Confirm deletion of the pending review")
	return command
}

func (a *app) commentCommand() *cobra.Command {
	root := &cobra.Command{Use: "comment", Short: "Add, edit, or delete review comments"}
	root.AddCommand(a.commentAddCommand(false), a.commentAddCommand(true), a.commentEditCommand(), a.commentDeleteCommand())
	return root
}

func (a *app) commentAddCommand(suggestion bool) *cobra.Command {
	var target target
	var reviewID int64
	var subject, path, body, bodyFile, side, startSide, replacementFile string
	var line, startLine int
	use, short := "add <pull-request>", "Add a comment to a pending review"
	operation := "comment.add"
	if suggestion {
		use, short, operation = "suggest <pull-request>", "Add an apply-able code suggestion to a pending review", "comment.suggest"
	}
	command := &cobra.Command{
		Use: use, Short: short, Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			if err := positiveID(reviewID, "review-id"); err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			text, err := resolveBody(body, bodyFile)
			if err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			comment := model.Comment{
				ClientID: "command", Subject: strings.ToLower(subject), Path: path, Body: text,
				Line: line, Side: side, StartLine: startLine, StartSide: startSide,
			}
			if comment.Subject == "file" {
				comment.Line, comment.StartLine = 0, 0
				comment.Side, comment.StartSide = "", ""
			} else if comment.StartLine != 0 && comment.StartSide == "" {
				comment.StartSide = comment.Side
			}
			if suggestion {
				data, readErr := os.ReadFile(replacementFile)
				if readErr != nil {
					a.finish(operation, target.repository, target.pr, nil,
						output.NewError(output.ExitValidation, "REPLACEMENT", readErr.Error(), false, nil))
					return
				}
				replacement := string(data)
				comment.Replacement = &replacement
			}
			manifest := &model.Manifest{
				SchemaVersion: model.SchemaVersion, Repository: target.repository, PullRequest: target.pr,
				ExpectedHeadSHA: "command", Event: model.EventComment, Comments: []model.Comment{comment},
			}
			issues := manifest.Validate(false)
			// The command validates the live head separately; ignore the placeholder SHA.
			filtered := issues[:0]
			for _, issue := range issues {
				if issue.Code != "EXPECTED_HEAD_SHA" {
					filtered = append(filtered, issue)
				}
			}
			if len(filtered) > 0 {
				a.finish(operation, target.repository, target.pr, nil,
					output.NewError(output.ExitValidation, "VALIDATION_FAILED", "comment failed validation", false, filtered))
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			review, err := client.GetReview(target.repository, target.pr, reviewID)
			if err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			if review.State != "PENDING" {
				a.finish(operation, target.repository, target.pr, nil,
					output.NewError(output.ExitConflict, "REVIEW_NOT_PENDING", "comments can be added only to a pending review", false, review))
				return
			}
			pull, err := client.GetPull(target.repository, target.pr)
			if err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			if review.CommitID != "" && !strings.EqualFold(review.CommitID, pull.Head.SHA) {
				a.finish(operation, target.repository, target.pr, nil,
					output.NewError(output.ExitHeadChanged, "HEAD_CHANGED",
						fmt.Sprintf("pending review targets %s but the pull request head is %s", review.CommitID, pull.Head.SHA),
						true, map[string]string{"review_head_sha": review.CommitID, "actual_head_sha": pull.Head.SHA}))
				return
			}
			manifest.ExpectedHeadSHA = pull.Head.SHA
			validation, err := reviewsvc.Validate(client, manifest, reviewsvc.ValidateOptions{ResumeReview: reviewID})
			if err == nil {
				err = reviewsvc.ValidationError(validation)
			}
			if err != nil {
				a.finish(operation, target.repository, target.pr, validation, err)
				return
			}
			thread, err := client.AddReviewThread(review.NodeID, comment)
			a.finish(operation, target.repository, target.pr, thread, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Pending review database ID")
	command.Flags().StringVar(&subject, "subject", "line", "Comment subject: line or file")
	command.Flags().StringVar(&path, "path", "", "Repository-relative changed file path")
	command.Flags().StringVar(&body, "body", "", "Comment body")
	command.Flags().StringVar(&bodyFile, "body-file", "", "Read comment body from a file")
	command.Flags().IntVar(&line, "line", 0, "End line in the diff")
	command.Flags().StringVar(&side, "side", "RIGHT", "Diff side: RIGHT or LEFT")
	command.Flags().IntVar(&startLine, "start-line", 0, "Start line for a multiline comment")
	command.Flags().StringVar(&startSide, "start-side", "", "Start diff side for a multiline comment")
	if suggestion {
		command.Flags().StringVar(&replacementFile, "replacement-file", "", "File containing replacement code")
		_ = command.MarkFlagRequired("replacement-file")
	}
	_ = command.MarkFlagRequired("path")
	return command
}

func (a *app) commentEditCommand() *cobra.Command {
	var repo, body, bodyFile, commentNodeID string
	var commentID int64
	command := &cobra.Command{
		Use: "edit", Short: "Edit an existing review comment", Args: cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			if repo == "" {
				a.finish("comment.edit", repo, 0, nil, output.NewError(output.ExitValidation, "REPOSITORY", "--repo is required", false, nil))
				return
			}
			if err := validateCommentReference(commentID, commentNodeID); err != nil {
				a.finish("comment.edit", repo, 0, nil, err)
				return
			}
			text, err := resolveBody(body, bodyFile)
			if err != nil {
				a.finish("comment.edit", repo, 0, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				if commentNodeID != "" {
					result, err = client.EditReviewCommentNode(commentNodeID, text)
				} else {
					result, err = client.EditReviewComment(repo, commentID, text)
				}
			}
			a.finish("comment.edit", repo, 0, result, err)
		},
	}
	command.Flags().StringVarP(&repo, "repo", "R", "", "Repository in owner/name form")
	command.Flags().Int64Var(&commentID, "comment-id", 0, "Submitted review comment database ID")
	command.Flags().StringVar(&commentNodeID, "comment-node-id", "", "Review comment node ID, including pending comments")
	command.Flags().StringVar(&body, "body", "", "New comment body")
	command.Flags().StringVar(&bodyFile, "body-file", "", "Read new body from a file")
	return command
}

func (a *app) commentDeleteCommand() *cobra.Command {
	var repo, commentNodeID string
	var commentID int64
	var confirm bool
	command := &cobra.Command{
		Use: "delete", Short: "Delete an existing review comment", Args: cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			if repo == "" {
				a.finish("comment.delete", repo, 0, nil, output.NewError(output.ExitValidation, "REPOSITORY", "--repo is required", false, nil))
				return
			}
			if !confirm {
				a.finish("comment.delete", repo, 0, nil, output.NewError(output.ExitValidation, "CONFIRMATION_REQUIRED", "--confirm is required", false, nil))
				return
			}
			if err := validateCommentReference(commentID, commentNodeID); err != nil {
				a.finish("comment.delete", repo, 0, nil, err)
				return
			}
			client, err := a.client()
			if err == nil {
				if commentNodeID != "" {
					err = client.DeleteReviewCommentNode(commentNodeID)
				} else {
					err = client.DeleteReviewComment(repo, commentID)
				}
			}
			result := map[string]interface{}{"deleted": err == nil}
			if commentNodeID != "" {
				result["comment_node_id"] = commentNodeID
			} else {
				result["comment_id"] = commentID
			}
			a.finish("comment.delete", repo, 0, result, err)
		},
	}
	command.Flags().StringVarP(&repo, "repo", "R", "", "Repository in owner/name form")
	command.Flags().Int64Var(&commentID, "comment-id", 0, "Submitted review comment database ID")
	command.Flags().StringVar(&commentNodeID, "comment-node-id", "", "Review comment node ID, including pending comments")
	command.Flags().BoolVar(&confirm, "confirm", false, "Confirm deletion")
	return command
}

func validateCommentReference(commentID int64, commentNodeID string) error {
	if commentID > 0 && commentNodeID != "" {
		return output.NewError(output.ExitValidation, "COMMENT_REFERENCE",
			"use only one of --comment-id or --comment-node-id", false, nil)
	}
	if commentID > 0 || commentNodeID != "" {
		return nil
	}
	return output.NewError(output.ExitValidation, "COMMENT_REFERENCE",
		"one of --comment-id or --comment-node-id is required", false, nil)
}

func (a *app) threadCommand() *cobra.Command {
	root := &cobra.Command{Use: "thread", Short: "Read and manage review threads"}
	root.AddCommand(
		a.threadListCommand(),
		a.threadShowCommand(),
		a.threadReplyCommand(),
		a.threadResolutionCommand(true),
		a.threadResolutionCommand(false),
	)
	return root
}

func (a *app) threadListCommand() *cobra.Command {
	var target target
	var unresolved bool
	command := &cobra.Command{
		Use: "list <pull-request>", Short: "List review threads", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("thread.list", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var threads []gh.Thread
			if err == nil {
				threads, _, err = client.ListThreads(target.repository, target.pr)
			}
			if unresolved && err == nil {
				filtered := threads[:0]
				for _, thread := range threads {
					if !thread.IsResolved {
						filtered = append(filtered, thread)
					}
				}
				threads = filtered
			}
			a.finish("thread.list", target.repository, target.pr, threads, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().BoolVar(&unresolved, "unresolved", false, "Return only unresolved threads")
	return command
}

func (a *app) threadShowCommand() *cobra.Command {
	var target target
	var threadID string
	command := &cobra.Command{
		Use: "show <pull-request>", Short: "Show one review thread", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("thread.show", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var threads []gh.Thread
			if err == nil {
				threads, _, err = client.ListThreads(target.repository, target.pr)
			}
			if err != nil {
				a.finish("thread.show", target.repository, target.pr, nil, err)
				return
			}
			for _, thread := range threads {
				if thread.ID == threadID {
					a.finish("thread.show", target.repository, target.pr, thread, nil)
					return
				}
			}
			a.finish("thread.show", target.repository, target.pr, nil, output.NewError(output.ExitAPI, "NOT_FOUND", "review thread not found", false, nil))
		},
	}
	bindTarget(command, &target)
	command.Flags().StringVar(&threadID, "thread-id", "", "Review thread GraphQL node ID")
	_ = command.MarkFlagRequired("thread-id")
	return command
}

func (a *app) threadReplyCommand() *cobra.Command {
	var target target
	var threadID, body, bodyFile string
	command := &cobra.Command{
		Use: "reply <pull-request>", Short: "Reply to a review thread", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("thread.reply", target.repository, target.pr, nil, err)
				return
			}
			text, err := resolveBody(body, bodyFile)
			if err != nil {
				a.finish("thread.reply", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.ReplyToThread(threadID, text)
			}
			a.finish("thread.reply", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().StringVar(&threadID, "thread-id", "", "Review thread GraphQL node ID")
	command.Flags().StringVar(&body, "body", "", "Reply body")
	command.Flags().StringVar(&bodyFile, "body-file", "", "Read reply body from a file")
	_ = command.MarkFlagRequired("thread-id")
	return command
}

func (a *app) threadResolutionCommand(resolve bool) *cobra.Command {
	var threadID string
	var confirm bool
	name, operation := "resolve", "thread.resolve"
	if !resolve {
		name, operation = "unresolve", "thread.unresolve"
	}
	command := &cobra.Command{
		Use: name, Short: name + " a review thread", Args: cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			if !confirm {
				a.finish(operation, "", 0, nil, output.NewError(output.ExitValidation, "CONFIRMATION_REQUIRED", "--confirm is required", false, nil))
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.ResolveThread(threadID, resolve)
			}
			a.finish(operation, "", 0, result, err)
		},
	}
	command.Flags().StringVar(&threadID, "thread-id", "", "Review thread GraphQL node ID")
	command.Flags().BoolVar(&confirm, "confirm", false, "Confirm state change")
	_ = command.MarkFlagRequired("thread-id")
	return command
}

func (a *app) reviewCommand() *cobra.Command {
	root := &cobra.Command{Use: "review", Short: "Read and manage pull request reviews"}
	root.AddCommand(
		a.reviewListCommand(), a.reviewShowCommand(), a.reviewEditCommand(),
		a.reviewSubmitCommand(), a.reviewDismissCommand(),
	)
	return root
}

func (a *app) reviewListCommand() *cobra.Command {
	var target target
	command := &cobra.Command{
		Use: "list <pull-request>", Short: "List reviews", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("review.list", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.ListReviews(target.repository, target.pr)
			}
			a.finish("review.list", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	return command
}

func (a *app) reviewShowCommand() *cobra.Command {
	var target target
	var reviewID int64
	command := &cobra.Command{
		Use: "show <pull-request>", Short: "Show one review", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("review.show", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.GetReview(target.repository, target.pr, reviewID)
			}
			a.finish("review.show", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Review database ID")
	_ = command.MarkFlagRequired("review-id")
	return command
}

func (a *app) reviewEditCommand() *cobra.Command {
	var target target
	var reviewID int64
	var body, bodyFile string
	command := &cobra.Command{
		Use: "edit <pull-request>", Short: "Edit a review summary", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("review.edit", target.repository, target.pr, nil, err)
				return
			}
			text, err := resolveBody(body, bodyFile)
			if err != nil {
				a.finish("review.edit", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.UpdateReview(target.repository, target.pr, reviewID, text)
			}
			a.finish("review.edit", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Review database ID")
	command.Flags().StringVar(&body, "body", "", "New review summary")
	command.Flags().StringVar(&bodyFile, "body-file", "", "Read summary from a file")
	_ = command.MarkFlagRequired("review-id")
	return command
}

func (a *app) reviewSubmitCommand() *cobra.Command {
	var target target
	var reviewID int64
	var event, body, bodyFile string
	var confirmDecision bool
	command := &cobra.Command{
		Use: "submit <pull-request>", Short: "Submit a pending review", Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("review.submit", target.repository, target.pr, nil, err)
				return
			}
			reviewEvent := model.Event(strings.ToUpper(event))
			if reviewEvent != model.EventComment && reviewEvent != model.EventApprove && reviewEvent != model.EventRequestChanges {
				a.finish("review.submit", target.repository, target.pr, nil, output.NewError(output.ExitValidation, "EVENT", "event must be COMMENT, APPROVE, or REQUEST_CHANGES", false, nil))
				return
			}
			if reviewEvent != model.EventComment && !confirmDecision {
				a.finish("review.submit", target.repository, target.pr, nil, output.NewError(output.ExitValidation, "DECISION_CONFIRMATION_REQUIRED", "--confirm-decision is required", false, nil))
				return
			}
			text, err := resolveBodyOptional(body, bodyFile)
			if err != nil {
				a.finish("review.submit", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				var pending *gh.Review
				pending, err = client.GetReview(target.repository, target.pr, reviewID)
				if err == nil && pending.State != "PENDING" {
					err = output.NewError(output.ExitConflict, "REVIEW_NOT_PENDING", "only a pending review can be submitted", false, pending)
				}
				var pull *gh.PullRequest
				if err == nil {
					pull, err = client.GetPull(target.repository, target.pr)
				}
				if err == nil && pending.CommitID != "" && !strings.EqualFold(pending.CommitID, pull.Head.SHA) {
					err = output.NewError(output.ExitHeadChanged, "HEAD_CHANGED",
						fmt.Sprintf("pending review targets %s but the pull request head is %s", pending.CommitID, pull.Head.SHA),
						true, map[string]string{"review_head_sha": pending.CommitID, "actual_head_sha": pull.Head.SHA})
				}
				if err == nil && reviewEvent == model.EventApprove {
					var user *gh.User
					user, err = client.CurrentUser()
					if err == nil && strings.EqualFold(user.Login, pull.User.Login) {
						err = output.NewError(output.ExitValidation, "SELF_APPROVAL", "GitHub does not allow authors to approve their own pull requests", false, nil)
					}
				}
				if err == nil && reviewEvent != model.EventComment && strings.TrimSpace(text) == "" {
					err = output.NewError(output.ExitValidation, "SUMMARY", "a summary is required for APPROVE and REQUEST_CHANGES", false, nil)
				}
				if err == nil {
					result, err = client.SubmitReview(target.repository, target.pr, reviewID, reviewEvent, text)
				}
			}
			a.finish("review.submit", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Pending review database ID")
	command.Flags().StringVar(&event, "event", "COMMENT", "COMMENT, APPROVE, or REQUEST_CHANGES")
	command.Flags().StringVar(&body, "body", "", "Review summary")
	command.Flags().StringVar(&bodyFile, "body-file", "", "Read summary from a file")
	command.Flags().BoolVar(&confirmDecision, "confirm-decision", false, "Confirm APPROVE or REQUEST_CHANGES")
	_ = command.MarkFlagRequired("review-id")
	return command
}

func (a *app) reviewDismissCommand() *cobra.Command {
	var target target
	var reviewID int64
	var message, messageFile string
	var confirm bool
	command := &cobra.Command{
		Use: "dismiss <pull-request>", Short: "Dismiss a submitted review", Args: cobra.MaximumNArgs(1),
		Long: "Dismiss a submitted review. GitHub records the dismissal explanation on the pull request timeline.",
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("review.dismiss", target.repository, target.pr, nil, err)
				return
			}
			if !confirm {
				a.finish("review.dismiss", target.repository, target.pr, nil,
					output.NewError(output.ExitValidation, "TIMELINE_COMMENT_CONFIRMATION_REQUIRED",
						"GitHub will add the dismissal explanation to the pull request timeline; pass --confirm-timeline-comment", false, nil))
				return
			}
			text, err := resolveBody(message, messageFile)
			if err != nil {
				a.finish("review.dismiss", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			var result interface{}
			if err == nil {
				result, err = client.DismissReview(target.repository, target.pr, reviewID, text)
			}
			a.finish("review.dismiss", target.repository, target.pr, result, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().Int64Var(&reviewID, "review-id", 0, "Submitted review database ID")
	command.Flags().StringVar(&message, "message", "", "Dismissal explanation")
	command.Flags().StringVar(&messageFile, "message-file", "", "Read explanation from a file")
	command.Flags().BoolVar(&confirm, "confirm-timeline-comment", false, "Confirm GitHub timeline side effect")
	_ = command.MarkFlagRequired("review-id")
	return command
}

func (a *app) fileCommand() *cobra.Command {
	root := &cobra.Command{Use: "file", Short: "Manage viewed state for pull request files"}
	root.AddCommand(a.fileStateCommand(true), a.fileStateCommand(false))
	return root
}

func (a *app) fileStateCommand(viewed bool) *cobra.Command {
	var target target
	var path string
	var confirm bool
	name, operation := "viewed", "file.viewed"
	if !viewed {
		name, operation = "unviewed", "file.unviewed"
	}
	command := &cobra.Command{
		Use: name + " <pull-request>", Short: "Mark a pull request file " + name, Args: cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish(operation, target.repository, target.pr, nil, err)
				return
			}
			if !confirm {
				a.finish(operation, target.repository, target.pr, nil, output.NewError(output.ExitValidation, "CONFIRMATION_REQUIRED", "--confirm is required", false, nil))
				return
			}
			client, err := a.client()
			var pullRequestID string
			if err == nil {
				_, pullRequestID, err = client.ListThreads(target.repository, target.pr)
			}
			if err == nil {
				err = client.MarkFile(pullRequestID, path, viewed)
			}
			a.finish(operation, target.repository, target.pr, map[string]interface{}{"path": path, "viewed": viewed}, err)
		},
	}
	bindTarget(command, &target)
	command.Flags().StringVar(&path, "path", "", "Repository-relative changed file path")
	command.Flags().BoolVar(&confirm, "confirm", false, "Confirm viewed-state change")
	_ = command.MarkFlagRequired("path")
	return command
}

func resolveBody(body, file string) (string, error) {
	value, err := resolveBodyOptional(body, file)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(value) == "" {
		return "", output.NewError(output.ExitValidation, "BODY", "body is required", false, nil)
	}
	return value, nil
}

func resolveBodyOptional(body, file string) (string, error) {
	if body != "" && file != "" {
		return "", output.NewError(output.ExitValidation, "BODY", "pass only one of --body and --body-file", false, nil)
	}
	if file == "" {
		return body, nil
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return "", output.NewError(output.ExitValidation, "BODY_FILE", err.Error(), false, nil)
	}
	return string(data), nil
}
