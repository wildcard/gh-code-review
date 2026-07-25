package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const SchemaVersion = "1.0"

type Event string

const (
	EventComment        Event = "COMMENT"
	EventApprove        Event = "APPROVE"
	EventRequestChanges Event = "REQUEST_CHANGES"
)

type Manifest struct {
	SchemaVersion   string    `json:"schema_version" yaml:"schema_version"`
	Repository      string    `json:"repository" yaml:"repository"`
	PullRequest     int       `json:"pull_request" yaml:"pull_request"`
	ExpectedHeadSHA string    `json:"expected_head_sha" yaml:"expected_head_sha"`
	Event           Event     `json:"event,omitempty" yaml:"event,omitempty"`
	Summary         string    `json:"summary,omitempty" yaml:"summary,omitempty"`
	IdempotencyKey  string    `json:"idempotency_key,omitempty" yaml:"idempotency_key,omitempty"`
	Comments        []Comment `json:"comments,omitempty" yaml:"comments,omitempty"`
}

type Comment struct {
	ClientID    string  `json:"client_id" yaml:"client_id"`
	Subject     string  `json:"subject" yaml:"subject"`
	Path        string  `json:"path" yaml:"path"`
	Body        string  `json:"body" yaml:"body"`
	Line        int     `json:"line,omitempty" yaml:"line,omitempty"`
	Side        string  `json:"side,omitempty" yaml:"side,omitempty"`
	StartLine   int     `json:"start_line,omitempty" yaml:"start_line,omitempty"`
	StartSide   string  `json:"start_side,omitempty" yaml:"start_side,omitempty"`
	Replacement *string `json:"replacement,omitempty" yaml:"replacement,omitempty"`

	// Agent metadata is local-only and is never included in GitHub API payloads.
	Severity   string   `json:"severity,omitempty" yaml:"severity,omitempty"`
	Category   string   `json:"category,omitempty" yaml:"category,omitempty"`
	Confidence *float64 `json:"confidence,omitempty" yaml:"confidence,omitempty"`
	RuleID     string   `json:"rule_id,omitempty" yaml:"rule_id,omitempty"`
	Evidence   []string `json:"evidence,omitempty" yaml:"evidence,omitempty"`
}

type APIComment struct {
	Path      string `json:"path"`
	Body      string `json:"body"`
	Line      int    `json:"line,omitempty"`
	Side      string `json:"side,omitempty"`
	StartLine int    `json:"start_line,omitempty"`
	StartSide string `json:"start_side,omitempty"`
}

func (c Comment) GitHubBody() string {
	if c.Replacement == nil {
		return strings.TrimSpace(c.Body)
	}
	body := strings.TrimSpace(c.Body)
	if body != "" {
		body += "\n\n"
	}
	return body + "```suggestion\n" + strings.TrimSuffix(*c.Replacement, "\n") + "\n```"
}

func (c Comment) APIComment() APIComment {
	return APIComment{
		Path:      c.Path,
		Body:      c.GitHubBody(),
		Line:      c.Line,
		Side:      strings.ToUpper(c.Side),
		StartLine: c.StartLine,
		StartSide: strings.ToUpper(c.StartSide),
	}
}

func LoadManifest(path string) (*Manifest, error) {
	var (
		data []byte
		err  error
	)
	if path == "-" {
		data, err = io.ReadAll(os.Stdin)
	} else {
		data, err = os.ReadFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	return DecodeManifest(data, path)
}

func DecodeManifest(data []byte, source string) (*Manifest, error) {
	var manifest Manifest
	format := strings.ToLower(filepath.Ext(source))
	if source == "-" {
		trimmed := bytes.TrimSpace(data)
		if len(trimmed) > 0 && (trimmed[0] == '{' || trimmed[0] == '[') {
			format = ".json"
		} else {
			format = ".yaml"
		}
	}
	switch format {
	case ".yaml", ".yml":
		dec := yaml.NewDecoder(bytes.NewReader(data))
		dec.KnownFields(true)
		if err := dec.Decode(&manifest); err != nil {
			return nil, fmt.Errorf("decode YAML manifest: %w", err)
		}
	default:
		dec := json.NewDecoder(bytes.NewReader(data))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&manifest); err != nil {
			return nil, fmt.Errorf("decode JSON manifest: %w", err)
		}
	}
	return &manifest, nil
}

func DeriveEvent(comments []Comment) Event {
	if len(comments) == 0 {
		return EventApprove
	}
	for _, comment := range comments {
		if strings.EqualFold(comment.Severity, "blocking") {
			return EventRequestChanges
		}
	}
	return EventComment
}
