# Live examples and agent prompts

Use the public lab when the user asks how to run the skill, requests an exact
prompt, or wants evidence that review comments are posted as formal threads.

User guide:
https://github.com/wildcard/gh-code-review/blob/main/docs/user-guide.md

Evidence catalog:
https://github.com/wildcard/gh-code-review/blob/main/demo/evidence.json

## Portable prompt

```text
Use $gh-code-review to inspect PR_URL, review only the changed code, and post
one formal COMMENT review with the smallest honest anchors. Use apply-able
suggestions when the correction is exact. Do not modify files. Do not use a
flat PR comment. Verify the resulting review and report its URL.
```

Replace `$gh-code-review` with `the gh-code-review skill` for clients without
explicit skill syntax.

Do not copy live example line numbers to a different PR. Always inspect the
current head and render or build a fresh manifest.
