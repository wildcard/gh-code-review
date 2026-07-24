# Non-author pilot checklist

The public preview can be installed and evaluated before any submission to a curated marketplace.

Record only sanitized repository/PR identifiers when required by the tester’s policy.

## Required pilot

- Tester is not the primary author.
- Install the tagged extension in a clean GitHub CLI extensions directory.
- Install the tagged Agent Skill in a clean agent home.
- Inspect a bounded PR with existing review threads.
- Confirm an existing equivalent finding is rejected as a duplicate.
- Validate and submit a `COMMENT` review containing:
  - one RIGHT-side line or range finding;
  - one apply-able suggestion whose replacement line count differs;
  - one file-level finding.
- Verify every thread is attached to one formal review and no flat PR comment exists.
- Force a head change between inspection and validation and confirm exit code 3.
- Exercise the JSON fallback on the same sanitized fixture.
- Capture installation friction, confusing output, and recovery guidance.

## Sign-off

```text
Tester:
Version:
Host:
Extension install: pass/fail
Skill install: pass/fail
Core review transaction: pass/fail
Fallback parity: pass/fail
Head-change protection: pass/fail
Duplicate protection: pass/fail
Friction:
Recommended changes:
```

Do not mark the curated-marketplace checklist complete until this pilot passes and any blocking friction is fixed.
