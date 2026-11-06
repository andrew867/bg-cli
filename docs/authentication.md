# Authentication

bg-cli uses the official BIG Games OAuth 2.0 Authorization Code flow with PKCE S256. The CLI never collects Roblox passwords or browser cookies.

## Credential types

| Value | Secret? | Role |
| --- | --- | --- |
| Client ID | No | Identifies the developer app in authorize and token requests |
| Client secret | Yes | Authenticates the token exchange with HTTP Basic auth |
| Access token | Yes | Bearer credential for `/v1/account/*` after player consent |

Do not treat a developer secret as a player bearer token. Label and validate each value by its dashboard field name.

## Register a developer app

1. Open https://db.biggames.io/settings/developer-apps
2. Create a private application
3. Register the exact redirect URI the CLI will use (scheme, host, path, and port). Example: `http://127.0.0.1:8765/callback`
4. Enable all seven Pet Simulator 99 account read scopes on the app. `bg-cli auth login` requests all of them by default.
5. Store the client secret in a password manager

If the dashboard rejects loopback redirect URIs, stop. A confidential callback and token-exchange service is required for that deployment model.

## Login

```bash
bg-cli auth login \
  --profile main \
  --client-id YOUR_CLIENT_ID \
  --redirect-uri http://127.0.0.1:8765/callback
```

Optional flags:

| Flag | Purpose |
| --- | --- |
| `--scopes` | Opt out of the full set: request only this comma/space-separated list |
| `--no-open` | Print the authorize URL instead of opening a browser |

Client secret input (never a CLI flag):

1. Masked interactive prompt when stdin is a TTY
2. `BG_CLI_CLIENT_SECRET` for the login process environment
3. A single line on stdin when non-interactive

Clear `BG_CLI_CLIENT_SECRET` after login. Do not place it in shell profiles, repositories, CI logs, or screenshots.

On success the CLI:

- Verifies OAuth `state` with a constant-time comparison
- Exchanges the authorization code once (no redirects, no automatic retries)
- Validates bearer token type, positive expiry, and granted scopes
- Stores the access token in the OS credential store
- Writes non-secret metadata to `config.json`

## Profiles

Each authorized Roblox account is stored under a local alias (`main`, `alt1`, …). Account selection happens on the BIG Games consent page. The alias is only a local label.

```bash
bg-cli auth profiles
bg-cli auth status --profile main
bg-cli auth logout --profile main
```

`auth status` is local by default. `--check` performs one authenticated profile request and may affect refresh quota. Non-interactive `--check` requires `--yes`.

`auth logout` deletes the local token and metadata. It does not revoke the connected app on BIG Games servers. Revoke access from the account dashboard under Connected apps when access should end immediately.

## Token lifetime

Access tokens are valid for 30 days. There is no refresh-token endpoint. When a token expires or is revoked, account commands exit with code 8 and login must be repeated.

## Multiple accounts

```bash
bg-cli auth login --profile main ...
bg-cli auth login --profile alt1 ...
bg-cli account inventory --profile main
bg-cli account inventory --profile alt1
```

One token never grants private access to another player.
