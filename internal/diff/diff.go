package diff

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/wildcard/gh-code-review/internal/model"
)

type File struct {
	Path         string `json:"path"`
	PreviousPath string `json:"previous_path,omitempty"`
	Status       string `json:"status"`
	Additions    int    `json:"additions"`
	Deletions    int    `json:"deletions"`
	Changes      int    `json:"changes"`
	Patch        string `json:"patch,omitempty"`
	Binary       bool   `json:"binary"`
	Truncated    bool   `json:"truncated"`
}

type Location struct {
	Path    string `json:"path"`
	Line    int    `json:"line"`
	Side    string `json:"side"`
	Hunk    int    `json:"hunk"`
	Context bool   `json:"context"`
}

type Index struct {
	Files     map[string]File
	Locations map[string]Location
	Previous  map[string]string
}

var hunkRE = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

func Build(files []File) *Index {
	index := &Index{
		Files:     make(map[string]File, len(files)),
		Locations: map[string]Location{},
		Previous:  map[string]string{},
	}
	for _, file := range files {
		index.Files[file.Path] = file
		if file.PreviousPath != "" {
			index.Previous[file.PreviousPath] = file.Path
		}
		if file.Patch == "" {
			continue
		}
		parsePatch(index, file)
	}
	return index
}

func parsePatch(index *Index, file File) {
	oldLine, newLine, hunk := 0, 0, -1
	for _, line := range strings.Split(file.Patch, "\n") {
		if match := hunkRE.FindStringSubmatch(line); match != nil {
			oldLine, _ = strconv.Atoi(match[1])
			newLine, _ = strconv.Atoi(match[3])
			hunk++
			continue
		}
		if hunk < 0 || line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			index.add(file.Path, oldLine, "LEFT", hunk, true)
			index.add(file.Path, newLine, "RIGHT", hunk, true)
			oldLine++
			newLine++
		case '-':
			index.add(file.Path, oldLine, "LEFT", hunk, false)
			oldLine++
		case '+':
			index.add(file.Path, newLine, "RIGHT", hunk, false)
			newLine++
		case '\\':
			// "\ No newline at end of file" consumes no source line.
		}
	}
}

func (i *Index) add(path string, line int, side string, hunk int, context bool) {
	key := locationKey(path, line, side)
	i.Locations[key] = Location{Path: path, Line: line, Side: side, Hunk: hunk, Context: context}
}

func (i *Index) ValidateComment(comment model.Comment) []model.Issue {
	var issues []model.Issue
	file, exists := i.Files[comment.Path]
	if !exists {
		if renamedTo, ok := i.Previous[comment.Path]; ok {
			issues = append(issues, issue("RENAMED_PATH", fmt.Sprintf("use renamed path %q", renamedTo), comment))
		} else {
			issues = append(issues, issue("PATH_NOT_IN_DIFF", "path is not changed by this pull request", comment))
		}
		return issues
	}
	if comment.Subject == "file" {
		return issues
	}
	if file.Binary || file.Patch == "" {
		code := "BINARY_FILE"
		message := "binary files support only file-level comments"
		if file.Truncated && !file.Binary {
			code = "PATCH_UNAVAILABLE"
			message = "the patch is unavailable or truncated; use a file-level comment"
		}
		return append(issues, issue(code, message, comment))
	}

	end, ok := i.Locations[locationKey(comment.Path, comment.Line, comment.Side)]
	if !ok {
		issues = append(issues, issue("LINE_NOT_IN_DIFF", fmt.Sprintf("%s line %d is not commentable in the current diff", comment.Side, comment.Line), comment))
		return issues
	}
	if comment.StartLine != 0 {
		start, ok := i.Locations[locationKey(comment.Path, comment.StartLine, comment.StartSide)]
		if !ok {
			issues = append(issues, issue("START_LINE_NOT_IN_DIFF", fmt.Sprintf("%s line %d is not commentable in the current diff", comment.StartSide, comment.StartLine), comment))
			return issues
		}
		if start.Hunk != end.Hunk {
			issues = append(issues, issue("CROSS_HUNK_RANGE", "a multiline comment cannot cross diff hunks", comment))
		}
		for line := comment.StartLine; line <= comment.Line; line++ {
			if _, ok := i.Locations[locationKey(comment.Path, line, comment.Side)]; !ok {
				issues = append(issues, issue("RANGE_GAP", fmt.Sprintf("%s line %d is not commentable in this range", comment.Side, line), comment))
				break
			}
		}
	}
	return issues
}

func (i *Index) CompactLocations() map[string]map[string][][2]int {
	result := map[string]map[string][][2]int{}
	for path := range i.Files {
		result[path] = map[string][][2]int{"LEFT": {}, "RIGHT": {}}
		for _, side := range []string{"LEFT", "RIGHT"} {
			var lines []int
			for _, location := range i.Locations {
				if location.Path == path && location.Side == side {
					lines = append(lines, location.Line)
				}
			}
			result[path][side] = ranges(lines)
		}
	}
	return result
}

func ranges(lines []int) [][2]int {
	if len(lines) == 0 {
		return [][2]int{}
	}
	sortInts(lines)
	out := make([][2]int, 0)
	start, end := lines[0], lines[0]
	for _, line := range lines[1:] {
		if line == end || line == end+1 {
			if line > end {
				end = line
			}
			continue
		}
		out = append(out, [2]int{start, end})
		start, end = line, line
	}
	return append(out, [2]int{start, end})
}

func sortInts(values []int) {
	for i := 1; i < len(values); i++ {
		for j := i; j > 0 && values[j] < values[j-1]; j-- {
			values[j], values[j-1] = values[j-1], values[j]
		}
	}
}

func locationKey(path string, line int, side string) string {
	return fmt.Sprintf("%s\x00%d\x00%s", path, line, strings.ToUpper(side))
}

func issue(code, message string, comment model.Comment) model.Issue {
	return model.Issue{Code: code, Message: message, ClientID: comment.ClientID, Path: comment.Path}
}
