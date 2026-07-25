# CLI reference

All commands emit a JSON envelope with `schema_version`, `ok`, `operation`, repository/PR context, and either `result` or `error`.

## Core transaction

```text
gh code-review inspect <pr> -R owner/repo [--unresolved] [--include patches,bodies]
gh code-review validate --input review.json [--derive-event] [--resume-review ID]
gh code-review submit --input review.json [--dry-run] [--derive-event]
  [--confirm-decision] [--resume-review ID]
gh code-review capabilities
```

Use `--input -` with `validate` or `submit` to read a JSON or YAML manifest
from standard input. This lets an agent stream a generated manifest without an
intermediate file.

## Pending reviews

```text
gh code-review pending start <pr> -R owner/repo
gh code-review pending show <pr> -R owner/repo
gh code-review pending abandon <pr> -R owner/repo --review-id ID --confirm
```

## Comments

`add` and `suggest` append a thread to an explicit pending review.

```text
gh code-review comment add <pr> -R owner/repo --review-id ID \
  --subject line --path src/a.go --line 12 --side RIGHT --body "..."

gh code-review comment add <pr> -R owner/repo --review-id ID \
  --subject file --path src/a.go --body "..."

gh code-review comment suggest <pr> -R owner/repo --review-id ID \
  --path src/a.go --line 12 --side RIGHT --body "..." \
  --replacement-file replacement.txt

gh code-review comment edit -R owner/repo --comment-id DATABASE_ID --body "..."
gh code-review comment edit -R owner/repo --comment-node-id NODE_ID --body "..."
gh code-review comment delete -R owner/repo --comment-id DATABASE_ID --confirm
gh code-review comment delete -R owner/repo --comment-node-id NODE_ID --confirm
```

For a range, add `--start-line` and `--start-side`.
Use the node ID returned by `comment add`, `comment suggest`, or `thread show`
when editing or deleting a comment in a pending review. Use the database ID for
submitted comments. Specify exactly one identifier.

## Threads

```text
gh code-review thread list <pr> -R owner/repo [--unresolved]
gh code-review thread show <pr> -R owner/repo --thread-id NODE_ID
gh code-review thread reply <pr> -R owner/repo --thread-id NODE_ID --body "..."
gh code-review thread resolve --thread-id NODE_ID --confirm
gh code-review thread unresolve --thread-id NODE_ID --confirm
```

## Reviews

```text
gh code-review review list <pr> -R owner/repo
gh code-review review show <pr> -R owner/repo --review-id ID
gh code-review review edit <pr> -R owner/repo --review-id ID --body "..."
gh code-review review submit <pr> -R owner/repo --review-id ID --event COMMENT
gh code-review review dismiss <pr> -R owner/repo --review-id ID \
  --message "reason" --confirm-timeline-comment
```

`APPROVE` and `REQUEST_CHANGES` require `--confirm-decision`.

## Viewed state

```text
gh code-review file viewed <pr> -R owner/repo --path src/a.go --confirm
gh code-review file unviewed <pr> -R owner/repo --path src/a.go --confirm
```

## Exit codes

```text
0  success
1  internal error
2  input or schema validation
3  PR head changed
4  authentication or authorization
5  GitHub API failure
6  partial transaction requiring recovery
7  unsupported host capability
8  conflict or duplicate operation
```
