# Review manifest v1

JSON is canonical. The Go extension also accepts YAML; the Python fallback accepts JSON.

```json
{
  "schema_version": "1.0",
  "repository": "owner/repo",
  "pull_request": 42,
  "expected_head_sha": "0123456789abcdef",
  "event": "COMMENT",
  "summary": "Focused review of the changed request path.",
  "idempotency_key": "review-42-0123456789abcdef",
  "comments": [
    {
      "client_id": "nil-result-1",
      "subject": "line",
      "path": "internal/service.go",
      "line": 112,
      "side": "RIGHT",
      "body": "The result is dereferenced when the call returns an error.",
      "replacement": "if err != nil {\n    return nil, err\n}",
      "severity": "blocking",
      "category": "correctness",
      "confidence": 0.96,
      "rule_id": "guard-before-use",
      "evidence": ["The error path leaves result nil."]
    },
    {
      "client_id": "storage-boundary-1",
      "subject": "file",
      "path": "internal/storage.go",
      "body": "This file now owns both persistence and cache invalidation; consider keeping the invalidation policy behind the storage interface.",
      "severity": "non_blocking",
      "category": "maintainability"
    }
  ]
}
```

## Required fields

- `schema_version`: `1.0`
- `repository`: `owner/name`
- `pull_request`: positive integer
- `expected_head_sha`: exact inspected PR head
- `event`: `COMMENT`, `APPROVE`, or `REQUEST_CHANGES`; omit only with `--derive-event`
- `comments[].client_id`: unique stable local identifier
- `comments[].subject`: `line` or `file`
- `comments[].path`: current repository-relative changed path
- `comments[].body`: concise actionable finding; may be empty only when `replacement` exists

## Location fields

Line subjects require `line` and `side`. Use `RIGHT` for added/current lines and `LEFT` for deleted/previous lines. A range adds `start_line` and `start_side`; both ends must use the same side and diff hunk.

File subjects contain no line or side fields.

`replacement` generates GitHub suggestion Markdown. It is valid only for RIGHT-side line/range subjects and may contain any number of replacement lines.

## Local-only fields

`severity`, `category`, `confidence`, `rule_id`, and `evidence` are used for validation, policy, and receipts. They are never sent to GitHub.

With `--derive-event`, the transparent default policy is:

- blocking finding: `REQUEST_CHANGES`
- non-blocking findings only: `COMMENT`
- no findings: `APPROVE`
