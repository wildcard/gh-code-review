package cmd

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/wildcard/gh-code-review/internal/diff"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
	reviewsvc "github.com/wildcard/gh-code-review/internal/review"
)

func (a *app) inspectCommand() *cobra.Command {
	var target target
	var include string
	var unresolved bool
	command := &cobra.Command{
		Use:   "inspect <pull-request>",
		Short: "Read normalized pull request, diff, checks, and review context",
		Args:  cobra.MaximumNArgs(1),
		Run: func(command *cobra.Command, args []string) {
			if err := resolveTarget(command, &target, args); err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			includes := parseCSV(include)
			pull, err := client.GetPull(target.repository, target.pr)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			files, err := client.ListFiles(target.repository, target.pr, true)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			index := diff.Build(files)
			if !includes["patches"] {
				for i := range files {
					files[i].Patch = ""
				}
			}
			reviews, err := client.ListReviews(target.repository, target.pr)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			comments, err := client.ListReviewComments(target.repository, target.pr)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			threads, _, err := client.ListThreads(target.repository, target.pr)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			if unresolved {
				filtered := threads[:0]
				for _, thread := range threads {
					if !thread.IsResolved {
						filtered = append(filtered, thread)
					}
				}
				threads = filtered
			}
			if !includes["bodies"] {
				for i := range comments {
					comments[i].Body = ""
				}
				for i := range reviews {
					reviews[i].Body = ""
				}
				for i := range threads {
					for j := range threads[i].Comments {
						threads[i].Comments[j].Body = ""
					}
				}
			}
			checks, statuses, err := client.Checks(target.repository, pull.Head.SHA)
			if err != nil {
				a.finish("inspect", target.repository, target.pr, nil, err)
				return
			}
			result := map[string]interface{}{
				"pull_request":       pull,
				"files":              files,
				"commentable_ranges": index.CompactLocations(),
				"reviews":            reviews,
				"review_comments":    comments,
				"threads":            threads,
				"checks":             checks,
				"commit_status":      statuses,
			}
			a.finish("inspect", target.repository, target.pr, result, nil)
		},
	}
	bindTarget(command, &target)
	command.Flags().StringVar(&include, "include", "", "Comma-separated extra fields: patches,bodies")
	command.Flags().BoolVar(&unresolved, "unresolved", false, "Return only unresolved review threads")
	return command
}

func (a *app) validateCommand() *cobra.Command {
	var input string
	var deriveEvent bool
	var resumeReview int64
	var includePatch bool
	command := &cobra.Command{
		Use:   "validate --input <review.json|review.yaml>",
		Short: "Validate a review manifest without writing to GitHub",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			manifest, err := model.LoadManifest(input)
			if err != nil {
				a.finish("validate", "", 0, nil, output.NewError(output.ExitValidation, "MANIFEST", err.Error(), false, nil))
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish("validate", manifest.Repository, manifest.PullRequest, nil, err)
				return
			}
			result, err := reviewsvc.Validate(client, manifest, reviewsvc.ValidateOptions{
				DeriveEvent: deriveEvent, ResumeReview: resumeReview, IncludePatch: includePatch,
			})
			if err == nil {
				err = reviewsvc.ValidationError(result)
			}
			if err != nil {
				a.finish("validate", manifest.Repository, manifest.PullRequest, result, err)
				return
			}
			a.finish("validate", manifest.Repository, manifest.PullRequest, result, nil)
		},
	}
	command.Flags().StringVarP(&input, "input", "i", "", "Review manifest path (JSON or YAML)")
	_ = command.MarkFlagRequired("input")
	command.Flags().BoolVar(&deriveEvent, "derive-event", false, "Derive the event from finding severities")
	command.Flags().Int64Var(&resumeReview, "resume-review", 0, "Explicit pending review ID to resume")
	command.Flags().BoolVar(&includePatch, "include-patches", false, "Include file patches in the result")
	return command
}

func (a *app) submitCommand() *cobra.Command {
	var input string
	var deriveEvent, confirmDecision, dryRun bool
	var resumeReview int64
	command := &cobra.Command{
		Use:   "submit --input <review.json|review.yaml>",
		Short: "Validate and submit one coherent pull request review",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			manifest, err := model.LoadManifest(input)
			if err != nil {
				a.finish("submit", "", 0, nil, output.NewError(output.ExitValidation, "MANIFEST", err.Error(), false, nil))
				return
			}
			client, err := a.client()
			if err != nil {
				a.finish("submit", manifest.Repository, manifest.PullRequest, nil, err)
				return
			}
			result, err := reviewsvc.Submit(client, manifest, reviewsvc.SubmitOptions{
				DeriveEvent: deriveEvent, ConfirmDecision: confirmDecision,
				ResumeReview: resumeReview, DryRun: dryRun,
			})
			a.finish("submit", manifest.Repository, manifest.PullRequest, result, err)
		},
	}
	command.Flags().StringVarP(&input, "input", "i", "", "Review manifest path (JSON or YAML)")
	_ = command.MarkFlagRequired("input")
	command.Flags().BoolVar(&deriveEvent, "derive-event", false, "Derive the event from finding severities")
	command.Flags().BoolVar(&confirmDecision, "confirm-decision", false, "Confirm APPROVE or REQUEST_CHANGES")
	command.Flags().BoolVar(&dryRun, "dry-run", false, "Validate and show the transport plan without writing")
	command.Flags().Int64Var(&resumeReview, "resume-review", 0, "Explicit pending review ID to resume")
	return command
}

func (a *app) capabilitiesCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "capabilities",
		Short: "Report host and command capabilities",
		Args:  cobra.NoArgs,
		Run: func(command *cobra.Command, _ []string) {
			client, err := a.client()
			if err != nil {
				a.finish("capabilities", "", 0, nil, err)
				return
			}
			githubDotCom := strings.EqualFold(client.Host, "github.com")
			result := map[string]interface{}{
				"host": client.Host,
				"host_support": map[string]interface{}{
					"github_com": githubDotCom,
					"status": func() string {
						if githubDotCom {
							return "supported"
						}
						return "capability-check-required"
					}(),
				},
				"manifest": map[string]interface{}{
					"schema_version": model.SchemaVersion,
					"formats":        []string{"json", "yaml"},
					"subjects":       []string{"line", "file"},
					"events":         []model.Event{model.EventComment, model.EventApprove, model.EventRequestChanges},
					"suggestions":    "RIGHT-side line and range subjects",
				},
				"transports": map[string]string{
					"line_only": "REST pending-review batch",
					"mixed":     "GraphQL pending-review threads",
				},
				"exit_codes": map[string]int{
					"success": output.ExitSuccess, "validation": output.ExitValidation,
					"head_changed": output.ExitHeadChanged, "authorization": output.ExitAuth,
					"api": output.ExitAPI, "partial_transaction": output.ExitPartial,
					"unsupported": output.ExitUnsupported, "conflict": output.ExitConflict,
				},
				"version": version,
			}
			if !githubDotCom {
				result["warning"] = fmt.Sprintf("%s is not a fully supported v0.1 host; mutations return structured capability errors when unavailable", client.Host)
			}
			a.finish("capabilities", "", 0, result, nil)
		},
	}
}
