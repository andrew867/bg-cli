# bg-cli

Unofficial, owner-operated, read-only CLI for official BIG Games APIs. Pet Simulator 99 is the first game surface; the binary name stays game-agnostic for future titles.

**Not affiliated with or endorsed by BIG Games.**

## What it does

- Authorize one or more Roblox accounts via official OAuth 2.0 Authorization Code + PKCE (S256)
- Store access tokens only in the OS credential store (Windows Credential Manager, macOS Keychain, Linux Secret Service)
- Read private account data for the authorizing profile only (inventory, profile/currencies, and other scoped views — later milestones)
- Read public leaderboards, opted-in player views, collections, RAP, and exists (later milestones)

## What it does not do

- Never asks for a Roblox password or `.ROBLOSECURITY` cookie
- Never embeds or accepts `client_secret` as a command-line flag
- Never caches `/v1/account/*` response bodies on disk
- Never automates gameplay, trading, mail, purchases, or mutations
- One access token never unlocks another player's private data

Account responses are **cached snapshots**, not real time. Refresh cadence follows BIG Games quota rules.

## Install

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@latest
```

Or build from source:

```bash
git clone https://github.com/andrew867/bg-cli.git
cd bg-cli
go build -o bg-cli ./cmd/bg-cli
```

## Account reads (Milestone 3)

Requires a logged-in profile with the matching scope:

```bash
bg-cli account inventory --profile main
bg-cli account profile --profile main
bg-cli account currencies --profile main
```

`--refresh` may consume daily quota and requires `--yes` when not interactive. Account bodies are never cached to disk. Missing `Currency` is reported as `currencyFieldPresent: false`, not zeroes.

## Authentication setup

1. Register a private developer app at https://db.biggames.io/settings/developer-apps
2. Register an **exact** loopback redirect URI, for example `http://127.0.0.1:8765/callback`
3. Enable only the scopes you need (start with inventory + profile read)
4. Keep `client_secret` in your password manager — never commit it

```bash
bg-cli auth login --profile main --client-id YOUR_CLIENT_ID --redirect-uri http://127.0.0.1:8765/callback
```

Supply the client secret via masked prompt, stdin, or `BG_CLI_CLIENT_SECRET` for the login process only. Clear the env var afterward.

```bash
bg-cli auth status --profile main
bg-cli auth profiles
bg-cli auth logout --profile main
```

`auth logout` deletes local credentials only. Revoke Connected apps in the BIG Games dashboard to end server-side access.

If the developer dashboard rejects loopback redirect URIs, a confidential token-exchange backend is required and this private CLI mode cannot proceed as designed.

## Output

Default stdout is one JSON object conforming to `testdata/schemas/cli-output.schema.json`. Diagnostics go to stderr. Secrets are never printed.

## Exit codes

| Code | Meaning |
| ---: | --- |
| 0 | Success |
| 2 | Invalid arguments / local validation |
| 3 | Public resource not found |
| 4 | Network failure |
| 5 | Upstream error / invalid JSON |
| 6 | HTTP 429 |
| 7 | Secure credential store failure |
| 8 | Not logged in / expired / missing scope |
| 9 | OAuth failure |

## Development

```bash
go test ./...
go vet ./...
GOOS=windows GOARCH=amd64 go build -o dist/bg-cli-windows-amd64.exe ./cmd/bg-cli
GOOS=linux GOARCH=amd64 go build -o dist/bg-cli-linux-amd64 ./cmd/bg-cli
GOOS=darwin GOARCH=amd64 go build -o dist/bg-cli-darwin-amd64 ./cmd/bg-cli
GOOS=darwin GOARCH=arm64 go build -o dist/bg-cli-darwin-arm64 ./cmd/bg-cli
```

## License

MIT
