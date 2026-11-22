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
bg-cli auth login --profile alt1
```

The first login stores the client id and redirect URI on the shared `oauth` object. A later `auth login --profile alt1` uses that developer app and does not ask for those values again. The access token from the second login is stored only under `alt1`.

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

Running `auth login` again for an existing profile replaces the token, client id, scopes, expiry, and credential epoch. It keeps `accountId`, `ps99UserId`, `ps99ClanTag`, and a linked-account id already stored. Those fields are not part of the OAuth token response. The new epoch starts a new snapshot lineage unless a provider-verified linked-account id says this is the same account. That stops a mistaken consent click from being diffed against the previous history. To authorize a different account, use a new profile alias.

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

`ps99UserId` is the account's PS99 user id. Snapshot history and `summary.ps99UserId` use that stored id. `--user-id` selects the battle contribution and rank row for one snapshot and does not replace `ps99UserId`. When the flag is omitted, contribution uses the stored id. `ps99ClanTag` is the clan tag for `GET /api/clan/{tag}`. `clan show` uses `--tag`, then the stored tag, and only then the account documents. A stored tag does not require an access token. `account snapshot` still prefers a tag on the profile or extended profile, then `ps99ClanTag`. `--clan-tag` overrides that snapshot order. `auth status` prints the three fields.

## Multiple accounts

```bash
bg-cli auth login --profile main ...
bg-cli auth login --profile alt1 ...
bg-cli account inventory --profile main
bg-cli account inventory --profile alt1
```

One token never grants private access to another Roblox player. A second login is how you authorize a different player.

## One grant, one Roblox account

`clientId` and `redirectUri` are the developer application. They are not a Roblox account, and they do not authorize sending another alias's access token. Each profile uses only its own keyring token. A hand-added profile does not reuse a credential, even when the Roblox id is the same. An old `credentialProfile` value is ignored and is not written back.

`GET /v1/account/*` returns only the Roblox account that authorized that access token. The body is that player's cached save (`Cache-Control: private, no-cache`). It is not a lookup by `accountId`, `linkedAccountId`, or `ps99UserId`.

Authorize the other Roblox account with a normal login. The consent screen selects the player. Unless the account profile actually includes an identity field, that selection is trusted for the alias you named. The new access token is stored only in that profile's keyring entry. The other profile's token is not copied. An expired token is not replaced with another profile's token. A new grant also stores a new `credentialEpoch`. That epoch is not a token hash. It keeps a mistaken consent click from continuing the previous snapshot history.

```bash
bg-cli auth login --profile alt1
```

`--client-id` and `--redirect-uri` can both be omitted when a complete pair is already on that profile, on the shared `oauth` object, or on the default profile. The profile named by `--profile` is created if it does not exist yet. Passing only one of the two flags does not pair a new client with an older redirect. Omitting them does not copy another profile's access token.

After the code exchange, bg-cli reads `GET /v1/account/profile` with the new token. The documented profile does not include a Roblox user id or a BIG Games linked-account id, so automatic discovery of linked accounts is unfinished. If a response positively includes `linkedAccountId` or a user id, bg-cli fills an empty field or matches an existing alias. It does not rename an alias when the username changes, does not point an alias at a different linked account, and does not delete a profile that was not in the response. If the returned identity disagrees with the alias, login stops and stores nothing.

`accountId` in an old config is kept and is not treated as `linkedAccountId`. `auth status` warns when that legacy field is set. `auth status` and `auth profiles` show the alias, linked account id, PS99 user id, username, display name, client id, whether a token is present, the credential owner, credential epoch, expiry, and scopes. They do not show the access token.

Dashboard routes such as `https://db.biggames.io/account.data` are not called. A bearer-only read of those routes was not confirmed, and they are not used to list linked accounts.

`alt1` below is a different Roblox account from `main`. Account reads for `alt1` fail until that login finishes.

```json
{
  "defaultProfile": "main",
  "oauth": {
    "clientId": "YOUR_CLIENT_ID",
    "redirectUri": "http://127.0.0.1:8765/callback"
  },
  "profiles": {
    "main": {
      "alias": "main",
      "credentialEpoch": "created-when-the-grant-is-installed",
      "accountId": "111",
      "ps99UserId": "111"
    },
    "alt1": {
      "alias": "alt1",
      "ps99UserId": "222",
      "ps99ClanTag": "OTHERTAG"
    }
  }
}
```

Logging out `alt1` removes that alias and leaves `main`'s token in place. Do not put the access token or client secret in `config.json`.

Snapshot summaries and manifests record `profileAlias`, `linkedAccountId`, `ps99UserId`, and `credentialEpoch` when they are known. File names still use the alias, not the provider id. A previous snapshot, a 5m/30m/1h/session window, or an inventory diff is used when the linked-account ids match. That id is provider-verified identity. A PS99 user id is metadata and is not proof of the bearer. When the linked-account id is absent, a snapshot that has a credential epoch matches only the same epoch. Two older summaries that both omit the epoch still compare with each other. The first snapshot after a new independent grant starts a new lineage, so an older file that stored this alias's user id on another account's payload is not a baseline. If both a linked-account id and a PS99 user id are present they must agree. The alias alone is not identity. A different account is not diffed and is not rated; the reason is that the previous snapshot was rejected because account identity differs. Old summaries stay readable. An old `accountId` is not a linked-account id. It still ties two summaries together only when neither side has a linked-account id, a PS99 user id, or a credential epoch.
