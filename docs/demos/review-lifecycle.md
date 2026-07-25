# Demo: pending review and thread lifecycle

Use lifecycle commands when a review must be assembled incrementally or when
you need to manage an existing conversation.

## Live evidence

The exact PR, review, thread, and comment IDs are recorded under
`review-lifecycle` in [`demo/evidence.json`](../../demo/evidence.json).

## Build and submit a pending review

```bash
gh code-review pending start PR_NUMBER \
  -R wildcard/gh-code-review

gh code-review pending show PR_NUMBER \
  -R wildcard/gh-code-review

gh code-review comment add PR_NUMBER \
  -R wildcard/gh-code-review \
  --review-id REVIEW_ID \
  --subject file \
  --path demo-prs/lifecycle/service.conf \
  --body "Document whether zero retries disables recovery."

gh code-review comment edit \
  -R wildcard/gh-code-review \
  --comment-node-id COMMENT_NODE_ID \
  --body "Zero retries disables recovery; please document that operational change."

printf 'retry_count=3\n' |
  gh code-review comment suggest PR_NUMBER \
    -R wildcard/gh-code-review \
    --review-id REVIEW_ID \
    --path demo-prs/lifecycle/service.conf \
    --line 2 \
    --side RIGHT \
    --body "Keep the previous recovery budget." \
    --replacement-file /dev/stdin

gh code-review review submit PR_NUMBER \
  -R wildcard/gh-code-review \
  --review-id REVIEW_ID \
  --event COMMENT \
  --body "Focused review of the retry policy."
```

## Manage the submitted review

```bash
gh code-review review list PR_NUMBER -R wildcard/gh-code-review
gh code-review review show PR_NUMBER -R wildcard/gh-code-review \
  --review-id REVIEW_ID
gh code-review review edit PR_NUMBER -R wildcard/gh-code-review \
  --review-id REVIEW_ID \
  --body "Focused review of the retry policy and recovery contract."

gh code-review thread list PR_NUMBER -R wildcard/gh-code-review
gh code-review thread show PR_NUMBER -R wildcard/gh-code-review \
  --thread-id THREAD_NODE_ID
gh code-review thread reply PR_NUMBER -R wildcard/gh-code-review \
  --thread-id THREAD_NODE_ID \
  --body "The live demo confirms this remains actionable."
gh code-review thread resolve --thread-id THREAD_NODE_ID --confirm
gh code-review thread unresolve --thread-id THREAD_NODE_ID --confirm

gh code-review file viewed PR_NUMBER -R wildcard/gh-code-review \
  --path demo-prs/lifecycle/service.conf --confirm
gh code-review file unviewed PR_NUMBER -R wildcard/gh-code-review \
  --path demo-prs/lifecycle/service.conf --confirm
```

## Edit, delete, and abandon safely

Pending comments use the GraphQL node ID returned by `comment add` or
`comment suggest`. Submitted comments may use their numeric database ID.

```bash
gh code-review comment delete \
  -R wildcard/gh-code-review \
  --comment-node-id COMMENT_NODE_ID \
  --confirm

gh code-review pending start PR_NUMBER -R wildcard/gh-code-review
gh code-review pending abandon PR_NUMBER -R wildcard/gh-code-review \
  --review-id SECOND_REVIEW_ID --confirm
```

Deleting comments, resolving threads, abandoning reviews, and changing viewed
state always require an explicit confirmation flag.
