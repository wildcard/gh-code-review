package model

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestManifestValidateAndDeriveEvent(t *testing.T) {
	replacement := "return nil"
	manifest := Manifest{
		SchemaVersion:   SchemaVersion,
		Repository:      "owner/repo",
		PullRequest:     7,
		ExpectedHeadSHA: "abc123",
		Summary:         "Please address the blocking issue.",
		Comments: []Comment{{
			ClientID: "correctness-1", Subject: "line", Path: "src/a.go",
			Line: 10, Side: "right", Body: "Return early.", Replacement: &replacement,
			Severity: "blocking",
		}},
	}
	issues := manifest.Validate(true)
	if len(issues) != 0 {
		t.Fatalf("unexpected issues: %#v", issues)
	}
	if manifest.Event != EventRequestChanges {
		t.Fatalf("expected derived request changes, got %s", manifest.Event)
	}
	if !strings.Contains(manifest.Comments[0].GitHubBody(), "```suggestion\nreturn nil\n```") {
		t.Fatalf("suggestion body was not generated safely: %q", manifest.Comments[0].GitHubBody())
	}
}

func TestManifestRejectsInvalidSuggestionAndDuplicates(t *testing.T) {
	replacement := "x"
	manifest := Manifest{
		SchemaVersion: SchemaVersion, Repository: "owner/repo", PullRequest: 1,
		ExpectedHeadSHA: "head", Event: EventComment,
		Comments: []Comment{
			{ClientID: "same", Subject: "line", Path: "a.go", Line: 3, Side: "LEFT", Replacement: &replacement},
			{ClientID: "same", Subject: "line", Path: "a.go", Line: 3, Side: "LEFT", Replacement: &replacement},
		},
	}
	issues := manifest.Validate(false)
	codes := map[string]bool{}
	for _, issue := range issues {
		codes[issue.Code] = true
	}
	for _, expected := range []string{"SUGGESTION_SIDE", "DUPLICATE_CLIENT_ID", "DUPLICATE_FINDING"} {
		if !codes[expected] {
			t.Errorf("expected %s in %#v", expected, issues)
		}
	}
}

func TestLoadManifestJSONAndYAMLWithStrictFields(t *testing.T) {
	dir := t.TempDir()
	jsonPath := filepath.Join(dir, "review.json")
	yamlPath := filepath.Join(dir, "review.yaml")
	contentJSON := `{"schema_version":"1.0","repository":"o/r","pull_request":2,"expected_head_sha":"h","event":"COMMENT"}`
	contentYAML := "schema_version: \"1.0\"\nrepository: o/r\npull_request: 2\nexpected_head_sha: h\nevent: COMMENT\n"
	if err := os.WriteFile(jsonPath, []byte(contentJSON), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yamlPath, []byte(contentYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{jsonPath, yamlPath} {
		manifest, err := LoadManifest(path)
		if err != nil {
			t.Fatalf("%s: %v", path, err)
		}
		if manifest.PullRequest != 2 {
			t.Fatalf("%s: wrong manifest %#v", path, manifest)
		}
	}
	if err := os.WriteFile(jsonPath, []byte(`{"schema_version":"1.0","unknown":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(jsonPath); err == nil {
		t.Fatal("unknown JSON field should be rejected")
	}
}

func TestDecodeManifestFromStdinFormatDetection(t *testing.T) {
	for name, content := range map[string]string{
		"JSON": `{"schema_version":"1.0","repository":"o/r","pull_request":2,"expected_head_sha":"h","event":"COMMENT"}`,
		"YAML": "schema_version: \"1.0\"\nrepository: o/r\npull_request: 2\nexpected_head_sha: h\nevent: COMMENT\n",
	} {
		manifest, err := DecodeManifest([]byte(content), "-")
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if manifest.PullRequest != 2 {
			t.Fatalf("%s: wrong manifest %#v", name, manifest)
		}
	}
}

func TestFingerprintExcludesAgentMetadata(t *testing.T) {
	a := Comment{ClientID: "a", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "same", Severity: "blocking"}
	b := a
	b.ClientID = "b"
	b.Severity = "nit"
	b.Evidence = []string{"local test output"}
	if a.Fingerprint("o/r", 1, "h") != b.Fingerprint("o/r", 1, "h") {
		t.Fatal("agent-only metadata must not affect normalized GitHub finding identity")
	}
}

func TestSharedManifestFixtures(t *testing.T) {
	for _, name := range []string{"review_line.json", "review_mixed.json"} {
		manifest, err := LoadManifest(filepath.Join("../../tests/fixtures", name))
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if issues := manifest.Validate(false); len(issues) != 0 {
			t.Fatalf("%s: %#v", name, issues)
		}
	}
}
