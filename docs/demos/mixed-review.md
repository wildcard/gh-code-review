# Demo: mixed line and whole-file review

This scenario returns a cache with an uninitialized map and leaves the file's
concurrency contract undefined. The review combines:

- a RIGHT-side line suggestion that fixes the constructor;
- a whole-file comment that asks for an explicit concurrency contract.

Because the manifest contains a file subject, the extension creates one
pending review and adds both `LINE` and `FILE` GraphQL threads before
submission.

## Live evidence

Recorded fixture: [PR #5](https://github.com/wildcard/gh-code-review/pull/5).
Both threads belong to
[review 4777656885](https://github.com/wildcard/gh-code-review/pull/5#pullrequestreview-4777656885).
The machine-verifiable receipt is recorded under `mixed-review` in
[`demo/evidence.json`](../../demo/evidence.json).

## Commands used

```bash
gh code-review inspect 5 \
  -R wildcard/gh-code-review \
  --include bodies

python3 scripts/demo_lab.py render \
  --scenario mixed-review \
  --pull-request 5 \
  --output /tmp/gh-code-review-mixed.json

gh code-review validate \
  --input /tmp/gh-code-review-mixed.json

gh code-review submit \
  --input /tmp/gh-code-review-mixed.json \
  --dry-run

gh code-review submit \
  --input /tmp/gh-code-review-mixed.json
```

The dry-run receipt selected `graphql-pending`. The submitted review contains
one `LINE` thread and one true `FILE` thread, with no issue-level PR comment.

## Agent instruction

```text
Use $gh-code-review to inspect the mixed-review demo PR. Post one formal
COMMENT review containing the exact line suggestion and the honest whole-file
finding from the manifest. Verify both threads belong to the same review.
```
