# Changelog

All notable changes are documented here.

## [0.2.0] - 2026-07-24

- Add an executable public review lab with six sanitized PR fixtures, exact
  commands, durable receipts, and live/offline evidence verification.
- Add agent-specific Codex and Claude Code walkthroughs backed by formal
  `COMMENT` reviews and zero flat PR comments.
- Accept JSON or YAML manifests from standard input with `--input -`.
- Keep fallback `inspect` output compact and aligned with the Go extension.

## [0.1.1] - 2026-07-24

- Edit and delete GraphQL-created pending review comments by node ID.
- Return the created comment identifiers directly from `comment add` and
  `comment suggest`.
- Preserve numeric REST comment IDs for submitted-review compatibility.

## [0.1.0] - 2026-07-24

Initial public preview.

- Declarative JSON/YAML review manifests and stable machine-readable envelopes.
- Diff-aware inspection and validation.
- Atomic pending-review submission for line, range, file, and suggestion threads.
- Idempotency journal, live reconciliation, head protection, and cleanup receipts.
- Complete comment, thread, review, pending-review, and viewed-file lifecycle.
- Portable Agent Skill, Claude plugin/marketplace, and Python `gh api` fallback.
