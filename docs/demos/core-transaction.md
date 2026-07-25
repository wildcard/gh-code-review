# Demo: core review transaction

This scenario starts with an ignored error in
`demo-prs/core/request_path.go`. One multiline suggestion restores the guard
and replaces two selected lines with four lines.

## Live evidence

The exact PR and review URLs are recorded under `core-transaction` in
[`demo/evidence.json`](../../demo/evidence.json).

## Commands used

```bash
gh code-review capabilities
gh code-review inspect PR_NUMBER \
  -R wildcard/gh-code-review \
  --unresolved

python3 scripts/demo_lab.py render \
  --scenario core-transaction \
  --pull-request PR_NUMBER \
  --output /tmp/gh-code-review-core.json

gh code-review validate \
  --input /tmp/gh-code-review-core.json

gh code-review submit \
  --input /tmp/gh-code-review-core.json \
  --dry-run

gh code-review submit \
  --input /tmp/gh-code-review-core.json
```

The final submit is run a second time with the same `idempotency_key`. The
receipt must report an idempotent replay and GitHub must still contain exactly
one matching review thread.

## Agent instruction

```text
Use $gh-code-review to inspect the core-transaction demo PR. Confirm the
manifest anchors are still valid, submit it as a COMMENT review, retry the
same manifest once to prove idempotency, and verify that no duplicate thread
or flat PR comment was created.
```
