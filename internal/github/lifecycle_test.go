package github

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/wildcard/gh-code-review/internal/model"
)

func TestRESTLifecyclePayloads(t *testing.T) {
	var seen []string
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		var data []byte
		if request.Body != nil {
			data, _ = io.ReadAll(request.Body)
		}
		seen = append(seen, request.Method+" "+request.URL.Path+" "+string(data))
		switch {
		case request.Method == http.MethodPatch:
			return response(request, 200, `{"id":3,"body":"edited"}`, nil), nil
		case request.Method == http.MethodDelete:
			return response(request, 204, "", nil), nil
		case strings.HasSuffix(request.URL.Path, "/dismissals"):
			return response(request, 200, `{"id":4,"state":"DISMISSED"}`, nil), nil
		case strings.HasSuffix(request.URL.Path, "/events"):
			return response(request, 200, `{"id":4,"state":"COMMENTED"}`, nil), nil
		case request.Method == http.MethodPut:
			return response(request, 200, `{"id":4,"body":"updated"}`, nil), nil
		default:
			return response(request, 200, `{"id":3,"body":"reply"}`, nil), nil
		}
	}))
	if _, err := client.EditReviewComment("o/r", 3, "edited"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteReviewComment("o/r", 3); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReplyToComment("o/r", 1, 3, "reply"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.UpdateReview("o/r", 1, 4, "updated"); err != nil {
		t.Fatal(err)
	}
	for _, event := range []model.Event{model.EventComment, model.EventApprove, model.EventRequestChanges} {
		if _, err := client.SubmitReview("o/r", 1, 4, event, "summary"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := client.DismissReview("o/r", 1, 4, "obsolete"); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(seen, "\n")
	for _, expected := range []string{
		`"in_reply_to":3`,
		`"event":"COMMENT"`,
		`"event":"APPROVE"`,
		`"event":"REQUEST_CHANGES"`,
		`"message":"obsolete"`,
	} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %s in requests:\n%s", expected, joined)
		}
	}
}

func TestGraphQLThreadAndViewedLifecycle(t *testing.T) {
	var fields []string
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(request.Body)
		var payload struct {
			Query string `json:"query"`
		}
		_ = json.Unmarshal(data, &payload)
		switch {
		case strings.Contains(payload.Query, "updatePullRequestReviewComment"):
			if !strings.Contains(string(data), `"pullRequestReviewCommentId":"C_1"`) {
				t.Fatalf("missing update comment node ID: %s", data)
			}
			fields = append(fields, "edit")
			return response(request, 200, `{"data":{"updatePullRequestReviewComment":{"pullRequestReviewComment":{"id":"C_1","databaseId":1,"body":"edited","url":"https://x","author":{"login":"u"}}}}}`, nil), nil
		case strings.Contains(payload.Query, "deletePullRequestReviewComment"):
			if !strings.Contains(string(data), `"id":"C_2"`) || strings.Contains(string(data), `"pullRequestReviewCommentId":"C_2"`) {
				t.Fatalf("wrong delete comment input: %s", data)
			}
			fields = append(fields, "delete")
			return response(request, 200, `{"data":{"deletePullRequestReviewComment":{"pullRequestReviewComment":{"id":"C_2"}}}}`, nil), nil
		case strings.Contains(payload.Query, "addPullRequestReviewThreadReply"):
			fields = append(fields, "reply")
			return response(request, 200, `{"data":{"addPullRequestReviewThreadReply":{"comment":{"id":"C_1","databaseId":1,"body":"reply","url":"https://x","author":{"login":"u"}}}}}`, nil), nil
		case strings.Contains(payload.Query, "unresolveReviewThread"):
			fields = append(fields, "unresolve")
			return response(request, 200, `{"data":{"unresolveReviewThread":{"thread":{"id":"T_1","path":"a.go","isResolved":false,"subjectType":"LINE"}}}}`, nil), nil
		case strings.Contains(payload.Query, "resolveReviewThread"):
			fields = append(fields, "resolve")
			return response(request, 200, `{"data":{"resolveReviewThread":{"thread":{"id":"T_1","path":"a.go","isResolved":true,"subjectType":"LINE"}}}}`, nil), nil
		case strings.Contains(payload.Query, "unmarkFileAsViewed"):
			fields = append(fields, "unviewed")
			return response(request, 200, `{"data":{"unmarkFileAsViewed":{"clientMutationId":null}}}`, nil), nil
		case strings.Contains(payload.Query, "markFileAsViewed"):
			fields = append(fields, "viewed")
			return response(request, 200, `{"data":{"markFileAsViewed":{"clientMutationId":null}}}`, nil), nil
		default:
			t.Fatalf("unexpected GraphQL query: %s", payload.Query)
			return nil, nil
		}
	}))
	if comment, err := client.EditReviewCommentNode("C_1", "edited"); err != nil || comment.Body != "edited" {
		t.Fatalf("edit: %#v %v", comment, err)
	}
	if err := client.DeleteReviewCommentNode("C_2"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.ReplyToThread("T_1", "reply"); err != nil {
		t.Fatal(err)
	}
	if thread, err := client.ResolveThread("T_1", true); err != nil || !thread.IsResolved {
		t.Fatalf("resolve: %#v %v", thread, err)
	}
	if thread, err := client.ResolveThread("T_1", false); err != nil || thread.IsResolved {
		t.Fatalf("unresolve: %#v %v", thread, err)
	}
	if err := client.MarkFile("PR_1", "a.go", true); err != nil {
		t.Fatal(err)
	}
	if err := client.MarkFile("PR_1", "a.go", false); err != nil {
		t.Fatal(err)
	}
	if strings.Join(fields, ",") != "edit,delete,reply,resolve,unresolve,viewed,unviewed" {
		t.Fatalf("unexpected operations: %#v", fields)
	}
}

func TestListThreadsPaginatesThreadsAndReplies(t *testing.T) {
	threadPage, commentPage := 0, 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, _ := io.ReadAll(request.Body)
		body := string(data)
		if strings.Contains(body, "... on PullRequestReviewThread") {
			commentPage++
			return response(request, 200, `{"data":{"node":{"comments":{"pageInfo":{"hasNextPage":false},"nodes":[{"id":"C_2","databaseId":2,"body":"second","url":"https://x/2","author":{"login":"b"}}]}}}}`, nil), nil
		}
		threadPage++
		if threadPage == 1 {
			return response(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"T_1","path":"a.go","line":1,"diffSide":"RIGHT","isResolved":false,"isOutdated":false,"subjectType":"LINE","comments":{"pageInfo":{"hasNextPage":true,"endCursor":"comments-next"},"nodes":[{"id":"C_1","databaseId":1,"body":"first","url":"https://x/1","author":{"login":"a"}}]}}]}}}}}`, nil), nil
		}
		return response(request, 200, `{"data":{"repository":{"pullRequest":{"id":"PR_1","reviewThreads":{"pageInfo":{"hasNextPage":false},"nodes":[{"id":"T_2","path":"b.go","isResolved":true,"isOutdated":false,"subjectType":"FILE","comments":{"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}}}}`, nil), nil
	}))
	threads, pullID, err := client.ListThreads("o/r", 1)
	if err != nil {
		t.Fatal(err)
	}
	if pullID != "PR_1" || len(threads) != 2 || len(threads[0].Comments) != 2 || commentPage != 1 || threadPage != 2 {
		t.Fatalf("pagination failed: pull=%s threads=%#v pages=%d/%d", pullID, threads, threadPage, commentPage)
	}
}
