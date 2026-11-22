package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func newAuthCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage OAuth profiles and access tokens",
		Long: `Authorize Roblox accounts through the official BIG Games OAuth flow.

One access token reads only the account that approved consent. Authorize each
Roblox account with its own login. The access token is stored only in that
profile's keyring entry. clientId is the developer application, not an
account, and it does not select another profile's token.

auth logout removes the local token only. To revoke server-side access, open
Connected apps in the BIG Games account dashboard.`,
	}
	cmd.AddCommand(newAuthLoginCmd(app))
	cmd.AddCommand(newAuthStatusCmd(app))
	cmd.AddCommand(newAuthLogoutCmd(app))
	cmd.AddCommand(newAuthProfilesCmd(app))
	cmd.AddCommand(newAuthProfileCmd(app))
	return cmd
}

func newAuthLoginCmd(app *App) *cobra.Command {
	var (
		clientID    string
		redirectURI string
		scopesRaw   string
		noOpen      bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Authorize a profile via OAuth Authorization Code + PKCE",
		Long: `Starts a loopback callback, opens the BIG Games consent page, exchanges the
authorization code, and stores the access token in the OS credential store.

The consent page selects one Roblox account. Unless that account profile
includes an identity field, the selection is trusted for the profile you
named. The access token is stored only in that profile's keyring entry.
A new grant starts a new snapshot history lineage, so a mistaken consent
click does not continue earlier history. The token is not copied onto
another alias. An alias is not renamed when a username changes, and it is
not pointed at a different linked account.

When both --client-id and --redirect-uri are omitted, bg-cli uses the first
complete shared developer app: this profile, then oauth in config.json, then
the default profile. The named profile does not have to exist yet. Passing
only one of those flags does not borrow the other half from an older app.
Those values are not a player. The bearer is stored only under the profile
you named, and that profile is created when it is new.

After the code exchange, bg-cli reads GET /v1/account/profile with the new
token. That official document does not include the Roblox user id or a
linked-account id, so discovery of those ids is unfinished. Dashboard
account routes are not called.

By default every documented account read scope is requested. Pass --scopes to
opt out and request a smaller set instead.

Client secret is never accepted as a flag. Provide it via:
  - masked interactive prompt
  - environment variable BG_CLI_CLIENT_SECRET (clear after login)
  - stdin when not a TTY

Auth commands do not support --output raw or --output csv.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectAuthOutput(app.Flags.Output); err != nil {
				return err
			}
			profile := app.Flags.Profile
			if profile == "" {
				profile = config.DefaultProfile
			}
			clientID, redirectURI, err := app.resolveLoginParams(profile, clientID, redirectURI)
			if err != nil {
				return err
			}
			scopes, err := auth.ParseScopeList(scopesRaw)
			if err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}

			secretSrc := auth.SecretSource{
				Env:    app.Deps.Env,
				Stdin:  cmd.InOrStdin(),
				Stderr: app.stderr(),
				IsTTY:  app.Deps.IsTTY,
			}
			if secretSrc.Env == nil {
				secretSrc.Env = os.Getenv
			}

			ctx, cancel := context.WithTimeout(cmd.Context(), 6*time.Minute)
			defer cancel()

			app.notice("Authorizing profile %s. The consent screen selects the Roblox account. Unless an identity field appears, the access token is stored on this profile.", profile)
			res, secrets, err := auth.Login(ctx, auth.LoginOptions{
				Profile:      profile,
				ClientID:     clientID,
				RedirectURI:  redirectURI,
				Scopes:       scopes,
				NoOpen:       noOpen,
				AuthorizeURL: app.Deps.OAuthAuthorizeURL,
				TokenURL:     app.Deps.OAuthTokenURL,
				HTTPClient:   app.Deps.HTTPClient,
				TokenStore:   app.tokenStore(),
				SecretSource: secretSrc,
				Stderr:       app.stderr(),
				OpenURL:      app.Deps.OpenURL,
				Now:          app.Deps.Now,
				Discover:     app.discoverOAuthAccount,
			})
			if err != nil {
				msg := err.Error()
				if secrets != nil {
					msg = secrets.Redact(msg)
				}
				code := exitcode.OAuthFailure
				if strings.Contains(msg, "credential store") || strings.Contains(err.Error(), "secure credential store") {
					code = exitcode.CredentialStore
				}
				if strings.Contains(msg, "invalid profile") || strings.Contains(msg, "required") || strings.Contains(msg, "unknown scope") || strings.Contains(msg, "redirect URI") || strings.Contains(msg, "loopback") {
					if !strings.Contains(msg, "state mismatch") && !strings.Contains(msg, "authorization") && !strings.Contains(msg, "token") {
						code = exitcode.Usage
					}
				}
				_ = app.emit(output.NewError("auth login", nil, output.DefaultFreshnessNull(), &output.ErrorBody{
					Code:      "oauth_failure",
					Message:   msg,
					Retryable: false,
				}))
				return &ExitError{Code: code, Msg: msg}
			}

			data := map[string]any{
				"profile":   res.Profile,
				"scopes":    res.Scopes,
				"issuedAt":  res.IssuedAt.UTC().Format(time.RFC3339),
				"expiresAt": res.ExpiresAt.UTC().Format(time.RFC3339),
				"message":   "Access token stored in the OS credential store. Secrets are never printed.",
			}
			env := output.NewSuccess("auth login", nil, output.DefaultFreshnessNull(), data)
			return app.emit(env)
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID (non-secret). Pass with --redirect-uri, or omit both to use the shared developer app")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "exact registered loopback redirect URI. Pass with --client-id, or omit both to use the shared developer app")
	cmd.Flags().StringVar(&scopesRaw, "scopes", "", "opt-out: request only these scopes (default: all seven account read scopes)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print authorize URL instead of opening a browser")
	return cmd
}

// resolveLoginParams fills the developer application for one login.
// It reads no keyring token and does not copy one profile's access token to another.
func (a *App) resolveLoginParams(profile, clientID, redirectURI string) (string, string, error) {
	clientID = strings.TrimSpace(clientID)
	redirectURI = strings.TrimSpace(redirectURI)
	if clientID != "" && redirectURI != "" {
		return clientID, redirectURI, nil
	}
	cfg, err := config.Load()
	if err != nil && (clientID == "" || redirectURI == "") {
		return "", "", exitf(exitcode.Usage, "client-id and redirect-uri are required\n  bg-cli auth login --profile %s --client-id <id> --redirect-uri <uri>", profile)
	}
	clientID, redirectURI = config.ResolveDeveloperApp(cfg, profile, clientID, redirectURI)
	if clientID == "" {
		return "", "", exitf(exitcode.Usage, "client-id is required\n  bg-cli auth login --profile %s --client-id <id> --redirect-uri <uri>", profile)
	}
	if redirectURI == "" {
		return "", "", exitf(exitcode.Usage, "redirect-uri is required\n  bg-cli auth login --profile %s --client-id <id> --redirect-uri <uri>", profile)
	}
	return clientID, redirectURI, nil
}

func newAuthStatusCmd(app *App) *cobra.Command {
	var check bool
	var yes bool
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Show local authorization metadata for a profile",
		Long: `By default, auth status is local only and performs no network request.

--check performs one minimal authenticated API request and may affect refresh
quota. Interactive runs print a warning; non-interactive runs require --yes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectAuthOutput(app.Flags.Output); err != nil {
				return err
			}
			profile := app.Flags.Profile
			cfg, err := config.Load()
			if err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			if profile == "" {
				profile = cfg.DefaultProfile
			}
			if profile == "" {
				return writeAuthError(app, "auth status", exitcode.AuthRequired, "not_logged_in", "no profile configured; run auth login")
			}
			p, aliases, ok := config.BindAccount(cfg, profile)
			if !ok {
				return writeAuthError(app, "auth status", exitcode.AuthRequired, "not_logged_in", fmt.Sprintf("profile %q not found", profile))
			}

			hasToken, _, tokErr := app.profileHasAccountToken(cfg, p, aliases)
			if tokErr != nil && !tokenMissing(tokErr) {
				if strings.Contains(strings.ToLower(tokErr.Error()), "credential store") {
					return writeAuthError(app, "auth status", exitcode.CredentialStore, "credential_store", tokErr.Error())
				}
			}

			if check {
				isTTY := false
				if app.Deps.IsTTY != nil {
					isTTY = app.Deps.IsTTY()
				}
				if !isTTY && !yes {
					return exitf(exitcode.Usage, "--check may consume refresh quota; pass --yes for non-interactive use")
				}
				app.notice("Warning: --check performs an authenticated API request and may affect refresh quota.")
				return app.runAccountView(cmd.Context(), accountViewRequest{
					Command:       "auth status",
					View:          "profile",
					RequiredScope: "player-data:pet-simulator-99:profile:read",
					Flags:         accountFlags{},
				})
			}

			owner := ""
			if hasToken {
				owner = p.Alias
			}
			data := map[string]any{
				"profile":         p.Alias,
				"alias":           p.Alias,
				"linkedAccountId": p.LinkedAccountID,
				"ps99UserId":      p.PS99UserID,
				"username":        p.Username,
				"displayName":     p.DisplayName,
				"clientId":        p.ClientID,
				"scopes":          p.Scopes,
				"issuedAt":        p.IssuedAt.UTC().Format(time.RFC3339),
				"expiresAt":       p.ExpiresAt.UTC().Format(time.RFC3339),
				"tokenPresent":    hasToken,
				"credentialOwner": owner,
				"expired":         tokenExpired(app.nowUTC(), p),
				"redirectUri":     p.RedirectURI,
				"accountId":       p.AccountID,
				"ps99ClanTag":     p.PS99ClanTag,
				"credentialEpoch": p.CredentialEpoch,
				"warnings":        config.LegacyIdentityWarnings(p),
			}
			return app.emit(output.NewSuccess("auth status", nil, output.DefaultFreshnessNull(), data))
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "perform one authenticated API request (may use refresh quota)")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm quota-affecting --check in non-interactive mode")
	return cmd
}

func newAuthLogoutCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Delete local token and profile metadata",
		Long: `Removes the access token from the OS credential store and deletes local profile
metadata. This does not revoke the connected app on BIG Games servers.

To revoke server-side: open the BIG Games account dashboard → Connected apps.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectAuthOutput(app.Flags.Output); err != nil {
				return err
			}
			profile := app.Flags.Profile
			if profile == "" {
				return exitf(exitcode.Usage, "--profile is required for auth logout")
			}
			if err := config.ValidateAlias(profile); err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			store := app.tokenStore()
			if err := store.Delete(profile); err != nil {
				return writeAuthError(app, "auth logout", exitcode.CredentialStore, "credential_store", err.Error())
			}
			cfg, err := config.Load()
			if err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			config.DeleteProfile(cfg, profile)
			if err := config.Save(cfg); err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			data := map[string]any{
				"profile": profile,
				"message": "Local token and metadata deleted. Revoke Connected apps in the BIG Games dashboard to end server-side access.",
			}
			return app.emit(output.NewSuccess("auth logout", nil, output.DefaultFreshnessNull(), data))
		},
	}
}

func newAuthProfilesCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "profiles",
		Short: "List local authorization profile aliases",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := rejectAuthOutput(app.Flags.Output); err != nil {
				return err
			}
			cfg, err := config.Load()
			if err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			list := make([]map[string]any, 0, len(cfg.Profiles))
			for _, raw := range cfg.Profiles {
				alias := raw.Alias
				p, credAliases, bound := config.BindAccount(cfg, alias)
				if !bound {
					p = raw
					credAliases = []string{alias}
				}
				hasToken, owner, tokErr := app.profileHasAccountToken(cfg, p, credAliases)
				if tokErr != nil || !hasToken {
					hasToken = false
					owner = ""
				}
				list = append(list, map[string]any{
					"alias":           p.Alias,
					"linkedAccountId": p.LinkedAccountID,
					"ps99UserId":      p.PS99UserID,
					"username":        p.Username,
					"displayName":     p.DisplayName,
					"clientId":        p.ClientID,
					"scopes":          p.Scopes,
					"expiresAt":       p.ExpiresAt.UTC().Format(time.RFC3339),
					"tokenPresent":    hasToken,
					"credentialOwner": owner,
					"credentialEpoch": p.CredentialEpoch,
					"isDefault":       p.Alias == cfg.DefaultProfile,
					"warnings":        config.LegacyIdentityWarnings(p),
				})
			}
			data := map[string]any{
				"defaultProfile": cfg.DefaultProfile,
				"profiles":       list,
			}
			return app.emit(output.NewSuccess("auth profiles", nil, output.DefaultFreshnessNull(), data))
		},
	}
}

func (a *App) discoverOAuthAccount(ctx context.Context, accessToken string) (config.GrantIdentity, error) {
	res, err := a.apiClient().GetProfile(ctx, accessToken, api.AccountGetOptions{})
	if err != nil {
		return config.GrantIdentity{}, err
	}
	if res.HTTPStatus < 200 || res.HTTPStatus >= 300 {
		return config.GrantIdentity{}, fmt.Errorf("account profile status %d", res.HTTPStatus)
	}
	return config.DiscoverProfile(res.Data), nil
}

func writeAuthError(app *App, command string, code int, errCode, msg string) error {
	_ = app.emit(output.NewError(command, nil, output.DefaultFreshnessNull(), &output.ErrorBody{
		Code:      errCode,
		Message:   msg,
		Retryable: false,
	}))
	return &ExitError{Code: code, Msg: msg}
}

func rejectAuthOutput(mode string) error {
	switch mode {
	case "raw", "csv":
		return exitf(exitcode.Usage, "auth commands do not support --output %s", mode)
	default:
		return nil
	}
}
