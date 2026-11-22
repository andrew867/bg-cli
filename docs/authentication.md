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

## One grant, one Roblox account

`clientId` is the developer application. It is not a Roblox account, and it does not authorize sending another alias's access token. A hand-added profile does not reuse a credential, even when the Roblox id is the same and even when `credentialProfile` names another alias.

`GET /v1/account/*` returns only the Roblox account that authorized that access token. The body is that player's cached save (`Cache-Control: private, no-cache`). It is not a lookup by `accountId`, `linkedAccountId`, or `ps99UserId`.

Authorize the other Roblox account with a normal login. The consent page selects the player. The new access token is stored only in the matching profile's keyring entry. The other profile's token is not copied. An expired token is not replaced with another profile's token.

```bash
bg-cli auth login --profile alt1 --client-id YOUR_CLIENT_ID --redirect-uri http://127.0.0.1:8765/callback
```

`--client-id` and `--redirect-uri` can be omitted only when they are already stored on that same profile. They are not copied from another alias.

After the code exchange, bg-cli reads `GET /v1/account/profile` with the new token. The documented profile does not include a Roblox user id or a BIG Games linked-account id, so discovery of those ids is unavailable from the official account API. The token stays on the profile you named. If a future response positively includes `linkedAccountId` or a user id, bg-cli fills an empty field or matches an existing alias. It does not rename an alias when the username changes, does not point an alias at a different linked account, and does not delete a profile that was not in the response. If the grant disagrees with the alias, login stops and stores nothing.

`accountId` in an old config is kept and is not treated as `linkedAccountId`. `auth status` warns when that legacy field is set. `auth status` and `auth profiles` show the alias, linked account id, PS99 user id, username, display name, client id, whether a token is present, the credential owner, expiry, and scopes. They do not show the access token.

Dashboard routes such as `https://db.biggames.io/account.data` are not called. A bearer-only read of those routes was not confirmed, and they are not used to list linked accounts.

`alt1` below is a different Roblox account from `main`. Account reads for `alt1` fail until that login finishes.

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
      "redirectUri": "http://127.0.0.1:8765/callback",
      "ps99UserId": "222",
      "ps99ClanTag": "OTHERTAG"
    }
  }
}
```

Logging out `alt1` removes that alias and leaves `main`'s token in place. Do not put the access token or client secret in `config.json`.

Snapshot summaries and manifests record `profileAlias`, `linkedAccountId`, and `ps99UserId` when they are known. File names still use the alias, not the provider id. A previous snapshot, a 5m/30m/1h/session window, or an inventory diff is used only when the linked-account id matches, or when that id is absent on both sides and the PS99 user id matches. If both ids are present they must agree. The alias alone is not identity. A different account is not diffed and is not rated; the reason is that the previous snapshot was rejected because account identity differs. Old summaries stay readable. An old `accountId` is not a linked-account id. It still ties two summaries together only when neither side has a linked-account id or a PS99 user id.
