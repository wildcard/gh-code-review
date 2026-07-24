# Curated Claude marketplace preparation

This repository is a self-hosted Claude marketplace. No submission to a curated Anthropic marketplace is performed by repository automation.

Before a future human submission:

- [ ] Latest public release is immutable and installable.
- [ ] Strict `claude plugin validate . --strict` passes at the release tag.
- [ ] A non-author completes [the pilot checklist](pilot-checklist.md).
- [ ] Plugin and marketplace versions match the release tag.
- [ ] Public README, license, security policy, and changelog are current.
- [ ] The plugin contains no hooks, background services, credentials, or model invocation.
- [ ] The bundled fallback is standard-library Python plus authenticated `gh api`.
- [ ] Repository provenance and secret scans are clean.
- [ ] Release binaries, checksums, SBOM, and attestations are present.
- [ ] The submission copy accurately calls v0.1 a public preview.

Submission is intentionally a separate human action.
