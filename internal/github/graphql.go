package github

import (
	"fmt"
	"strings"
	"time"

	"github.com/wildcard/gh-code-review/internal/model"
	"github.com/wildcard/gh-code-review/internal/output"
)

const threadsQuery = `
query($owner:String!, $name:String!, $number:Int!, $after:String) {
  repository(owner:$owner, name:$name) {
    pullRequest(number:$number) {
      id
      reviewThreads(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          path
          line
          startLine
          diffSide
          isResolved
          isOutdated
          subjectType
          comments(first:100) {
            pageInfo { hasNextPage endCursor }
            nodes {
              id
              databaseId
              body
              url
              author { login }
            }
          }
        }
      }
    }
  }
}`

func (c *Client) ListThreads(repository string, number int) ([]Thread, string, error) {
	owner, name, err := SplitRepository(repository)
	if err != nil {
		return nil, "", err
	}
	threads := []Thread{}
	after := interface{}(nil)
	pullRequestID := ""
	for {
		var response struct {
			Repository struct {
				PullRequest struct {
					ID            string `json:"id"`
					ReviewThreads struct {
						PageInfo struct {
							HasNextPage bool   `json:"hasNextPage"`
							EndCursor   string `json:"endCursor"`
						} `json:"pageInfo"`
						Nodes []struct {
							ID          string `json:"id"`
							Path        string `json:"path"`
							Line        int    `json:"line"`
							StartLine   int    `json:"startLine"`
							DiffSide    string `json:"diffSide"`
							IsResolved  bool   `json:"isResolved"`
							IsOutdated  bool   `json:"isOutdated"`
							SubjectType string `json:"subjectType"`
							Comments    struct {
								PageInfo struct {
									HasNextPage bool   `json:"hasNextPage"`
									EndCursor   string `json:"endCursor"`
								} `json:"pageInfo"`
								Nodes []struct {
									ID         string `json:"id"`
									DatabaseID int64  `json:"databaseId"`
									Body       string `json:"body"`
									URL        string `json:"url"`
									Author     *struct {
										Login string `json:"login"`
									} `json:"author"`
								} `json:"nodes"`
							} `json:"comments"`
						} `json:"nodes"`
					} `json:"reviewThreads"`
				} `json:"pullRequest"`
			} `json:"repository"`
		}
		variables := map[string]interface{}{"owner": owner, "name": name, "number": number, "after": after}
		if err := c.graphQLDo(threadsQuery, variables, &response); err != nil {
			return nil, "", classifyGraphQLError(err)
		}
		pullRequestID = response.Repository.PullRequest.ID
		for _, node := range response.Repository.PullRequest.ReviewThreads.Nodes {
			thread := Thread{
				ID: node.ID, Path: node.Path, Line: node.Line, StartLine: node.StartLine,
				Side: node.DiffSide, IsResolved: node.IsResolved, IsOutdated: node.IsOutdated,
				Subject: strings.ToLower(node.SubjectType),
			}
			for _, comment := range node.Comments.Nodes {
				author := ""
				if comment.Author != nil {
					author = comment.Author.Login
				}
				thread.Comments = append(thread.Comments, ThreadComment{
					ID: comment.ID, DatabaseID: comment.DatabaseID, Body: comment.Body, URL: comment.URL, Author: author,
				})
			}
			if node.Comments.PageInfo.HasNextPage {
				more, err := c.listThreadComments(node.ID, node.Comments.PageInfo.EndCursor)
				if err != nil {
					return nil, "", err
				}
				thread.Comments = append(thread.Comments, more...)
			}
			threads = append(threads, thread)
		}
		pageInfo := response.Repository.PullRequest.ReviewThreads.PageInfo
		if !pageInfo.HasNextPage {
			break
		}
		after = pageInfo.EndCursor
	}
	return threads, pullRequestID, nil
}

func (c *Client) listThreadComments(threadID, after string) ([]ThreadComment, error) {
	query := `
query($id:ID!, $after:String) {
  node(id:$id) {
    ... on PullRequestReviewThread {
      comments(first:100, after:$after) {
        pageInfo { hasNextPage endCursor }
        nodes { id databaseId body url author { login } }
      }
    }
  }
}`
	result := []ThreadComment{}
	cursor := interface{}(after)
	for {
		var response struct {
			Node struct {
				Comments struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []struct {
						ID         string `json:"id"`
						DatabaseID int64  `json:"databaseId"`
						Body       string `json:"body"`
						URL        string `json:"url"`
						Author     *struct {
							Login string `json:"login"`
						} `json:"author"`
					} `json:"nodes"`
				} `json:"comments"`
			} `json:"node"`
		}
		if err := c.graphQLDo(query, map[string]interface{}{"id": threadID, "after": cursor}, &response); err != nil {
			return nil, classifyGraphQLError(err)
		}
		for _, comment := range response.Node.Comments.Nodes {
			author := ""
			if comment.Author != nil {
				author = comment.Author.Login
			}
			result = append(result, ThreadComment{
				ID: comment.ID, DatabaseID: comment.DatabaseID, Body: comment.Body,
				URL: comment.URL, Author: author,
			})
		}
		if !response.Node.Comments.PageInfo.HasNextPage {
			return result, nil
		}
		cursor = response.Node.Comments.PageInfo.EndCursor
	}
}

func (c *Client) AddReviewThread(reviewNodeID string, comment model.Comment) (*Thread, error) {
	input := map[string]interface{}{
		"pullRequestReviewId": reviewNodeID,
		"body":                comment.GitHubBody(),
		"path":                comment.Path,
		"subjectType":         strings.ToUpper(comment.Subject),
	}
	if comment.Subject == "line" {
		input["line"] = comment.Line
		input["side"] = strings.ToUpper(comment.Side)
		if comment.StartLine != 0 {
			input["startLine"] = comment.StartLine
			input["startSide"] = strings.ToUpper(comment.StartSide)
		}
	}
	var response struct {
		AddPullRequestReviewThread struct {
			Thread struct {
				ID          string `json:"id"`
				Path        string `json:"path"`
				Line        int    `json:"line"`
				StartLine   int    `json:"startLine"`
				DiffSide    string `json:"diffSide"`
				IsResolved  bool   `json:"isResolved"`
				IsOutdated  bool   `json:"isOutdated"`
				SubjectType string `json:"subjectType"`
			} `json:"thread"`
		} `json:"addPullRequestReviewThread"`
	}
	query := `mutation($input:AddPullRequestReviewThreadInput!) {
	  addPullRequestReviewThread(input:$input) {
	    thread { id path line startLine diffSide isResolved isOutdated subjectType }
	  }
	}`
	if err := c.graphQLDo(query, map[string]interface{}{"input": input}, &response); err != nil {
		return nil, classifyGraphQLError(err)
	}
	node := response.AddPullRequestReviewThread.Thread
	return &Thread{
		ID: node.ID, Path: node.Path, Line: node.Line, StartLine: node.StartLine,
		Side: node.DiffSide, IsResolved: node.IsResolved, IsOutdated: node.IsOutdated,
		Subject: strings.ToLower(node.SubjectType),
	}, nil
}

func (c *Client) ReplyToThread(threadID, body string) (*ThreadComment, error) {
	var response struct {
		AddPullRequestReviewThreadReply struct {
			Comment struct {
				ID         string `json:"id"`
				DatabaseID int64  `json:"databaseId"`
				Body       string `json:"body"`
				URL        string `json:"url"`
				Author     *struct {
					Login string `json:"login"`
				} `json:"author"`
			} `json:"comment"`
		} `json:"addPullRequestReviewThreadReply"`
	}
	query := `mutation($input:AddPullRequestReviewThreadReplyInput!) {
	  addPullRequestReviewThreadReply(input:$input) {
	    comment { id databaseId body url author { login } }
	  }
	}`
	input := map[string]interface{}{"pullRequestReviewThreadId": threadID, "body": body}
	if err := c.graphQLDo(query, map[string]interface{}{"input": input}, &response); err != nil {
		return nil, classifyGraphQLError(err)
	}
	node := response.AddPullRequestReviewThreadReply.Comment
	author := ""
	if node.Author != nil {
		author = node.Author.Login
	}
	return &ThreadComment{ID: node.ID, DatabaseID: node.DatabaseID, Body: node.Body, URL: node.URL, Author: author}, nil
}

func (c *Client) ResolveThread(threadID string, resolve bool) (*Thread, error) {
	field := "resolveReviewThread"
	inputType := "ResolveReviewThreadInput"
	if !resolve {
		field = "unresolveReviewThread"
		inputType = "UnresolveReviewThreadInput"
	}
	var response map[string]struct {
		Thread struct {
			ID          string `json:"id"`
			Path        string `json:"path"`
			Line        int    `json:"line"`
			StartLine   int    `json:"startLine"`
			DiffSide    string `json:"diffSide"`
			IsResolved  bool   `json:"isResolved"`
			IsOutdated  bool   `json:"isOutdated"`
			SubjectType string `json:"subjectType"`
		} `json:"thread"`
	}
	query := fmt.Sprintf(`mutation($input:%s!) {
	  %s(input:$input) { thread { id path line startLine diffSide isResolved isOutdated subjectType } }
	}`, inputType, field)
	input := map[string]interface{}{"threadId": threadID}
	if err := c.graphQLDo(query, map[string]interface{}{"input": input}, &response); err != nil {
		return nil, classifyGraphQLError(err)
	}
	node := response[field].Thread
	return &Thread{
		ID: node.ID, Path: node.Path, Line: node.Line, StartLine: node.StartLine,
		Side: node.DiffSide, IsResolved: node.IsResolved, IsOutdated: node.IsOutdated,
		Subject: strings.ToLower(node.SubjectType),
	}, nil
}

func (c *Client) MarkFile(pullRequestID, path string, viewed bool) error {
	field := "markFileAsViewed"
	inputType := "MarkFileAsViewedInput"
	if !viewed {
		field = "unmarkFileAsViewed"
		inputType = "UnmarkFileAsViewedInput"
	}
	query := fmt.Sprintf(`mutation($input:%s!) { %s(input:$input) { clientMutationId } }`, inputType, field)
	input := map[string]interface{}{"pullRequestId": pullRequestID, "path": path}
	if err := c.graphQLDo(query, map[string]interface{}{"input": input}, &map[string]interface{}{}); err != nil {
		return classifyGraphQLError(err)
	}
	return nil
}

func (c *Client) graphQLDo(query string, variables map[string]interface{}, response interface{}) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = c.GraphQL.Do(query, variables, response)
		if err == nil {
			return nil
		}
		lower := strings.ToLower(err.Error())
		if !strings.Contains(lower, "secondary rate limit") && !strings.Contains(lower, "rate limit") {
			return err
		}
		if attempt < 2 {
			time.Sleep(time.Duration(attempt+1) * time.Second)
		}
	}
	return err
}

func classifyGraphQLError(err error) error {
	message := err.Error()
	lower := strings.ToLower(message)
	if strings.Contains(lower, "resource not accessible") || strings.Contains(lower, "forbidden") {
		return output.NewError(output.ExitAuth, "AUTHORIZATION", message, false, nil)
	}
	if strings.Contains(lower, "not supported") || strings.Contains(lower, "unknown type") || strings.Contains(lower, "undefined field") {
		return output.NewError(output.ExitUnsupported, "UNSUPPORTED_CAPABILITY", message, false, nil)
	}
	return output.NewError(output.ExitAPI, "GITHUB_GRAPHQL", message, true, nil)
}
