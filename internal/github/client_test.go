package github

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/cli/go-gh/v2/pkg/api"
	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func testClient(t *testing.T, transport http.RoundTripper) *Client {
	t.Helper()
	client, err := NewWithOptions(api.ClientOptions{
		Host: "github.com", AuthToken: "test-token", Transport: transport,
		Headers: map[string]string{"X-GitHub-Api-Version": APIVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func response(request *http.Request, status int, body string, headers map[string]string) *http.Response {
	header := http.Header{"Content-Type": []string{"application/json"}}
	for key, value := range headers {
		header.Set(key, value)
	}
	return &http.Response{
		StatusCode: status, Status: http.StatusText(status), Header: header,
		Body: io.NopCloser(strings.NewReader(body)), Request: request,
	}
}

func TestListReviewsPaginates(t *testing.T) {
	calls := 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		switch request.URL.Query().Get("page") {
		case "2":
			return response(request, 200, `[{"id":2,"state":"COMMENTED"}]`, nil), nil
		default:
			headers := map[string]string{"Link": `<https://api.github.com/repos/o/r/pulls/1/reviews?per_page=100&page=2>; rel="next"`}
			return response(request, 200, `[{"id":1,"state":"APPROVED"}]`, headers), nil
		}
	}))
	reviews, err := client.ListReviews("o/r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(reviews) != 2 || reviews[1].ID != 2 {
		t.Fatalf("pagination failed: calls=%d reviews=%#v", calls, reviews)
	}
}

func TestCreatePendingReviewUsesNormalizedGitHubFieldsOnly(t *testing.T) {
	var payload map[string]interface{}
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(request.Body)
		if err := json.Unmarshal(data, &payload); err != nil {
			t.Fatal(err)
		}
		return response(request, 200, `{"id":9,"node_id":"R_9","state":"PENDING"}`, nil), nil
	}))
	replacement := "return nil"
	comment := model.Comment{
		ClientID: "local", Subject: "line", Path: "a.go", Line: 5, Side: "RIGHT",
		Body: "Return early.", Replacement: &replacement, Severity: "blocking",
		RuleID: "local-rule", Evidence: []string{"test"},
	}
	review, err := client.CreatePendingReview("o/r", 1, "head", []model.APIComment{comment.APIComment()})
	if err != nil {
		t.Fatal(err)
	}
	if review.ID != 9 {
		t.Fatalf("wrong review: %#v", review)
	}
	encoded, _ := json.Marshal(payload)
	for _, forbidden := range []string{"client_id", "severity", "rule_id", "evidence"} {
		if bytes.Contains(encoded, []byte(forbidden)) {
			t.Fatalf("local-only field %q leaked into payload: %s", forbidden, encoded)
		}
	}
	if !bytes.Contains(encoded, []byte("```suggestion")) {
		t.Fatalf("missing suggestion markdown: %s", encoded)
	}
}

func TestAddFileReviewThreadUsesGraphQLFileSubject(t *testing.T) {
	var requestBody string
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(request.Body)
		requestBody = string(data)
		return response(request, 200, `{"data":{"addPullRequestReviewThread":{"thread":{"id":"T_1","path":"a.go","subjectType":"FILE","isResolved":false,"isOutdated":false,"comments":{"nodes":[{"id":"C_1","databaseId":7,"body":"Whole-file design note.","url":"https://x/7","author":{"login":"u"}}]}}}}}`, nil), nil
	}))
	thread, err := client.AddReviewThread("R_1", model.Comment{
		ClientID: "file", Subject: "file", Path: "a.go", Body: "Whole-file design note.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if thread.Subject != "file" || thread.ID != "T_1" || len(thread.Comments) != 1 || thread.Comments[0].ID != "C_1" {
		t.Fatalf("unexpected thread: %#v", thread)
	}
	if !strings.Contains(requestBody, `"subjectType":"FILE"`) || strings.Contains(requestBody, `"line"`) {
		t.Fatalf("wrong GraphQL variables: %s", requestBody)
	}
}

func TestClassifyRateLimit(t *testing.T) {
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return response(request, 429, `{"message":"secondary rate limit"}`, map[string]string{"Retry-After": "0"}), nil
	}))
	if _, err := client.GetPull("o/r", 1); err == nil || !strings.Contains(err.Error(), "secondary rate limit") {
		t.Fatalf("expected classified error, got %v", err)
	}
}

func TestRetriesSecondaryRateLimit(t *testing.T) {
	calls := 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		calls++
		if calls == 1 {
			return response(request, 429, `{"message":"secondary rate limit"}`, map[string]string{"Retry-After": "0"}), nil
		}
		return response(request, 200, `{"number":1,"head":{"sha":"h"}}`, nil), nil
	}))
	pull, err := client.GetPull("o/r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || pull.Head.SHA != "h" {
		t.Fatalf("retry did not recover: calls=%d pull=%#v", calls, pull)
	}
}

func TestClassifiesPermissionFailure(t *testing.T) {
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return response(request, 403, `{"message":"Resource not accessible by integration"}`, nil), nil
	}))
	_, err := client.GetPull("o/r", 1)
	if err == nil {
		t.Fatal("expected permission error")
	}
	var cliErr *output.CLIError
	if !errors.As(err, &cliErr) || cliErr.ExitCode != output.ExitAuth || cliErr.Code != "AUTHORIZATION" {
		t.Fatalf("wrong permission classification: %#v", err)
	}
}
