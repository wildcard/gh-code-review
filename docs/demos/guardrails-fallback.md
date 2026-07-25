# Demo: guardrails and the `gh api` fallback

This scenario proves that failure paths stop safely before a review is posted.

## Live evidence

Recorded fixture: [PR #7](https://github.com/wildcard/gh-code-review/pull/7).
The old head was `bcda6a709768c955c54218ac96616a9fa78dbc7e`; the advanced
head was `1cf064373b9a74487911efafa56dbebdc5adcce1`. The fallback
created
[review 4777666383](https://github.com/wildcard/gh-code-review/pull/7#pullrequestreview-4777666383).
The expected error envelopes and review receipt are recorded under
`guardrails-fallback` in
[`demo/evidence.json`](../../demo/evidence.json).

## Stale-head protection

Render the manifest against the first head:

```bash
python3 scripts/demo_lab.py render \
  --scenario guardrails-fallback \
  --pull-request 7 \
  --output /tmp/gh-code-review-stale.json
```

Advance the fixture PR:

```bash
python3 scripts/demo_lab.py advance \
  --scenario guardrails-fallback \
  --confirm
```

The old manifest now fails read-only validation:

```bash
gh code-review validate --input /tmp/gh-code-review-stale.json
# exit 3, error.code = HEAD_CHANGED
```

The extension never silently retargets a comment to a nearby line.

## Self-approval protection

```bash
python3 scripts/demo_lab.py render \
  --scenario guardrails-fallback \
  --template self-approval \
  --pull-request 7 \
  --output /tmp/gh-code-review-self-approval.json

gh code-review validate --input /tmp/gh-code-review-self-approval.json
# exit 2, error.code = SELF_APPROVAL
```

`APPROVE` and `REQUEST_CHANGES` also require `--confirm-decision`. A public
single-account demo can prove the self-approval guard, but cannot honestly
manufacture a successful approval by a second reviewer.

## Dismissal safety

Review dismissal is only valid for dismissible review states and creates a
visible timeline explanation. The command therefore requires
`--confirm-timeline-comment`. The live lab records GitHub's rejection when a
`COMMENTED` review is not dismissible; mocks cover the successful API shape.

```bash
gh code-review review dismiss 7 \
  -R wildcard/gh-code-review \
  --review-id 4777666383 \
  --message "Disposable dismissal-safety check." \
  --confirm-timeline-comment
# exit 8, error.code = GITHUB_CONFLICT
```

## Standard-library fallback

Render a fresh manifest, then run the same core transaction without installing
the extension:

```bash
python3 scripts/demo_lab.py render \
  --scenario guardrails-fallback \
  --pull-request 7 \
  --output /tmp/gh-code-review-fallback.json

python3 skills/gh-code-review/scripts/gh_code_review_fallback.py \
  inspect 7 -R wildcard/gh-code-review

python3 skills/gh-code-review/scripts/gh_code_review_fallback.py \
  validate --input /tmp/gh-code-review-fallback.json

python3 skills/gh-code-review/scripts/gh_code_review_fallback.py \
  submit --input /tmp/gh-code-review-fallback.json
```

The fallback uses authenticated `gh api`, creates a formal review, and emits
the same versioned success/error envelope. Lifecycle commands remain an
extension feature.
