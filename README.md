# bg-cli

`bg-cli` is an unofficial, owner-operated command-line client for the official BIG Games HTTP APIs. Pet Simulator 99 is the first supported game surface. The tool is read-only with respect to game state: it authorizes accounts through OAuth, reads consented private snapshots, and reads public leaderboard and catalog data.

**Not affiliated with or endorsed by BIG Games.**

## Features

- OAuth 2.0 Authorization Code with PKCE (S256) against `db.biggames.io`
- Access tokens stored in the operating system credential store only
- Authenticated account reads for inventory, profile, currencies, and related views
- Public leaderboards, opted-in player views, collections, RAP, and exists
- Stable JSON output (default), richer table view, CSV export for inventory/currencies, and raw upstream bodies for data commands
- `doctor` and shell completions for local setup checks
- Deterministic exit codes suitable for scripting

## Requirements

- Go 1.23 or newer to build from source
- A BIG Games developer application for private account access
- An OS credential store: Windows Credential Manager, macOS Keychain, or Linux Secret Service

## Install

**Prebuilt archives** (Linux, macOS, Windows) ship with every tagged release, plus a source tarball and SHA-256 checksums. Download from [Releases](https://github.com/andrew867/bg-cli/releases), extract, and put `bg-cli` on your `PATH`. Details: [Installation](docs/installation.md).

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@latest
```

From a local checkout:

```bash
git clone https://github.com/andrew867/bg-cli.git
cd bg-cli
go build -o bg-cli ./cmd/bg-cli
```

## Quick start

1. Register a developer app at [BIG Games developer apps](https://db.biggames.io/settings/developer-apps).
2. Add an exact loopback redirect URI, for example `http://127.0.0.1:8765/callback`.
3. Enable all account read scopes on the developer app (bg-cli requests all of them by default; narrow with `auth login --scopes` if you prefer).
4. Log in:

```bash
bg-cli auth login \
  --profile main \
  --client-id YOUR_CLIENT_ID \
  --redirect-uri http://127.0.0.1:8765/callback
```

Provide the client secret through the masked prompt, stdin, or the `BG_CLI_CLIENT_SECRET` environment variable for that process only. Never pass the secret as a command-line flag.

```bash
bg-cli doctor
bg-cli account quota --profile main --pretty
bg-cli account snapshot --profile main --dir . --prefix ps99 --note "baseline"
bg-cli account snapshot diff --from OLD-manifest.json --to NEW-manifest.json --pretty
bg-cli account currencies --profile main --pretty
bg-cli account inventory --profile main --pretty
bg-cli account inventory --profile main --output csv --join-rap --excel > inventory.csv
bg-cli leaderboard list --pretty
```

## Documentation

| Document | Description |
| --- | --- |
| [Installation](docs/installation.md) | Build, install, prebuilt archives, releases |
| [GitHub repo settings](docs/github-repo-settings.md) | Description, topics, release triggers |
| [Authentication](docs/authentication.md) | Developer apps, OAuth, profiles, rotation |
| [Command reference](docs/commands.md) | Flags and subcommands |
| [Output and errors](docs/output-and-errors.md) | JSON schema, exit codes, freshness |
| [Security](docs/security.md) | Secret handling and threat model |
| [Operator guide](docs/operator-guide.md) | Day-two operations |
| [Acceptance checklist](docs/acceptance-checklist.md) | Pre-release verification gates |
| [SECURITY.md](SECURITY.md) | Vulnerability reporting |
| [CHANGELOG.md](CHANGELOG.md) | Release history |

## Important limits

- One access token reads **only** the Roblox account that authorized it.
- Account payloads are **cached snapshots**, not live game state. Refresh cadence is governed by BIG Games quota.
- Missing currency fields or leaderboard rows are **not** interpreted as zero.
- RAP is an index, not a guaranteed sale price. Exists counts are global, not per-player.
- Distributing a shared `client_secret` inside a public binary is unsupported. Public multi-tenant deployments require a confidential token-exchange backend.

## Development

```bash
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
```

Cross-compile targets:

```bash
GOOS=windows GOARCH=amd64 go build -o dist/bg-cli-windows-amd64.exe ./cmd/bg-cli
GOOS=linux   GOARCH=amd64 go build -o dist/bg-cli-linux-amd64 ./cmd/bg-cli
GOOS=darwin  GOARCH=amd64 go build -o dist/bg-cli-darwin-amd64 ./cmd/bg-cli
GOOS=darwin  GOARCH=arm64 go build -o dist/bg-cli-darwin-arm64 ./cmd/bg-cli
```

## License

MIT. See [LICENSE](LICENSE).
