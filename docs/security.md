# Security model

## Trust boundaries

bg-cli is designed for a private, owner-operated deployment. The operator owns the BIG Games developer application and keeps its client secret outside the binary and repository.

| Asset | Storage |
| --- | --- |
| Client ID | Local config (non-secret) |
| Client secret | Operator password manager; supplied only at login |
| Access token | OS credential store |
| PKCE verifier / OAuth state | Process memory for the duration of login |
| Account API bodies | stdout by default; deliberate `account snapshot` exports are user-authorized local files |

## Hard requirements

- Client secret is never accepted as a command-line flag
- Client secret is never embedded in source, fixtures, or release artifacts
- Access tokens are never printed to stdout or stderr
- Bearer credentials are sent only to `https://ps99.biggamesapi.io` on allowlisted `/v1/account/*` paths
- HTTP Basic credentials for token exchange are sent only to `https://db.biggames.io/oauth/token`
- Account requests reject redirects; token exchange rejects redirects and automatic retries
- Public requests never attach account tokens
- No Roblox password, cookie, browser-session extraction, or game automation is implemented

## OAuth protections

- Cryptographically random state and PKCE verifier
- S256 code challenge
- Loopback listener started before the browser opens
- One-shot callback with path checks, query size limits, and timeout
- Constant-time state comparison before code exchange
- Granted scopes validated before token storage

## Public distribution

Shipping one shared client secret inside a distributable client is unsupported by the current BIG Games token endpoint. Multi-user public deployments require a confidential backend that performs token exchange. That architecture is outside this repository.

## Operational hygiene

- Rotate the client secret in the developer dashboard if exposure is suspected
- Revoke Connected apps when access should end immediately
- Prefer short-lived environment variables over persistent shell configuration for secrets
- Treat CI logs and crash dumps as untrusted sinks; redaction is best-effort

## Reporting vulnerabilities

See [SECURITY.md](../SECURITY.md).

## Snapshot privacy

Deliberate local snapshot exports (`account snapshot`) write account payloads to a user-chosen directory. The default three-view export does **not** include trades, booth, or mail. Capturing those histories requires explicit `--all-views` or `--include`. Do not commit private snapshot directories, upload them to CI, or attach them to issues.
