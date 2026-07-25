# Demo: core review transaction

This scenario starts with an ignored error in
`testdata/demo-prs/core/request_path.go`. One multiline suggestion restores the guard
and replaces two selected lines with four lines.

## Live evidence

Recorded fixture: [PR #4](https://github.com/wildcard/gh-code-review/pull/4).
The submitted review is
[review 4777655056](https://github.com/wildcard/gh-code-review/pull/4#pullrequestreview-4777655056).
The machine-verifiable receipt is recorded under `core-transaction` in
[`demo/evidence.json`](../../demo/evidence.json).

## Commands used

```bash
gh code-review capabilities
gh code-review inspect 4 \
  -R wildcard/gh-code-review \
  --unresolved

python3 scripts/demo_lab.py render \
  --scenario core-transaction \
  --pull-request 4 \
  --output /tmp/gh-code-review-core.json

gh code-review validate \
  --input /tmp/gh-code-review-core.json

gh code-review submit \
  --input /tmp/gh-code-review-core.json \
  --dry-run

gh code-review submit \
  --input /tmp/gh-code-review-core.json
```

The dry run selected `rest-batch`. Running the final submit a second time with
the same `idempotency_key` returned `transport: journal` and
`idempotent_replay: true`. GitHub still contained exactly one matching review
thread and zero flat PR comments.

## Agent instruction

```text
Use $gh-code-review to inspect the core-transaction demo PR. Confirm the
manifest anchors are still valid, submit it as a COMMENT review, retry the
same manifest once to prove idempotency, and verify that no duplicate thread
or flat PR comment was created.
```
