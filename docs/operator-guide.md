# Operator guide

This guide covers routine operation of a private bg-cli installation.

## Initial setup checklist

1. Create the developer app and record the client ID (non-secret).
2. Store the client secret in a password manager.
3. Register the exact loopback redirect URI used by the CLI.
4. Enable all seven account read scopes on the developer app (login requests them by default).
5. Confirm the OS credential store is available for the interactive user session.
6. Run `bg-cli auth login` and verify `bg-cli auth status` shows `tokenPresent: true`.

## Daily use

```bash
bg-cli account currencies --profile main --pretty
bg-cli account inventory --profile main --class Pet --pretty
bg-cli leaderboard list --pretty
```

Avoid polling. Prefer on-demand reads. Use `--refresh` only when a fresher snapshot is required and quota remains.

## Token expiry

Tokens expire after 30 days. Account commands return exit code 8 when the token is missing, expired, or revoked. Re-run `auth login` for that profile.

## Rotation and revocation

| Event | Action |
| --- | --- |
| Client secret may have leaked | Rotate the secret in the developer dashboard; redeploy operator procedures |
| Access should end for a player | Revoke the app under Connected apps; optionally `auth logout` locally |
| Profile no longer needed | `bg-cli auth logout --profile <alias>` |
| Machine is shared or being decommissioned | Logout all profiles; clear the OS credential entries for service `bg-cli` |

## Automation

For non-interactive login on a trusted host:

```bash
BG_CLI_CLIENT_SECRET=... bg-cli auth login \
  --profile main \
  --client-id YOUR_CLIENT_ID \
  --redirect-uri http://127.0.0.1:8765/callback \
  --no-open
```

Complete the browser consent step out of band if required, or run login interactively once per machine. Clear the environment variable afterward.

Scripts should:

- Rely on exit codes rather than scraping stderr
- Parse the JSON envelope on stdout
- Treat exit 6 as rate limiting and honor `retryAfterSeconds` without busy loops
- Never log raw mode output from authenticated endpoints to shared systems

## Backups

Back up non-secret `config.json` if profile aliases matter. Do not attempt to back up access tokens from the credential store into files. Re-authorization is the recovery path.

## Upgrades

```bash
go install github.com/andrew867/bg-cli/cmd/bg-cli@latest
bg-cli version
```

Review `CHANGELOG.md` before production upgrades. Configuration and credential store layouts are intended to remain compatible across 0.x releases unless noted otherwise.
