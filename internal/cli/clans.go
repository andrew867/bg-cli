package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/publicintel"
)

func newClansCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "clans",
		Short: "Public clan aggregates and battle detail (sampled coverage)",
	}
	cmd.AddCommand(newClansPlayersCmd(app))
	cmd.AddCommand(newClansBattleCmd(app))
	return cmd
}

func newClansPlayersCmd(app *App) *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "players",
		Short: "Clan player aggregate sample (not a complete population)",
		RunE: func(c *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("pageSize", strconv.Itoa(pageSize))
			path := "/v1/clans/players"
			pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "clans players", err)
			}
			if err := app.mapPublicHTTP("clans players", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseClanPlayers(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("clans players", path, pr, rep, nil)
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "page size")
	return cmd
}

func newClansBattleCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "battle BATTLE_ID",
		Short: "Clan battle detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "/v1/clans/battles/" + url.PathEscape(args[0])
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "clans battle", err)
			}
			if err := app.mapPublicHTTP("clans battle", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseClanBattle(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("clans battle", path, pr, rep, nil)
		},
	}
}
