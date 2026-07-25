# Demo: pending review and thread lifecycle

Use lifecycle commands when a review must be assembled incrementally or when
you need to manage an existing conversation.

## Live evidence

Recorded fixture: [PR #6](https://github.com/wildcard/gh-code-review/pull/6).
The assembled review is
[review 4777659246](https://github.com/wildcard/gh-code-review/pull/6#pullrequestreview-4777659246).
The exact thread and comment IDs are recorded under `review-lifecycle` in
[`demo/evidence.json`](../../demo/evidence.json).

## Build and submit a pending review

```bash
gh code-review pending start 6 \
  -R wildcard/gh-code-review

gh code-review pending show 6 \
  -R wildcard/gh-code-review

gh code-review comment add 6 \
  -R wildcard/gh-code-review \
  --review-id 4777659246 \
  --subject file \
  --path demo-prs/lifecycle/service.conf \
  --body "Document whether zero retries disables recovery."

gh code-review comment edit \
  -R wildcard/gh-code-review \
  --comment-node-id PRRC_kwDOTiGpAs7ZfJ8c \
  --body "Zero retries disables recovery; please document that operational change."

printf 'retry_count=3\n' |
  gh code-review comment suggest 6 \
    -R wildcard/gh-code-review \
    --review-id 4777659246 \
    --path demo-prs/lifecycle/service.conf \
    --line 2 \
    --side RIGHT \
    --body "Keep the previous recovery budget." \
    --replacement-file /dev/stdin

gh code-review comment delete \
  -R wildcard/gh-code-review \
  --comment-node-id PRRC_kwDOTiGpAs7ZfKD- \
  --confirm

gh code-review comment add 6 \
  -R wildcard/gh-code-review \
  --review-id 4777659246 \
  --subject line \
  --path demo-prs/lifecycle/service.conf \
  --line 2 \
  --side RIGHT \
  --body "A zero retry budget removes transient-failure recovery; retain a bounded retry path."

gh code-review review submit 6 \
  -R wildcard/gh-code-review \
  --review-id 4777659246 \
  --event COMMENT \
  --body "Focused review of the retry policy."
```

The first `comment add` returned node ID
`PRRC_kwDOTiGpAs7ZfJ8c`; use it in the edit command above. The suggestion was
created only to exercise pending-node deletion, then removed before the final
line finding was added.

## Manage the submitted review

```bash
gh code-review review list 6 -R wildcard/gh-code-review
gh code-review review show 6 -R wildcard/gh-code-review \
  --review-id 4777659246
gh code-review review edit 6 -R wildcard/gh-code-review \
  --review-id 4777659246 \
  --body "Focused review of the retry policy and recovery contract."

gh code-review comment edit \
  -R wildcard/gh-code-review \
  --comment-id 3648824217 \
  --body "A zero retry budget removes transient-failure recovery; retain a bounded retry path."

gh code-review thread list 6 -R wildcard/gh-code-review
gh code-review thread show 6 -R wildcard/gh-code-review \
  --thread-id PRRT_kwDOTiGpAs6Tseh-
gh code-review thread reply 6 -R wildcard/gh-code-review \
  --thread-id PRRT_kwDOTiGpAs6Tseh- \
  --body "The live demo confirms this remains actionable."
gh code-review thread resolve --thread-id PRRT_kwDOTiGpAs6Tseh- --confirm
gh code-review thread unresolve --thread-id PRRT_kwDOTiGpAs6Tseh- --confirm

gh code-review file viewed 6 -R wildcard/gh-code-review \
  --path demo-prs/lifecycle/service.conf --confirm
gh code-review file unviewed 6 -R wildcard/gh-code-review \
  --path demo-prs/lifecycle/service.conf --confirm
```

## Edit, delete, and abandon safely

Pending comments use the GraphQL node ID returned by `comment add` or
`comment suggest`. Submitted comments may use their numeric database ID.

```bash
gh code-review comment delete \
  -R wildcard/gh-code-review \
  --comment-id 3648824344 \
  --confirm

gh code-review pending start 6 -R wildcard/gh-code-review
gh code-review pending abandon 6 -R wildcard/gh-code-review \
  --review-id 4777662709 --confirm
```

Deleting comments, resolving threads, abandoning reviews, and changing viewed
state always require an explicit confirmation flag.
