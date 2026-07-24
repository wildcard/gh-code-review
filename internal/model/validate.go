package model

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
)

type Issue struct {
	Code     string `json:"code"`
	Message  string `json:"message"`
	ClientID string `json:"client_id,omitempty"`
	Path     string `json:"path,omitempty"`
}

func (m *Manifest) Validate(deriveEvent bool) []Issue {
	var issues []Issue
	add := func(code, message string) {
		issues = append(issues, Issue{Code: code, Message: message})
	}

	if m.SchemaVersion != SchemaVersion {
		add("SCHEMA_VERSION", fmt.Sprintf("schema_version must be %q", SchemaVersion))
	}
	if !validRepository(m.Repository) {
		add("REPOSITORY", "repository must be owner/name")
	}
	if m.PullRequest <= 0 {
		add("PULL_REQUEST", "pull_request must be a positive integer")
	}
	if strings.TrimSpace(m.ExpectedHeadSHA) == "" {
		add("EXPECTED_HEAD_SHA", "expected_head_sha is required")
	}
	if m.Event == "" {
		if deriveEvent {
			m.Event = DeriveEvent(m.Comments)
		} else {
			add("EVENT", "event is required unless --derive-event is used")
		}
	}
	if m.Event != "" && !validEvent(m.Event) {
		add("EVENT", "event must be COMMENT, APPROVE, or REQUEST_CHANGES")
	}
	if m.Event != EventComment && strings.TrimSpace(m.Summary) == "" {
		add("SUMMARY", "summary is required for APPROVE and REQUEST_CHANGES")
	}
	if m.Comments == nil {
		add("COMMENTS", "comments is required and must be an array")
	}

	clientIDs := map[string]struct{}{}
	fingerprints := map[string]string{}
	for i := range m.Comments {
		comment := &m.Comments[i]
		comment.Path = path.Clean(strings.TrimSpace(comment.Path))
		comment.Side = strings.ToUpper(comment.Side)
		comment.StartSide = strings.ToUpper(comment.StartSide)
		if comment.ClientID == "" {
			issues = append(issues, Issue{Code: "CLIENT_ID", Message: "client_id is required", Path: comment.Path})
		} else if _, ok := clientIDs[comment.ClientID]; ok {
			issues = append(issues, Issue{Code: "DUPLICATE_CLIENT_ID", Message: "client_id must be unique", ClientID: comment.ClientID, Path: comment.Path})
		} else {
			clientIDs[comment.ClientID] = struct{}{}
		}
		if comment.Subject != "line" && comment.Subject != "file" {
			issues = append(issues, Issue{Code: "SUBJECT", Message: "subject must be line or file", ClientID: comment.ClientID, Path: comment.Path})
		}
		if comment.Path == "." || strings.HasPrefix(comment.Path, "../") || strings.HasPrefix(comment.Path, "/") {
			issues = append(issues, Issue{Code: "PATH", Message: "path must be repository-relative", ClientID: comment.ClientID, Path: comment.Path})
		}
		if strings.TrimSpace(comment.Body) == "" && comment.Replacement == nil {
			issues = append(issues, Issue{Code: "BODY", Message: "body or replacement is required", ClientID: comment.ClientID, Path: comment.Path})
		}
		if comment.Confidence != nil && (*comment.Confidence < 0 || *comment.Confidence > 1) {
			issues = append(issues, Issue{Code: "CONFIDENCE", Message: "confidence must be between 0 and 1", ClientID: comment.ClientID, Path: comment.Path})
		}
		if comment.Severity != "" && !oneOf(comment.Severity, "blocking", "non_blocking", "suggestion", "nit") {
			issues = append(issues, Issue{Code: "SEVERITY", Message: "severity must be blocking, non_blocking, suggestion, or nit", ClientID: comment.ClientID, Path: comment.Path})
		}
		if comment.Category != "" && !oneOf(comment.Category,
			"correctness", "security", "performance", "reliability", "maintainability",
			"testing", "API-design", "documentation", "accessibility") {
			issues = append(issues, Issue{Code: "CATEGORY", Message: "category is not supported by schema v1", ClientID: comment.ClientID, Path: comment.Path})
		}

		if comment.Subject == "file" {
			if comment.Line != 0 || comment.StartLine != 0 || comment.Side != "" || comment.StartSide != "" {
				issues = append(issues, Issue{Code: "FILE_LOCATION", Message: "file subjects cannot include line or side fields", ClientID: comment.ClientID, Path: comment.Path})
			}
			if comment.Replacement != nil {
				issues = append(issues, Issue{Code: "FILE_SUGGESTION", Message: "file subjects cannot contain replacements", ClientID: comment.ClientID, Path: comment.Path})
			}
		} else {
			if comment.Line <= 0 {
				issues = append(issues, Issue{Code: "LINE", Message: "line subjects require a positive line", ClientID: comment.ClientID, Path: comment.Path})
			}
			if comment.Side != "LEFT" && comment.Side != "RIGHT" {
				issues = append(issues, Issue{Code: "SIDE", Message: "line subjects require side LEFT or RIGHT", ClientID: comment.ClientID, Path: comment.Path})
			}
			if comment.StartLine != 0 {
				if comment.StartLine <= 0 || comment.StartLine > comment.Line {
					issues = append(issues, Issue{Code: "START_LINE", Message: "start_line must be positive and no greater than line", ClientID: comment.ClientID, Path: comment.Path})
				}
				if comment.StartSide != "LEFT" && comment.StartSide != "RIGHT" {
					issues = append(issues, Issue{Code: "START_SIDE", Message: "range subjects require start_side LEFT or RIGHT", ClientID: comment.ClientID, Path: comment.Path})
				}
				if comment.StartSide != comment.Side {
					issues = append(issues, Issue{Code: "MIXED_SIDE_RANGE", Message: "ranges cannot cross diff sides", ClientID: comment.ClientID, Path: comment.Path})
				}
			} else if comment.StartSide != "" {
				issues = append(issues, Issue{Code: "START_SIDE", Message: "start_side requires start_line", ClientID: comment.ClientID, Path: comment.Path})
			}
			if comment.Replacement != nil && (comment.Side != "RIGHT" || (comment.StartLine != 0 && comment.StartSide != "RIGHT")) {
				issues = append(issues, Issue{Code: "SUGGESTION_SIDE", Message: "replacements are valid only on RIGHT-side line or range subjects", ClientID: comment.ClientID, Path: comment.Path})
			}
		}

		fp := comment.Fingerprint(m.Repository, m.PullRequest, m.ExpectedHeadSHA)
		if previous, ok := fingerprints[fp]; ok {
			issues = append(issues, Issue{Code: "DUPLICATE_FINDING", Message: fmt.Sprintf("same normalized finding as %s", previous), ClientID: comment.ClientID, Path: comment.Path})
		} else {
			fingerprints[fp] = comment.ClientID
		}
	}

	sort.SliceStable(issues, func(i, j int) bool {
		if issues[i].ClientID == issues[j].ClientID {
			return issues[i].Code < issues[j].Code
		}
		return issues[i].ClientID < issues[j].ClientID
	})
	return issues
}

func (c Comment) Fingerprint(repository string, pullRequest int, headSHA string) string {
	parts := []string{
		strings.ToLower(repository),
		fmt.Sprint(pullRequest),
		strings.ToLower(headSHA),
		path.Clean(c.Path),
		c.Subject,
		fmt.Sprint(c.StartLine),
		strings.ToUpper(c.StartSide),
		fmt.Sprint(c.Line),
		strings.ToUpper(c.Side),
		normalizeText(c.GitHubBody()),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func normalizeText(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}

func validRepository(repository string) bool {
	parts := strings.Split(repository, "/")
	return len(parts) == 2 && parts[0] != "" && parts[1] != "" && !strings.ContainsAny(repository, " \t\n")
}

func validEvent(event Event) bool {
	return event == EventComment || event == EventApprove || event == EventRequestChanges
}

func oneOf(value string, allowed ...string) bool {
	for _, candidate := range allowed {
		if value == candidate {
			return true
		}
	}
	return false
}
