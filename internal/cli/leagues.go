package cli

import (
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/publicintel"
)

func newLeaguesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leagues",
		Short: "Public league seasons and rankings (sampled contributor lists)",
	}
	cmd.AddCommand(newLeaguesSeasonsCmd(app))
	cmd.AddCommand(newLeaguesLeaderboardCmd(app))
	cmd.AddCommand(newLeaguesPlayersCmd(app))
	return cmd
}

func newLeaguesSeasonsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "seasons",
		Short: "List league seasons (camelCase metadata)",
		RunE: func(c *cobra.Command, args []string) error {
			path := "/v1/leagues/seasons"
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leagues seasons", err)
			}
			if err := app.mapPublicHTTP("leagues seasons", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseSeasons(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("leagues seasons", path, pr, rep, nil)
		},
	}
}

func newLeaguesLeaderboardCmd(app *App) *cobra.Command {
	var season string
	var page, pageSize int
	var allPages bool
	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "Paginated league leaderboard (PascalCase rows; bounded ranking depth)",
		RunE: func(c *cobra.Command, args []string) error {
			q := url.Values{}
			if season != "" {
				q.Set("season", season)
			}
			q.Set("page", strconv.Itoa(page))
			q.Set("pageSize", strconv.Itoa(pageSize))
			path := "/v1/leagues"
			pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leagues leaderboard", err)
			}
			if err := app.mapPublicHTTP("leagues leaderboard", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseLeagueLeaderboard(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			_ = allPages // full multi-page concatenation can be added when Total/HasMore confirmed per season
			return app.writePublicSuccess("leagues leaderboard", path, pr, rep, nil)
		},
	}
	cmd.Flags().StringVar(&season, "season", "", "season id")
	cmd.Flags().IntVar(&page, "page", 1, "page")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "page size")
	cmd.Flags().BoolVar(&allPages, "all-pages", false, "reserved: follow pagination when available")
	return cmd
}

func newLeaguesPlayersCmd(app *App) *cobra.Command {
	var season string
	cmd := &cobra.Command{
		Use:   "players",
		Short: "Top contributor sample (documented top-500; not all players)",
		RunE: func(c *cobra.Command, args []string) error {
			q := url.Values{}
			if season != "" {
				q.Set("season", season)
			}
			path := "/v1/leagues/players"
			pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leagues players", err)
			}
			if err := app.mapPublicHTTP("leagues players", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseLeaguePlayers(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("leagues players", path, pr, rep, nil)
		},
	}
	cmd.Flags().StringVar(&season, "season", "", "season id")
	return cmd
}
