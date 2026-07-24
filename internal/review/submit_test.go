package review

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	gh "github.com/wildcard/gh-code-review/internal/github"
	"github.com/wildcard/gh-code-review/internal/journal"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

type testRoundTrip func(*http.Request) (*http.Response, error)

func (f testRoundTrip) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func apiResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status),
		Header: http.Header{"Content-Type": []string{"application/json"}},
		Body:   io.NopCloser(strings.NewReader(body)), Request: request,
	}
}

func clientWith(t *testing.T, transport http.RoundTripper) *gh.Client {
	t.Helper()
	client, err := gh.NewWithOptions(api.ClientOptions{
		Host: "github.com", AuthToken: "test", Transport: transport,
		Headers: map[string]string{"X-GitHub-Api-Version": gh.APIVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func manifestWith(comments []model.Comment) *model.Manifest {
	return &model.Manifest{
		SchemaVersion: model.SchemaVersion, Repository: "o/r", PullRequest: 7,
		ExpectedHeadSHA: "abc123", Event: model.EventComment, Summary: "Focused review.",
		Comments: comments,
	}
}

func TestSubmitLineOnlyUsesRESTBatchAndSubmits(t *testing.T) {
	var createdPayload map[string]interface{}
	createCalls, submitCalls := 0, 0
	client := clientWith(t, testRoundTrip(func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		switch {
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7":
			return apiResponse(request, 200, `{"number":7,"head":{"sha":"abc123"},"user":{"login":"author"}}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/files":
			return apiResponse(request, 200, `[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"changes":1,"patch":"@@ -1 +1 @@\n-old\n+new"}]`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodGet && path == "/user":
			return apiResponse(request, 200, `{"login":"reviewer"}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/comments":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodPost && path == "/graphql":
			return apiResponse(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews":
			createCalls++
			data, _ := io.ReadAll(request.Body)
			_ = json.Unmarshal(data, &createdPayload)
			return apiResponse(request, 200, `{"id":9,"node_id":"R_9","state":"PENDING"}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews/9/events":
			submitCalls++
			return apiResponse(request, 200, `{"id":9,"node_id":"R_9","state":"COMMENTED","html_url":"https://github.test/review/9"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	}))
	receipt, err := Submit(client, manifestWith([]model.Comment{{
		ClientID: "line-1", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "Finding.",
	}}), SubmitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if createCalls != 1 || submitCalls != 1 || receipt.Transport != "rest-batch" || receipt.ReviewURL == "" {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	comments, ok := createdPayload["comments"].([]interface{})
	if !ok || len(comments) != 1 {
		t.Fatalf("comments were not batched: %#v", createdPayload)
	}
}

func TestSubmitMixedUsesGraphQLThreads(t *testing.T) {
	var graphBodies []string
	client := clientWith(t, testRoundTrip(func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		switch {
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7":
			return apiResponse(request, 200, `{"number":7,"head":{"sha":"abc123"},"user":{"login":"author"}}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/files":
			return apiResponse(request, 200, `[
			  {"filename":"a.go","status":"modified","additions":1,"deletions":0,"changes":1,"patch":"@@ -1 +1 @@\n-old\n+new"},
			  {"filename":"asset.png","status":"modified","additions":0,"deletions":0,"changes":1}
			]`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodGet && path == "/user":
			return apiResponse(request, 200, `{"login":"reviewer"}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/comments":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodPost && path == "/graphql":
			data, _ := io.ReadAll(request.Body)
			graphBodies = append(graphBodies, string(data))
			if strings.Contains(string(data), "reviewThreads") {
				return apiResponse(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false,"endCursor":null},"nodes":[]}}}}}`), nil
			}
			return apiResponse(request, 200, `{"data":{"addPullRequestReviewThread":{"thread":{"id":"T_1","path":"a.go","subjectType":"LINE"}}}}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `{"id":10,"node_id":"R_10","state":"PENDING"}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews/10/events":
			return apiResponse(request, 200, `{"id":10,"state":"COMMENTED","html_url":"https://github.test/review/10"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	}))
	receipt, err := Submit(client, manifestWith([]model.Comment{
		{ClientID: "line", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "Line."},
		{ClientID: "file", Subject: "file", Path: "asset.png", Body: "File."},
	}), SubmitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Transport != "graphql-pending" || receipt.CommentsCreated != 2 {
		t.Fatalf("unexpected receipt: %#v", receipt)
	}
	joined := strings.Join(graphBodies, "\n")
	if !strings.Contains(joined, `"subjectType":"LINE"`) || !strings.Contains(joined, `"subjectType":"FILE"`) {
		t.Fatalf("mixed thread subjects missing: %s", joined)
	}
}

func TestSubmitCleansPendingReviewWhenHeadChanges(t *testing.T) {
	pullCalls, cleanupCalls := 0, 0
	client := clientWith(t, testRoundTrip(func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		switch {
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7":
			pullCalls++
			sha := "abc123"
			if pullCalls > 1 {
				sha = "def456"
			}
			return apiResponse(request, 200, `{"number":7,"head":{"sha":"`+sha+`"},"user":{"login":"author"}}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/files":
			return apiResponse(request, 200, `[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"changes":1,"patch":"@@ -1 +1 @@\n-old\n+new"}]`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodGet && path == "/user":
			return apiResponse(request, 200, `{"login":"reviewer"}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/comments":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodPost && path == "/graphql":
			return apiResponse(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `{"id":11,"node_id":"R_11","state":"PENDING"}`), nil
		case request.Method == http.MethodDelete && path == "/repos/o/r/pulls/7/reviews/11":
			cleanupCalls++
			return apiResponse(request, 204, ""), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	}))
	receipt, err := Submit(client, manifestWith([]model.Comment{{
		ClientID: "line", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "Line.",
	}}), SubmitOptions{})
	if err == nil {
		t.Fatal("expected head change")
	}
	var cliErr *output.CLIError
	if !strings.Contains(err.Error(), "head changed") || cleanupCalls != 1 || receipt == nil || !receipt.CleanupSucceeded {
		t.Fatalf("unexpected cleanup outcome: receipt=%#v error=%v cli=%#v", receipt, err, cliErr)
	}
}

func TestSubmitReturnsRecoveryReceiptWhenCleanupFails(t *testing.T) {
	pullCalls := 0
	client := clientWith(t, testRoundTrip(func(request *http.Request) (*http.Response, error) {
		path := request.URL.Path
		switch {
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7":
			pullCalls++
			sha := "abc123"
			if pullCalls > 1 {
				sha = "changed"
			}
			return apiResponse(request, 200, `{"number":7,"head":{"sha":"`+sha+`"},"user":{"login":"author"}}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/files":
			return apiResponse(request, 200, `[{"filename":"a.go","status":"modified","additions":1,"deletions":0,"changes":1,"patch":"@@ -1 +1 @@\n-old\n+new"}]`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodGet && path == "/user":
			return apiResponse(request, 200, `{"login":"reviewer"}`), nil
		case request.Method == http.MethodGet && path == "/repos/o/r/pulls/7/comments":
			return apiResponse(request, 200, `[]`), nil
		case request.Method == http.MethodPost && path == "/graphql":
			return apiResponse(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[]}}}}}`), nil
		case request.Method == http.MethodPost && path == "/repos/o/r/pulls/7/reviews":
			return apiResponse(request, 200, `{"id":12,"node_id":"R_12","state":"PENDING"}`), nil
		case request.Method == http.MethodDelete && path == "/repos/o/r/pulls/7/reviews/12":
			return apiResponse(request, 500, `{"message":"cleanup failed"}`), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	}))
	receipt, err := Submit(client, manifestWith([]model.Comment{{
		ClientID: "line", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "Line.",
	}}), SubmitOptions{})
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitPartial || receipt == nil || !receipt.CleanupAttempted || receipt.CleanupSucceeded {
		t.Fatalf("expected partial recovery receipt, got receipt=%#v error=%#v", receipt, err)
	}
}

func TestSubmitRequiresDecisionConfirmation(t *testing.T) {
	manifest := manifestWith(nil)
	manifest.Event = model.EventApprove
	manifest.Summary = "Approved."
	_, err := Submit(nil, manifest, SubmitOptions{})
	if err == nil || !strings.Contains(err.Error(), "--confirm-decision") {
		t.Fatalf("expected confirmation error, got %v", err)
	}
}

func TestSubmitReconcilesIdempotentReplay(t *testing.T) {
	store := &journal.Store{Dir: t.TempDir()}
	if err := store.Save(&journal.Entry{
		IdempotencyKey: "run-1", Repository: "o/r", PullRequest: 7, HeadSHA: "abc123",
		ReviewID: 20, ReviewNodeID: "R_20", ReviewURL: "https://github.test/review/20",
		State: "submitted", CommentIDs: []string{"line-1"},
	}); err != nil {
		t.Fatal(err)
	}
	client := clientWith(t, testRoundTrip(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/repos/o/r/pulls/7/reviews/20":
			return apiResponse(request, 200, `{"id":20,"state":"COMMENTED","html_url":"https://github.test/review/20"}`), nil
		case "/repos/o/r/pulls/7/comments":
			return apiResponse(request, 200, `[{"id":99,"path":"a.go","line":1,"side":"RIGHT","start_line":0,"start_side":null,"body":"Finding."}]`), nil
		default:
			t.Fatalf("idempotent replay made unexpected request %s %s", request.Method, request.URL.String())
			return nil, nil
		}
	}))
	manifest := manifestWith([]model.Comment{{
		ClientID: "line-1", Subject: "line", Path: "a.go", Line: 1, Side: "RIGHT", Body: "Finding.",
	}})
	manifest.IdempotencyKey = "run-1"
	receipt, err := Submit(client, manifest, SubmitOptions{Journal: store})
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.IdempotentReplay || receipt.Transport != "journal" || receipt.ReviewID != 20 {
		t.Fatalf("unexpected replay receipt: %#v", receipt)
	}
}
