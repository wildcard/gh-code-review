# Agent-guided reviews

The extension does not review code. The portable skill gives an agent the
review workflow; the extension validates and transports its findings.

## Codex

Install:

```bash
gh extension install wildcard/gh-code-review
gh skill install wildcard/gh-code-review gh-code-review \
  --agent codex --scope user
```

Prompt:

```text
Use $gh-code-review to inspect PR_URL, review only the changed code, and post
one formal COMMENT review with the smallest honest anchors. Do not use a flat
PR comment. Verify the resulting review and report its URL.
```

The exact public PR, review URL, prompt, and sanitized agent result are
recorded under `codex-skill` in
[`demo/evidence.json`](../../demo/evidence.json).

## Claude Code

Install:

```bash
claude plugin marketplace add wildcard/gh-code-review
claude plugin install gh-code-review@gh-code-review
```

Prompt:

```text
Use the gh-code-review skill to inspect PR_URL, review only the changed code,
and post one formal COMMENT review with the smallest honest anchors. Do not
use a flat PR comment. Verify the resulting review and report its URL.
```

The exact public PR, review URL, prompt, model, and sanitized result are
recorded under `claude-plugin` in
[`demo/evidence.json`](../../demo/evidence.json).

## Cursor, Copilot CLI, Gemini CLI, and other skill clients

Install the same canonical skill with the client identifier supported by your
`gh skill` version:

```bash
gh skill install wildcard/gh-code-review gh-code-review \
  --agent cursor --scope user
```

Use the Codex prompt above. If the client does not support `$skill-name`
syntax, say `Use the gh-code-review skill`.

## What to look for

An agent run is successful only when:

- it inspects existing review threads before posting;
- each finding uses the smallest honest line, range, or file anchor;
- exact corrections use GitHub suggestion blocks;
- the final decision is `COMMENT` unless another decision was explicitly
  authorized;
- the result is one formal review, not `gh pr comment`;
- a final inspection confirms the expected review and no duplicates.
