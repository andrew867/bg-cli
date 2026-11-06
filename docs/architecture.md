# Architecture

## Layout

```text
cmd/bg-cli/          process entrypoint
internal/api/        official HTTP client, host/path allowlists
internal/auth/       OAuth, PKCE, loopback callback, keyring store
internal/cache/      public response cache only
internal/cli/        Cobra command wiring
internal/config/     non-secret profile metadata
internal/exitcode/   stable process codes
internal/output/     JSON and table envelopes
internal/redact/     secret scrubbing helpers
testdata/            synthetic fixtures and output schema
```

## Request paths

```text
auth login
  → loopback listener
  → browser authorize (db.biggames.io)
  → token exchange (db.biggames.io/oauth/token)
  → OS credential store + local config metadata

account *
  → load profile metadata + keyring token
  → scope preflight
  → GET https://ps99.biggamesapi.io/v1/account/...
  → JSON envelope on stdout

player|leaderboard|collection|market *
  → optional public cache lookup
  → unauthenticated GET
  → JSON envelope on stdout
```

## Design constraints

- Production binaries do not accept arbitrary API base URLs. Tests inject local servers through internal constructors.
- Account bodies are excluded from disk cache by design.
- Output defaults to one JSON object so automation can rely on a stable contract.
