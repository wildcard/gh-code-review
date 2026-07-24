package output

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	ExitSuccess     = 0
	ExitInternal    = 1
	ExitValidation  = 2
	ExitHeadChanged = 3
	ExitAuth        = 4
	ExitAPI         = 5
	ExitPartial     = 6
	ExitUnsupported = 7
	ExitConflict    = 8
)

type ErrorDetail struct {
	Code      string      `json:"code"`
	Message   string      `json:"message"`
	Retryable bool        `json:"retryable"`
	Details   interface{} `json:"details,omitempty"`
}

type Envelope struct {
	SchemaVersion string       `json:"schema_version"`
	OK            bool         `json:"ok"`
	Operation     string       `json:"operation"`
	Repository    string       `json:"repository,omitempty"`
	PullRequest   int          `json:"pull_request,omitempty"`
	Result        interface{}  `json:"result,omitempty"`
	Error         *ErrorDetail `json:"error,omitempty"`
}

type CLIError struct {
	ExitCode  int
	Code      string
	Message   string
	Retryable bool
	Details   interface{}
}

func (e *CLIError) Error() string { return e.Message }

func NewError(exitCode int, code, message string, retryable bool, details interface{}) *CLIError {
	return &CLIError{ExitCode: exitCode, Code: code, Message: message, Retryable: retryable, Details: details}
}

func Write(w io.Writer, envelope Envelope) error {
	envelope.SchemaVersion = "1.0"
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(envelope)
}

func WriteSuccess(w io.Writer, operation, repository string, pullRequest int, result interface{}) int {
	if err := Write(w, Envelope{OK: true, Operation: operation, Repository: repository, PullRequest: pullRequest, Result: result}); err != nil {
		_, _ = fmt.Fprintf(w, `{"schema_version":"1.0","ok":false,"operation":%q,"error":{"code":"OUTPUT","message":%q,"retryable":false}}`+"\n", operation, err.Error())
		return ExitInternal
	}
	return ExitSuccess
}

func WriteError(w io.Writer, operation, repository string, pullRequest int, err error) int {
	var cliErr *CLIError
	if !errors.As(err, &cliErr) {
		cliErr = NewError(ExitInternal, "INTERNAL", err.Error(), false, nil)
	}
	_ = Write(w, Envelope{
		OK:          false,
		Operation:   operation,
		Repository:  repository,
		PullRequest: pullRequest,
		Error: &ErrorDetail{
			Code:      cliErr.Code,
			Message:   cliErr.Message,
			Retryable: cliErr.Retryable,
			Details:   cliErr.Details,
		},
	})
	return cliErr.ExitCode
}
