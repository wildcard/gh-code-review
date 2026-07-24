# Security policy

## Reporting a vulnerability

Please use GitHub private vulnerability reporting for this repository. Do not open a public issue containing an exploit, access token, private pull-request content, or other sensitive evidence.

## Supported versions

The latest tagged public-preview release receives security fixes.

## Trust boundary

`gh-code-review` uses credentials already managed by GitHub CLI. It does not accept or store GitHub tokens directly. It sends only normalized review fields to GitHub; agent metadata remains local.

The tool validates locations and transaction state but does not determine whether a review finding is correct. Review generated manifests before submission, especially when using `APPROVE` or `REQUEST_CHANGES`.
