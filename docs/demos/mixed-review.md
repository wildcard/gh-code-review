# Demo: mixed line and whole-file review

This scenario returns a cache with an uninitialized map and leaves the file's
concurrency contract undefined. The review combines:

- a RIGHT-side line suggestion that fixes the constructor;
- a whole-file comment that asks for an explicit concurrency contract.

Because the manifest contains a file subject, the extension creates one
pending review and adds both `LINE` and `FILE` GraphQL threads before
submission.

## Live evidence

The exact PR and review URLs are recorded under `mixed-review` in
[`demo/evidence.json`](../../demo/evidence.json).

## Commands used

```bash
gh code-review inspect PR_NUMBER \
  -R wildcard/gh-code-review \
  --include bodies

python3 scripts/demo_lab.py render \
  --scenario mixed-review \
  --pull-request PR_NUMBER \
  --output /tmp/gh-code-review-mixed.json

gh code-review validate \
  --input /tmp/gh-code-review-mixed.json

gh code-review submit \
  --input /tmp/gh-code-review-mixed.json \
  --dry-run

gh code-review submit \
  --input /tmp/gh-code-review-mixed.json
```

The dry-run receipt must select `graphql-pending`; the submitted review must
contain both subjects and no issue-level PR comment.

## Agent instruction

```text
Use $gh-code-review to inspect the mixed-review demo PR. Post one formal
COMMENT review containing the exact line suggestion and the honest whole-file
finding from the manifest. Verify both threads belong to the same review.
```
