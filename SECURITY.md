# Security Policy

## Supported versions

Security fixes are applied to the latest released version on the default branch.

## Reporting a vulnerability

Report security issues privately through GitHub Security Advisories for this repository, or by contacting the repository owner through GitHub.

Please include:

- A description of the issue and its impact
- Steps to reproduce
- Affected version or commit, if known

Do not open public issues for unfixed vulnerabilities. Do not include live access tokens, client secrets, or personal game account data in reports.

## Scope

In scope:

- Secret leakage through CLI output, logs, or committed artifacts
- Improper transmission of Bearer or Basic credentials
- Authentication bypass in local OAuth callback handling
- Path or host allowlist failures that send credentials to unexpected destinations

Out of scope:

- Compromise of the operator's password manager or OS credential store
- Abuse of upstream BIG Games API quotas after valid authorization
- Issues in third-party services that are not part of this repository
