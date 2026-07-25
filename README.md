# gh-code-review

[![CI](https://github.com/wildcard/gh-code-review/actions/workflows/ci.yml/badge.svg)](https://github.com/wildcard/gh-code-review/actions/workflows/ci.yml)
[![skills.sh](https://skills.sh/b/wildcard/gh-code-review)](https://skills.sh/wildcard/gh-code-review)
[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

`gh-code-review` turns structured findings into the same coherent GitHub review a human reviewer submits: inline and multiline threads, whole-file comments, apply-able suggestions, and one final `COMMENT`, `APPROVE`, or `REQUEST_CHANGES` decision.

It has two deliberately separate layers:

- `gh code-review` is a deterministic GitHub CLI extension for inspection, validation, submission, retry safety, and review-thread lifecycle operations. It does not invoke a model or analyze code.
- The portable `gh-code-review` Agent Skill teaches coding agents how to review changed code, avoid duplicate threads, choose honest anchors, and use the extension or its `gh api` fallback.

This is a public preview. GitHub.com is the supported v0.1 host; GitHub Enterprise Server reports capabilities explicitly.

## Install

### GitHub CLI extension

```bash
gh extension install wildcard/gh-code-review
gh code-review capabilities
```

### Agent Skill

With skills.sh:

```bash
npx skills add wildcard/gh-code-review
```

With GitHub CLI for Codex:

```bash
gh skill install wildcard/gh-code-review gh-code-review --agent codex --scope user
```

Other supported `gh skill` agents include Claude Code, Cursor, Copilot, Gemini CLI, and more:

```bash
gh skill install wildcard/gh-code-review gh-code-review --agent claude-code --scope user
```

### Claude plugin marketplace

```bash
claude plugin marketplace add wildcard/gh-code-review
claude plugin install gh-code-review@gh-code-review
```

Restart or reload Claude Code after installation.

None of these installations happen automatically. The skill can use its bundled standard-library Python fallback when the extension is unavailable.

## User guide and live review lab

Follow [the user guide](docs/user-guide.md) for exact commands, portable agent
prompts, and sanitized public PRs covering core submission, mixed line/file
reviews, pending-review lifecycle operations, guardrails, the Python fallback,
Codex, and Claude Code.

The guide's evidence is machine-readable and read-only verifiable:

```bash
python3 scripts/demo_lab.py verify --live
```

## Quick start

Inspect the PR and existing review threads:

```bash
gh code-review inspect 42 -R owner/repo --unresolved
```

Create a manifest using the exact `head.sha` and commentable ranges returned by `inspect`:

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
      "replacement": "if err != nil {\n    return nil, err\n}"
    },
    {
      "client_id": "storage-boundary-1",
      "subject": "file",
      "path": "internal/storage.go",
      "body": "This file now owns both persistence and cache invalidation."
    }
  ]
}
```

Validate without writing:

```bash
gh code-review validate --input review.json
```

Submit one formal review:

```bash
gh code-review submit --input review.json
```

`APPROVE` and `REQUEST_CHANGES` require `--confirm-decision`. A stale PR head fails with exit code 3 before submission.

## Commands

```text
gh code-review inspect
gh code-review validate
gh code-review submit
gh code-review capabilities

gh code-review pending start|show|abandon
gh code-review comment add|suggest|edit|delete
gh code-review thread list|show|reply|resolve|unresolve
gh code-review review list|show|edit|submit|dismiss
gh code-review file viewed|unviewed
```

All commands return a versioned JSON envelope. See [the CLI reference](skills/gh-code-review/references/cli-reference.md) and [review schema](schema/v1/review.schema.json).

## Why a manifest?

The manifest makes a review inspectable before it is written. Validation catches:

- stale head SHAs;
- paths and lines outside the diff;
- wrong LEFT/RIGHT sides and cross-hunk ranges;
- renamed, deleted, binary, and unavailable-patch limitations;
- suggestions on deleted lines or file subjects;
- duplicate findings and existing equivalent comments;
- pending-review conflicts and self-approval;
- unsupported host capabilities.

Line-only reviews use one REST pending-review batch. Reviews containing file subjects use GraphQL threads inside one pending review. The head is checked again before final submission. If a pre-submit step fails, the extension attempts to delete the pending review and returns a recovery receipt if cleanup also fails.

Agent-only metadata (`severity`, `category`, `confidence`, `rule_id`, and `evidence`) is not posted to GitHub. The extension does not insert hidden provenance markers.

## Direct API fallback

Users who do not install the extension can run:

```bash
python3 skills/gh-code-review/scripts/gh_code_review_fallback.py inspect 42 -R owner/repo
python3 skills/gh-code-review/scripts/gh_code_review_fallback.py validate --input review.json
python3 skills/gh-code-review/scripts/gh_code_review_fallback.py submit --input review.json
```

The fallback uses only Python’s standard library and the authenticated `gh api` command. It accepts JSON manifests. Advanced raw API recipes are documented in [direct-api.md](skills/gh-code-review/references/direct-api.md).

## Output contract

Success:

```json
{
  "schema_version": "1.0",
  "ok": true,
  "operation": "submit",
  "repository": "owner/repo",
  "pull_request": 42,
  "result": {}
}
```

Failure:

```json
{
  "schema_version": "1.0",
  "ok": false,
  "operation": "validate",
  "error": {
    "code": "HEAD_CHANGED",
    "message": "pull request head changed",
    "retryable": true,
    "details": {}
  }
}
```

Stable exit codes distinguish validation, head changes, authorization, API failure, partial transactions, unsupported capabilities, and conflicts.

## Development

Requirements: Go 1.26.5+, Python 3.9+, GitHub CLI, and Claude Code only for plugin validation.

```bash
go test ./...
go test -race ./...
go vet ./...
python3 -m unittest discover -s tests -p 'test_*.py'
gh skill publish --dry-run
claude plugin validate . --strict
```

Install the extension from a local checkout:

```bash
go build -o gh-code-review .
gh extension install .
```

See [CONTRIBUTING.md](CONTRIBUTING.md) for fixture and release guidance.

## Security and privacy

The extension uses the authentication already managed by `gh`. It never reads or prints tokens. Local idempotency journals are stored in the operating system user cache with restrictive permissions.

Review bodies are public to everyone who can view the target pull request. Do not include credentials, personal data, hidden reasoning, or private evidence. Report security issues using [SECURITY.md](SECURITY.md).

## License

[MIT](LICENSE)
