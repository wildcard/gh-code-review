---
name: gh-code-review
description: Review GitHub pull request changes and publish coherent inline, multiline, file-level, and apply-able suggestion comments as one formal review. Use when asked to review a PR, leave code review comments, submit line or file comments, approve, request changes, inspect review threads, reply, resolve discussions, or manage reviewed-file state. Prefer the gh code-review extension; use the bundled gh api fallback when installation is declined or unavailable. Never replace a formal review with a flat gh pr comment.
license: MIT
---

# GitHub Code Review

Use a formal pull-request review so every finding is anchored, individually followable, and submitted with one clear decision. The extension is deterministic transport and validation; you remain responsible for review judgment.

## Choose the transport

1. Run `gh code-review capabilities`.
2. If the command exists, use it.
3. If it is absent, offer `gh extension install wildcard/gh-code-review`. Never install without the user's consent.
4. If installation is declined or unavailable, run `scripts/gh_code_review_fallback.py`. The fallback supports JSON manifests and the core `inspect`, `validate`, and `submit` transaction.
5. Read [references/cli-reference.md](references/cli-reference.md) only for advanced lifecycle operations. Read [references/direct-api.md](references/direct-api.md) when the extension cannot be used.
6. Read [references/live-examples.md](references/live-examples.md) when the user asks for an exact prompt, a walkthrough, or public evidence.

## Review workflow

### 1. Inspect before judging

Resolve the repository and PR, then read normalized review context:

```bash
gh code-review inspect 42 -R owner/repo --unresolved
```

Use `--include bodies` when full comment text is needed and `--include patches` only when local diff access is unavailable. Always inspect existing threads before proposing or posting findings.

### 2. Review the changed code

Confirm each finding against the current diff and surrounding implementation. Avoid duplicating an existing thread or restating a resolved finding without new evidence.

Choose the smallest useful anchor:

- `line`: one changed or context line.
- range: one coherent block within a single diff hunk.
- `file`: a concern about the entire changed file that has no honest line anchor.

Use a `replacement` only when the exact corrected code is known. Suggestions may replace a different number of lines, but they must target `RIGHT`-side line or range subjects. Do not attach suggestions to deleted lines or whole files.

Write findings so a maintainer can act:

- State the concrete failure or risk.
- Explain when it occurs and why it matters.
- Keep unrelated concerns in separate threads.
- Prefer a suggestion block for a small, exact correction.
- Do not expose hidden reasoning, credentials, or local-only evidence.

### 3. Build a manifest

Copy [references/review-manifest.md](references/review-manifest.md) and fill the current head SHA returned by `inspect`. Use unique, stable `client_id` values.

The default event for unspecified review feedback is `COMMENT`. Use `APPROVE` or `REQUEST_CHANGES` only when the user explicitly authorizes that decision. Do not infer approval from “looks good,” and do not infer a changes request from the presence of findings.

Agent metadata such as severity, category, confidence, rule ID, and evidence remains local; the transport never posts it.

### 4. Validate before any write

```bash
gh code-review validate --input review.json
```

Validation must pass against the live PR head, diff locations, permissions, existing comments, and pending-review state. Treat `HEAD_CHANGED` as a required re-inspection. Never move a finding to a nearby valid line merely to satisfy the API.

If event derivation was explicitly requested, preview it:

```bash
gh code-review validate --input review.json --derive-event
```

Derived decisions are visible before submission and still require `--confirm-decision` for `APPROVE` or `REQUEST_CHANGES`.

### 5. Submit one coherent review

Only submit when the user asked to post or submit the review:

```bash
gh code-review submit --input review.json
```

For an explicitly authorized approval or changes request:

```bash
gh code-review submit --input review.json --confirm-decision
```

Do not use `gh pr comment` as a fallback. If submission is partially applied, report the recovery receipt and pending review ID exactly; do not blindly retry.

### 6. Verify

Inspect again and confirm:

- the review state matches the requested event;
- each expected thread exists and is not outdated;
- apply-able suggestions render correctly;
- no duplicate or flat PR comment was created.

Report the review URL, decision, number of threads, and any remaining limitation.

## Safety rules

- Reading a PR does not authorize a write. “Post,” “submit,” “leave comments,” “approve,” or “request changes” does.
- Default an unspecified posting decision to `COMMENT`.
- Require explicit user authorization for `APPROVE` and `REQUEST_CHANGES`.
- Never install the extension silently.
- Never abandon a pending review, delete a comment, resolve a thread, dismiss a review, or change viewed state without explicit confirmation.
- Review dismissal creates a visible timeline explanation; warn and require confirmation.
- Never write secrets or hidden provenance markers into comment bodies.
