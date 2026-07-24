# Contributing

Issues and pull requests are welcome.

## Development checks

```bash
go test ./...
go test -race ./...
go vet ./...
python3 -m unittest discover -s tests -p 'test_*.py'
gh skill publish --dry-run
claude plugin validate . --strict
```

Run `gofmt` on Go changes. Keep Python fallback code compatible with Python 3.9 and the standard library.

## Review transaction changes

Changes to validation or submission must include fixtures for both the Go extension and Python fallback when the core `inspect`, `validate`, or `submit` contract changes.

Cover the relevant boundary:

- RIGHT, LEFT, and context lines;
- multiline ranges and cross-hunk rejection;
- different-size suggestions;
- file subjects and mixed reviews;
- renamed, deleted, binary, and truncated patches;
- changed heads, duplicate threads, and pending-review conflicts;
- approval, comment, and changes-requested decisions;
- partial submission and cleanup recovery.

Do not record live tokens, private repositories, or real review content in fixtures.

## Release

The extension, Agent Skill, and Claude plugin share one semantic version. Update `CHANGELOG.md`, `.claude-plugin/plugin.json`, and `.claude-plugin/marketplace.json` together.

`gh skill publish --dry-run` is validation only; the tagged GitHub release belongs to the precompiled extension and contains its platform binaries, checksums, attestations, and SBOM.
