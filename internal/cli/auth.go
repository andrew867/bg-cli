package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

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
account under a distinct --profile alias.

auth logout removes the local token only. To revoke server-side access, open
Connected apps in the BIG Games account dashboard.`,
	}
	cmd.AddCommand(newAuthLoginCmd(app))
	cmd.AddCommand(newAuthStatusCmd(app))
	cmd.AddCommand(newAuthLogoutCmd(app))
	cmd.AddCommand(newAuthProfilesCmd(app))
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

By default every documented account read scope is requested. Pass --scopes to
opt out and request a smaller set instead.

Client secret is never accepted as a flag. Provide it via:
  - masked interactive prompt
  - environment variable BG_CLI_CLIENT_SECRET (clear after login)
  - stdin when not a TTY

Auth commands do not support --output raw.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if app.Flags.Output == "raw" {
				return exitf(exitcode.Usage, "auth commands do not support --output raw")
			}
			profile := app.Flags.Profile
			if profile == "" {
				profile = config.DefaultProfile
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

			res, secrets, err := auth.Login(ctx, auth.LoginOptions{
				Profile:      profile,
				ClientID:     clientID,
				RedirectURI:  redirectURI,
				Scopes:       scopes,
				NoOpen:       noOpen,
				TokenStore:   app.tokenStore(),
				SecretSource: secretSrc,
				Stderr:       app.stderr(),
				OpenURL:      app.Deps.OpenURL,
				Now:          app.Deps.Now,
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
	cmd.Flags().StringVar(&clientID, "client-id", "", "OAuth client ID (non-secret)")
	cmd.Flags().StringVar(&redirectURI, "redirect-uri", "", "exact registered loopback redirect URI")
	cmd.Flags().StringVar(&scopesRaw, "scopes", "", "opt-out: request only these scopes (default: all seven account read scopes)")
	cmd.Flags().BoolVar(&noOpen, "no-open", false, "print authorize URL instead of opening a browser")
	_ = cmd.MarkFlagRequired("client-id")
	_ = cmd.MarkFlagRequired("redirect-uri")
	return cmd
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
			if app.Flags.Output == "raw" {
				return exitf(exitcode.Usage, "auth commands do not support --output raw")
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
			p, ok := config.Get(cfg, profile)
			if !ok {
				return writeAuthError(app, "auth status", exitcode.AuthRequired, "not_logged_in", fmt.Sprintf("profile %q not found", profile))
			}

			store := app.tokenStore()
			_, tokErr := store.Get(profile)
			hasToken := tokErr == nil
			if tokErr != nil && !strings.Contains(tokErr.Error(), "not found") && tokErr.Error() != "secret not found in keyring" {
				// zalando returns keyring.ErrNotFound
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

			data := map[string]any{
				"profile":      p.Alias,
				"clientId":     p.ClientID,
				"scopes":       p.Scopes,
				"issuedAt":     p.IssuedAt.UTC().Format(time.RFC3339),
				"expiresAt":    p.ExpiresAt.UTC().Format(time.RFC3339),
				"tokenPresent": hasToken,
				"expired":      time.Now().UTC().After(p.ExpiresAt),
				"redirectUri":  p.RedirectURI,
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
			if app.Flags.Output == "raw" {
				return exitf(exitcode.Usage, "auth commands do not support --output raw")
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
			if app.Flags.Output == "raw" {
				return exitf(exitcode.Usage, "auth commands do not support --output raw")
			}
			cfg, err := config.Load()
			if err != nil {
				return exitf(exitcode.Usage, "%v", err)
			}
			list := make([]map[string]any, 0, len(cfg.Profiles))
			for _, p := range cfg.Profiles {
				list = append(list, map[string]any{
					"alias":     p.Alias,
					"clientId":  p.ClientID,
					"scopes":    p.Scopes,
					"expiresAt": p.ExpiresAt.UTC().Format(time.RFC3339),
					"isDefault": p.Alias == cfg.DefaultProfile,
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

func writeAuthError(app *App, command string, code int, errCode, msg string) error {
	_ = app.emit(output.NewError(command, nil, output.DefaultFreshnessNull(), &output.ErrorBody{
		Code:      errCode,
		Message:   msg,
		Retryable: false,
	}))
	return &ExitError{Code: code, Msg: msg}
}
