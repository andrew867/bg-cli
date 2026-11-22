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

Running `auth login` again for an existing profile replaces the token, client id, scopes, and expiry. It keeps `accountId`, `ps99UserId`, and `ps99ClanTag`. Those fields are not part of the OAuth response, and a reauthorization does not erase them. Reauthorizing an existing profile assumes it still represents the same game account. To authorize a different account, use a new profile alias or log out that profile first so stored account identity metadata is not carried forward.

## Stored account identity

OAuth does not return the player id or clan tag. Until login can read them from an account endpoint, store them on the local profile. Key order in `config.json` does not matter. A later save writes them after `redirectUri`.

```bash
bg-cli auth profile set --profile main --ps99-user-id 1234 --clan-tag YOURTAG
bg-cli auth profile unset --profile main --clan-tag
```

```json
{
  "defaultProfile": "main",
  "profiles": {
    "main": {
      "redirectUri": "http://127.0.0.1:8765/callback",
      "accountId": "1234",
      "ps99UserId": "1234",
      "ps99ClanTag": "YOURTAG"
    }
  }
}
```

`ps99UserId` is the PS99 user id used for battle contribution and rank. `--user-id` overrides it. `ps99ClanTag` is the clan tag for `GET /api/clan/{tag}`. `clan show` uses `--tag`, then the stored tag, and only then the account documents. A stored tag does not require an access token. `account snapshot` still prefers a tag on the profile or extended profile, then `ps99ClanTag`. `--clan-tag` overrides that snapshot order. `auth status` prints the three fields.

## Multiple accounts

```bash
bg-cli auth login --profile main ...
bg-cli auth login --profile alt1 ...
bg-cli account inventory --profile main
bg-cli account inventory --profile alt1
```

One token never grants private access to another Roblox player. A second login is how you authorize a different player.

## Same OAuth client, second alias

The keyring entry belongs to the OAuth client (`clientId`), not to the Roblox id. You can add another profile in `config.json` for a different Roblox id without a second login and without pasting a token when that profile uses the same `clientId` as a profile that already logged in.

`alt1` below omits scopes, expiry, and `redirectUri`. bg-cli uses the values already stored on `main`, including that callback. `accountId` and `ps99UserId` stay the values written on `alt1`.

```json
{
  "defaultProfile": "main",
  "profiles": {
    "main": {
      "alias": "main",
      "clientId": "YOUR_CLIENT_ID",
      "redirectUri": "http://127.0.0.1:8765/callback",
      "accountId": "111",
      "ps99UserId": "111"
    },
    "alt1": {
      "alias": "alt1",
      "clientId": "YOUR_CLIENT_ID",
      "accountId": "222",
      "ps99UserId": "222",
      "ps99ClanTag": "OTHERTAG"
    }
  }
}
```

Account commands for `alt1` send the token stored for that client. `auth status` reports `credentialProfile` as the alias that holds the keyring entry. Logging out only `alt1` removes that alias and leaves the token in place. Logging out the alias that holds the token removes the shared credential.

A different `clientId` does not receive this token. Authorize that client with `auth login`. Do not put the access token or client secret in `config.json`.
