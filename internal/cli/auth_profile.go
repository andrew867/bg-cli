package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func newAuthProfileCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Edit stored account identity on a local profile",
		Long: `Reads and writes non-secret identity fields in config.json.

OAuth does not return these values. auth profile set stores them, and a later
auth login keeps them. The access token is not changed.

ps99ClanTag stores the clan tag used by GET /api/clan/{tag}.`,
		Example: `  bg-cli auth profile set --profile main --ps99-user-id 1234 --clan-tag SYN
  bg-cli auth profile unset --profile main --clan-tag`,
	}
	cmd.AddCommand(newAuthProfileSetCmd(app))
	cmd.AddCommand(newAuthProfileUnsetCmd(app))
	return cmd
}

func newAuthProfileSetCmd(app *App) *cobra.Command {
	var userID, clanTag, accountID string
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "set",
		Short: "Set ps99UserId, ps99ClanTag, or accountId",
		Example: `  bg-cli auth profile set --profile main --ps99-user-id 1234 --clan-tag SYN
  bg-cli auth profile set --profile main --account-id 1234 --dry-run`,
		RunE: func(c *cobra.Command, args []string) error {
			fields := map[string]string{}
			if c.Flags().Changed("ps99-user-id") {
				v, err := cleanIdentity("ps99-user-id", userID)
				if err != nil {
					return profileUsage(app, err)
				}
				fields["ps99UserId"] = v
			}
			if c.Flags().Changed("clan-tag") {
				v := strings.TrimSpace(clanTag)
				if err := api.ValidateClanTag(v); err != nil {
					return profileUsage(app, fmt.Errorf("%w\n  bg-cli auth profile set --profile main --clan-tag <TAG>", err))
				}
				fields["ps99ClanTag"] = v
			}
			if c.Flags().Changed("account-id") {
				v, err := cleanIdentity("account-id", accountID)
				if err != nil {
					return profileUsage(app, err)
				}
				fields["accountId"] = v
			}
			if len(fields) == 0 {
				return profileUsage(app, fmt.Errorf("set at least one of --ps99-user-id, --clan-tag, or --account-id\n  bg-cli auth profile set --profile main --ps99-user-id <id> --clan-tag <TAG>"))
			}
			return app.writeProfileIdentity(fields, false, dryRun)
		},
	}
	cmd.Flags().StringVar(&userID, "ps99-user-id", "", "PS99 user id stored as ps99UserId")
	cmd.Flags().StringVar(&clanTag, "clan-tag", "", "clan tag stored as ps99ClanTag")
	cmd.Flags().StringVar(&accountID, "account-id", "", "account id stored as accountId")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the profile fields that would be saved")
	return cmd
}

func newAuthProfileUnsetCmd(app *App) *cobra.Command {
	var userID, clanTag, accountID bool
	var dryRun bool
	cmd := &cobra.Command{
		Use:   "unset",
		Short: "Clear ps99UserId, ps99ClanTag, or accountId",
		Example: `  bg-cli auth profile unset --profile main --ps99-user-id --clan-tag
  bg-cli auth profile unset --profile main --account-id --dry-run`,
		RunE: func(c *cobra.Command, args []string) error {
			fields := map[string]string{}
			if userID {
				fields["ps99UserId"] = ""
			}
			if clanTag {
				fields["ps99ClanTag"] = ""
			}
			if accountID {
				fields["accountId"] = ""
			}
			if len(fields) == 0 {
				return profileUsage(app, fmt.Errorf("name at least one field to clear\n  bg-cli auth profile unset --profile main --ps99-user-id --clan-tag"))
			}
			return app.writeProfileIdentity(fields, true, dryRun)
		},
	}
	cmd.Flags().BoolVar(&userID, "ps99-user-id", false, "clear ps99UserId")
	cmd.Flags().BoolVar(&clanTag, "clan-tag", false, "clear ps99ClanTag")
	cmd.Flags().BoolVar(&accountID, "account-id", false, "clear accountId")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "print the profile fields that would be saved")
	return cmd
}

func profileUsage(app *App, err error) error {
	msg := err.Error()
	_ = app.emit(output.NewError("auth profile", nil, output.DefaultFreshnessNull(), &output.ErrorBody{
		Code:      "usage",
		Message:   msg,
		Retryable: false,
	}))
	return &ExitError{Code: exitcode.Usage, Msg: msg}
}

func cleanIdentity(flag, value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("%s is empty; clear it with auth profile unset\n  bg-cli auth profile unset --profile main --%s", flag, flag)
	}
	if err := config.ValidateAlias(value); err != nil {
		return "", fmt.Errorf("%s %q is not a safe identity value", flag, value)
	}
	return value, nil
}

func (a *App) writeProfileIdentity(fields map[string]string, clearing, dryRun bool) error {
	if err := rejectAuthOutput(a.Flags.Output); err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return exitf(exitcode.Usage, "%v", err)
	}
	alias := a.Flags.Profile
	if alias == "" {
		alias = cfg.DefaultProfile
	}
	if alias == "" {
		alias = config.DefaultProfile
	}
	if err := config.ValidateAlias(alias); err != nil {
		return profileUsage(a, err)
	}
	p, ok := config.Get(cfg, alias)
	if !ok {
		if clearing {
			return profileUsage(a, fmt.Errorf("profile %q not found\n  bg-cli auth profile set --profile %s --ps99-user-id <id> --clan-tag <TAG>", alias, alias))
		}
		p = config.Profile{Alias: alias}
	}
	updated := make([]string, 0, len(fields))
	for _, key := range []string{"accountId", "ps99UserId", "ps99ClanTag"} {
		value, changed := fields[key]
		if !changed {
			continue
		}
		switch key {
		case "accountId":
			p.AccountID = value
		case "ps99UserId":
			p.PS99UserID = value
		case "ps99ClanTag":
			p.PS99ClanTag = value
		}
		updated = append(updated, key)
	}
	if !dryRun {
		if err := config.UpsertProfile(cfg, p, false); err != nil {
			return profileUsage(a, err)
		}
		if err := config.Save(cfg); err != nil {
			return exitf(exitcode.Usage, "%v", err)
		}
	}
	return a.emit(output.NewSuccess("auth profile", nil, output.DefaultFreshnessNull(), map[string]any{
		"profile":     alias,
		"accountId":   p.AccountID,
		"ps99UserId":  p.PS99UserID,
		"ps99ClanTag": p.PS99ClanTag,
		"updated":     updated,
		"dryRun":      dryRun,
	}))
}
