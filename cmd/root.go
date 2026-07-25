package cmd

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/cli/go-gh/v2/pkg/repository"
	"github.com/spf13/cobra"
	gh "github.com/wildcard/gh-code-review/internal/github"
	"github.com/wildcard/gh-code-review/internal/output"
)

var version = "0.2.1"

type app struct {
	exitCode int
	host     string
}

type target struct {
	repository string
	pr         int
}

func Execute() int {
	a := &app{}
	root := a.rootCommand()
	if err := root.Execute(); err != nil {
		return output.WriteError(os.Stdout, "command", "", 0, output.NewError(output.ExitValidation, "USAGE", err.Error(), false, nil))
	}
	return a.exitCode
}

func (a *app) rootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "gh-code-review",
		Short:         "Validated, agent-friendly GitHub pull request reviews",
		SilenceErrors: true,
		SilenceUsage:  true,
		Version:       version,
	}
	root.SetOut(os.Stdout)
	root.SetErr(os.Stderr)
	root.PersistentFlags().StringVar(&a.host, "host", "", "GitHub host (default: current gh host)")
	root.AddCommand(
		a.inspectCommand(),
		a.validateCommand(),
		a.submitCommand(),
		a.capabilitiesCommand(),
		a.pendingCommand(),
		a.commentCommand(),
		a.threadCommand(),
		a.reviewCommand(),
		a.fileCommand(),
	)
	return root
}

func (a *app) client() (*gh.Client, error) {
	return gh.New(a.host)
}

func (a *app) finish(operation, repo string, pr int, result interface{}, err error) {
	if err != nil {
		a.exitCode = output.WriteError(os.Stdout, operation, repo, pr, err)
		return
	}
	a.exitCode = output.WriteSuccess(os.Stdout, operation, repo, pr, result)
}

func bindTarget(command *cobra.Command, target *target) {
	command.Flags().StringVarP(&target.repository, "repo", "R", "", "Repository in owner/name form")
}

func resolveTarget(command *cobra.Command, target *target, args []string) error {
	if len(args) > 0 {
		number, err := strconv.Atoi(args[0])
		if err != nil || number <= 0 {
			return output.NewError(output.ExitValidation, "PULL_REQUEST", "pull request must be a positive integer", false, nil)
		}
		target.pr = number
	}
	if target.repository == "" {
		current, err := repository.Current()
		if err != nil {
			return output.NewError(output.ExitValidation, "REPOSITORY", "pass --repo owner/name outside a GitHub repository", false, nil)
		}
		target.repository = current.Owner + "/" + current.Name
	}
	if target.pr == 0 {
		return output.NewError(output.ExitValidation, "PULL_REQUEST", "pull request number is required", false, nil)
	}
	return nil
}

func parseCSV(value string) map[string]bool {
	result := map[string]bool{}
	for _, item := range strings.Split(value, ",") {
		item = strings.TrimSpace(strings.ToLower(item))
		if item != "" {
			result[item] = true
		}
	}
	return result
}

func positiveID(value int64, name string) error {
	if value <= 0 {
		return output.NewError(output.ExitValidation, strings.ToUpper(strings.ReplaceAll(name, "-", "_")),
			fmt.Sprintf("%s must be a positive integer", name), false, nil)
	}
	return nil
}
