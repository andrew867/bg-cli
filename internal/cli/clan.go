package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/exitcode"
)

func newClanCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clan",
		Short: "Read-only clan document and active clan battle",
		Long: `Reads the public clan document and the current clan battle.

clan show calls GET /api/clan/{tag}. The tag comes from the signed-in profile
or extended profile, or from --tag. The document includes Battles, which is the
per-clan history the API returns. There is no separate historic HTTP route.
Sampled battle detail by id remains "clans battle" (GET /v1/clans/battles/:id).

clan battle calls GET /api/activeClanBattle. active is true only when the
schedule is open: StartTime <= now < FinishTime, and Enabled is not false.

These routes are public. No Authorization header is sent and refresh is never
requested.`,
		Example: `  bg-cli clan show --profile main
  bg-cli clan show --tag SYN
  bg-cli clan battle`,
	}
	cmd.AddCommand(newClanShowCmd(app))
	cmd.AddCommand(newClanBattleCmd(app))
	return cmd
}

func newClanShowCmd(app *App) *cobra.Command {
	var tag string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show one clan, including its Battles history",
		Example: `  bg-cli clan show --profile main
  bg-cli clan show --tag SYN`,
		RunE: func(c *cobra.Command, args []string) error {
			source := "flag"
			field := "--tag"
			if strings.TrimSpace(tag) == "" {
				var err error
				tag, field, source, err = app.resolveClanTag(c.Context())
				if err != nil {
					return err
				}
			}
			if err := api.ValidateClanTag(tag); err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error() + "\n  bg-cli clan show --tag <CLAN_TAG>"}
			}
			path := "/api/clan/" + url.PathEscape(tag)
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "clan show", err)
			}
			if err := app.mapPublicHTTP("clan show", path, pr); err != nil {
				return err
			}
			if len(pr.Res.Data) == 0 || string(pr.Res.Data) == "null" {
				return &ExitError{Code: exitcode.Upstream, Msg: "clan payload is empty"}
			}
			return app.writePublicSuccess("clan show", path, pr, map[string]any{
				"tag":       tag,
				"tagField":  field,
				"tagSource": source,
				"clan":      json.RawMessage(pr.Res.Data),
			}, nil)
		},
	}
	cmd.Flags().StringVar(&tag, "tag", "", "clan tag; skips the account lookup when set")
	return cmd
}

func newClanBattleCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:     "battle",
		Short:   "Show the active clan battle, if one is scheduled now",
		Example: `  bg-cli clan battle`,
		RunE: func(c *cobra.Command, args []string) error {
			const path = "/api/activeClanBattle"
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "clan battle", err)
			}
			if err := app.mapPublicHTTP("clan battle", path, pr); err != nil {
				return err
			}
			now := app.nowLocal()
			userID, _ := app.configuredPS99UserID("")
			observed := clan.Observe(now, userID, "", nil, pr.Res.Data, false, true)
			active := observed.ActiveBattle != nil
			data := map[string]any{
				"active": active,
				"reason": observed.Reason,
			}
			if observed.ActiveBattle != nil {
				data["id"] = observed.ActiveBattle.ID
				data["title"] = observed.ActiveBattle.Title
				data["startTime"] = observed.ActiveBattle.StartTime
				data["finishTime"] = observed.ActiveBattle.FinishTime
			}
			if len(pr.Res.Data) > 0 && string(pr.Res.Data) != "null" {
				data["battle"] = json.RawMessage(pr.Res.Data)
			}
			return app.writePublicSuccess("clan battle", path, pr, data, nil)
		},
	}
}

func (a *App) resolveClanTag(ctx context.Context) (tag, field, source string, err error) {
	profile, err := a.fetchAccountData(ctx, "/v1/account/profile", api.ScopeProfile)
	if err != nil {
		return "", "", "", err
	}
	extended, extErr := a.fetchAccountData(ctx, "/v1/account/extendedProfile", api.ScopeExtendedProfile)
	if extErr != nil {
		a.notice("extended profile lookup skipped: %v", extErr)
	}
	tag, field, ok := clan.ExtractTag(profile, extended)
	if !ok {
		return "", "", "", &ExitError{
			Code: exitcode.Usage,
			Msg:  "clan tag is absent from the signed-in profile and extended profile\n  bg-cli clan show --tag <CLAN_TAG>",
		}
	}
	return tag, field, "profile", nil
}

func (a *App) fetchAccountData(ctx context.Context, path, scope string) (json.RawMessage, error) {
	_, tok, secrets, err := a.resolveProfileToken(scope)
	if err != nil {
		return nil, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetAccountView(ctx, path, tok, api.AccountGetOptions{})
	if err != nil {
		return nil, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return nil, err
	}
	return append(json.RawMessage(nil), res.Data...), nil
}

func (a *App) configuredPS99UserID(flag string) (string, bool) {
	configured := ""
	if p, _, _, err := a.resolveProfileToken(""); err == nil {
		configured = p.PS99UserID
	}
	return clan.ResolveUserID(flag, configured)
}

func (a *App) configuredAccountID(profileBody json.RawMessage) string {
	if id := clan.ExtractAccountID(profileBody); id != "" {
		return id
	}
	if p, _, _, err := a.resolveProfileToken(""); err == nil {
		return strings.TrimSpace(p.AccountID)
	}
	return ""
}
