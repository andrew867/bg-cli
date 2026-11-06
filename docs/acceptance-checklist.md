# Acceptance checklist

Use this checklist before tagging a release. Deterministic CI green is necessary but not sufficient: authenticated live checks must run on a trusted machine with credentials kept outside chat and source control.

## Automated (CI)

- [ ] `gofmt -l .` clean on Linux
- [ ] `go vet ./...` and `go test ./...` pass on Windows, Linux, and macOS
- [ ] `go test -race ./...` passes on Linux
- [ ] Four release targets cross-compile
- [ ] `govulncheck ./...` clean
- [ ] Security / secret scan tests pass
- [ ] Optional: `public-smoke` workflow green against live public endpoints

## Operator / local (trusted machine)

- [ ] Exact loopback redirect URI accepted in the BIG Games developer dashboard
- [ ] Developer app has all seven account read scopes enabled
- [ ] `bg-cli doctor` reports `healthy: true` after login
- [ ] `bg-cli auth login` with default scopes; `auth status` shows `tokenPresent: true`
- [ ] `account inventory` and `account currencies` succeed (JSON)
- [ ] `account inventory --output csv` and `account currencies --output csv` preserve exact large integers
- [ ] `account inventory --output table` shows item columns
- [ ] Public: `leaderboard list`, `collection list`, `market rap` without attaching an account token
- [ ] Missing leaderboard row returns exit 3 / `not_ranked_or_not_retained`
- [ ] Logout deletes local credentials; Connected-apps revocation verified separately when required

## Immediate rejection

Stop the release if any secret appears in source, flags, output, logs, fixtures, or artifacts; if account bodies are cached to disk; if missing fields become invented zeroes; or if authenticated acceptance is claimed without a live authenticated run.

## Evidence notes

Record non-secret evidence (command names, exit codes, timestamps in America/St_Johns) in the release PR or operator notes. Never paste tokens, client secrets, inventory bodies, or mail content into chat or the repository.
