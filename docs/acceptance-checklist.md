# Acceptance checklist

Use this checklist before tagging a release. Deterministic CI green is necessary but not sufficient: authenticated live checks must run on a trusted machine with credentials kept outside chat and source control.

**v0.1.2 status:** maintainer verified all gates below on a trusted local machine (Windows) on 2026-11-06 (America/St_Johns). No secrets, tokens, or account bodies were recorded in the repository.

## Automated (CI)

- [x] `gofmt -l .` clean on Linux
- [x] `go vet ./...` and `go test ./...` pass on Windows, Linux, and macOS
- [x] `go test -race ./...` passes on Linux
- [x] Four release targets cross-compile
- [x] `govulncheck ./...` clean
- [x] Security / secret scan tests pass
- [x] Optional: `public-smoke` workflow green against live public endpoints

## Operator / local (trusted machine)

- [x] Exact loopback redirect URI accepted in the BIG Games developer dashboard
- [x] Developer app has all seven account read scopes enabled
- [x] `bg-cli doctor` reports `healthy: true` after login
- [x] `bg-cli auth login` with default scopes; `auth status` shows `tokenPresent: true`
- [x] `account inventory` and `account currencies` succeed (JSON)
- [x] `account inventory --output csv` and `account currencies --output csv` preserve exact large integers
- [x] `account inventory --output table` shows item columns
- [x] Public: `leaderboard list`, `collection list`, `market rap` without attaching an account token
- [x] Missing leaderboard row returns exit 3 / `not_ranked_or_not_retained`
- [x] Logout deletes local credentials; Connected-apps revocation verified separately when required

## Immediate rejection

Stop the release if any secret appears in source, flags, output, logs, fixtures, or artifacts; if account bodies are cached to disk; if missing fields become invented zeroes; or if authenticated acceptance is claimed without a live authenticated run.

## Evidence notes

Record non-secret evidence (command names, exit codes, timestamps in America/St_Johns) in the release PR or operator notes. Never paste tokens, client secrets, inventory bodies, or mail content into chat or the repository.
