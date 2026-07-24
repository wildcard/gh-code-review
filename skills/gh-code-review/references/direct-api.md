# Direct `gh api` fallback

Prefer `scripts/gh_code_review_fallback.py`; it preserves the same envelope and core transaction:

```bash
python3 scripts/gh_code_review_fallback.py inspect 42 -R owner/repo
python3 scripts/gh_code_review_fallback.py validate --input review.json
python3 scripts/gh_code_review_fallback.py submit --input review.json
```

The fallback requires Python 3.9+ and an authenticated `gh`. It accepts JSON manifests.

## Individual REST operations

Use the PR’s current head SHA and real diff line numbers. `RIGHT` means current/addition; `LEFT` means previous/deletion.

```bash
gh api --method POST repos/owner/repo/pulls/42/comments --input - <<'JSON'
{
  "commit_id": "HEAD_SHA",
  "path": "src/service.go",
  "line": 87,
  "side": "RIGHT",
  "body": "This state can be modified concurrently."
}
JSON
```

Whole-file comment:

```bash
gh api --method POST repos/owner/repo/pulls/42/comments --input - <<'JSON'
{
  "commit_id": "HEAD_SHA",
  "path": "src/storage.go",
  "subject_type": "file",
  "body": "This file combines persistence and invalidation responsibilities."
}
JSON
```

Batch line comments into a pending review by omitting `event`:

```bash
gh api --method POST repos/owner/repo/pulls/42/reviews --input review-payload.json
```

Submit the returned review:

```bash
gh api --method POST repos/owner/repo/pulls/42/reviews/REVIEW_ID/events --input - <<'JSON'
{"event":"COMMENT","body":"Focused review."}
JSON
```

Do not silently post file comments as independent comments when the requested product is one coherent review. Mixed line/file reviews require GraphQL pending-review threads; use the fallback script or extension.

## Lifecycle endpoints

- Edit/delete comment: `PATCH|DELETE repos/{owner}/{repo}/pulls/comments/{comment_id}`
- Abandon pending review: `DELETE repos/{owner}/{repo}/pulls/{pr}/reviews/{review_id}`
- Update review summary: `PUT repos/{owner}/{repo}/pulls/{pr}/reviews/{review_id}`
- Submit pending review: `POST repos/{owner}/{repo}/pulls/{pr}/reviews/{review_id}/events`
- Dismiss review: `PUT repos/{owner}/{repo}/pulls/{pr}/reviews/{review_id}/dismissals`

Thread replies, resolution, file subjects inside a pending review, and viewed-file state use GitHub GraphQL mutations. Use `gh code-review` to avoid hand-building their node IDs and input objects.
