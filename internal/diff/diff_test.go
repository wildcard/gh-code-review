package diff

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/wildcard/gh-code-review/internal/model"
)

func TestBuildAndValidateLocations(t *testing.T) {
	files := []File{{
		Path: "a.go", Status: "modified",
		Patch: "@@ -1,3 +1,4 @@\n one\n-old\n+new\n+extra\n three\n@@ -20,2 +21,2 @@\n old-context\n-last\n+last-new",
	}}
	index := Build(files)
	for _, test := range []struct {
		line int
		side string
	}{
		{1, "LEFT"}, {1, "RIGHT"}, {2, "LEFT"}, {2, "RIGHT"}, {3, "RIGHT"}, {21, "LEFT"}, {22, "RIGHT"},
	} {
		if _, ok := index.Locations[locationKey("a.go", test.line, test.side)]; !ok {
			t.Errorf("missing %s:%d %s", "a.go", test.line, test.side)
		}
	}
	valid := model.Comment{ClientID: "valid", Subject: "line", Path: "a.go", StartLine: 2, StartSide: "RIGHT", Line: 3, Side: "RIGHT"}
	if issues := index.ValidateComment(valid); len(issues) != 0 {
		t.Fatalf("valid range rejected: %#v", issues)
	}
	crossHunk := model.Comment{ClientID: "cross", Subject: "line", Path: "a.go", StartLine: 3, StartSide: "RIGHT", Line: 22, Side: "RIGHT"}
	issues := index.ValidateComment(crossHunk)
	if !hasCode(issues, "CROSS_HUNK_RANGE") {
		t.Fatalf("expected cross hunk issue: %#v", issues)
	}
}

func TestSharedDiffFixture(t *testing.T) {
	data, err := os.ReadFile("../../tests/fixtures/diff_files.json")
	if err != nil {
		t.Fatal(err)
	}
	var raw []struct {
		Filename         string `json:"filename"`
		PreviousFilename string `json:"previous_filename"`
		Status           string `json:"status"`
		Additions        int    `json:"additions"`
		Deletions        int    `json:"deletions"`
		Changes          int    `json:"changes"`
		Patch            string `json:"patch"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	files := make([]File, 0, len(raw))
	for _, item := range raw {
		binary := item.Patch == "" && item.Changes > 0 && item.Additions == 0 && item.Deletions == 0
		files = append(files, File{
			Path: item.Filename, PreviousPath: item.PreviousFilename, Status: item.Status,
			Additions: item.Additions, Deletions: item.Deletions, Changes: item.Changes,
			Patch: item.Patch, Binary: binary, Truncated: item.Patch == "" && item.Changes > 0 && !binary,
		})
	}
	index := Build(files)
	if _, ok := index.Locations[locationKey("src/deleted.go", 1, "LEFT")]; !ok {
		t.Fatal("shared fixture lost LEFT deleted line")
	}
	if index.Previous["src/old_name.go"] != "src/new_name.go" {
		t.Fatal("shared fixture lost rename mapping")
	}
	if !index.Files["assets/logo.png"].Binary {
		t.Fatal("shared fixture lost binary classification")
	}
}

func TestRenamedBinaryAndUnavailablePatch(t *testing.T) {
	index := Build([]File{
		{Path: "new.go", PreviousPath: "old.go", Status: "renamed", Patch: "@@ -1 +1 @@\n-old\n+new"},
		{Path: "asset.png", Status: "modified", Binary: true},
		{Path: "large.txt", Status: "modified", Truncated: true},
	})
	if issues := index.ValidateComment(model.Comment{ClientID: "r", Subject: "line", Path: "old.go", Line: 1, Side: "RIGHT"}); !hasCode(issues, "RENAMED_PATH") {
		t.Fatalf("expected renamed path issue: %#v", issues)
	}
	if issues := index.ValidateComment(model.Comment{ClientID: "b", Subject: "line", Path: "asset.png", Line: 1, Side: "RIGHT"}); !hasCode(issues, "BINARY_FILE") {
		t.Fatalf("expected binary issue: %#v", issues)
	}
	if issues := index.ValidateComment(model.Comment{ClientID: "f", Subject: "file", Path: "asset.png"}); len(issues) != 0 {
		t.Fatalf("file comment on binary should be valid: %#v", issues)
	}
	if issues := index.ValidateComment(model.Comment{ClientID: "l", Subject: "line", Path: "large.txt", Line: 1, Side: "RIGHT"}); !hasCode(issues, "PATCH_UNAVAILABLE") {
		t.Fatalf("expected unavailable patch issue: %#v", issues)
	}
}

func hasCode(issues []model.Issue, code string) bool {
	for _, issue := range issues {
		if issue.Code == code {
			return true
		}
	}
	return false
}
