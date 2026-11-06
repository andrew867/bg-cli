package cli

import (
	"encoding/json"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/jsonutil"
	"github.com/andrew867/bg-cli/internal/output"
)

func newLeaderboardCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "leaderboard",
		Short: "Read public event leaderboards",
		Long: `Leaderboard commands do not require authentication.
A missing retained player row means the player is not ranked or not retained
within the board's bounded standings — not proof of a zero score.`,
	}
	cmd.AddCommand(newLeaderboardListCmd(app))
	cmd.AddCommand(newLeaderboardShowCmd(app))
	cmd.AddCommand(newLeaderboardPlayerCmd(app))
	return cmd
}

func newLeaderboardListCmd(app *App) *cobra.Command {
	var state, id string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List public event leaderboards",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/v1/leaderboards"
			pr, err := app.getPublicCached(cmd.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leaderboard list", err)
			}
			if err := app.mapPublicHTTP("leaderboard list", path, pr); err != nil {
				return err
			}
			var root map[string]json.RawMessage
			_ = json.Unmarshal(pr.Res.Data, &root)
			var boards []map[string]any
			if raw, ok := root["leaderboards"]; ok {
				_ = jsonutil.Decode(raw, &boards)
			} else {
				_ = jsonutil.Decode(pr.Res.Data, &boards)
			}
			filtered := make([]map[string]any, 0, len(boards))
			for _, b := range boards {
				if state != "" {
					if s, _ := b["state"].(string); !strings.EqualFold(s, state) {
						continue
					}
				}
				if id != "" {
					if bid, _ := b["id"].(string); !strings.EqualFold(bid, id) {
						continue
					}
				}
				filtered = append(filtered, b)
			}
			return app.writePublicSuccess("leaderboard list", path, pr, map[string]any{
				"leaderboards": filtered,
				"count":        len(filtered),
			}, nil)
		},
	}
	cmd.Flags().StringVar(&state, "state", "", "exact case-insensitive board state filter")
	cmd.Flags().StringVar(&id, "id", "", "exact case-insensitive board id filter")
	return cmd
}

func newLeaderboardShowCmd(app *App) *cobra.Command {
	var source string
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "show <board-id>",
		Short: "Show a leaderboard page",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardID := args[0]
			if source == "" {
				source = "live"
			}
			if page < 1 {
				page = 1
			}
			if pageSize < 1 || pageSize > 100 {
				return exitf(exitcode.Usage, "--page-size must be between 1 and 100")
			}
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("pageSize", strconv.Itoa(pageSize))
			q.Set("source", source)
			path := "/v1/leaderboards/" + url.PathEscape(boardID)
			pr, err := app.getPublicCached(cmd.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leaderboard show", err)
			}
			if err := app.mapPublicHTTP("leaderboard show", path, pr); err != nil {
				return err
			}
			return app.writePublicSuccess("leaderboard show", path, pr, map[string]any{
				"page": jsonutil.CloneRaw(pr.Res.Data),
			}, nil)
		},
	}
	cmd.Flags().StringVar(&source, "source", "live", "live or final")
	cmd.Flags().IntVar(&page, "page", 1, "page number (1-based)")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "page size (1-100)")
	return cmd
}

func newLeaderboardPlayerCmd(app *App) *cobra.Command {
	var source string
	cmd := &cobra.Command{
		Use:   "player <board-id> <roblox-user-id>",
		Short: "Show one retained leaderboard player row",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			boardID, userID := args[0], args[1]
			if source == "" {
				source = "live"
			}
			q := url.Values{}
			q.Set("source", source)
			path := "/v1/leaderboards/" + url.PathEscape(boardID) + "/players/" + url.PathEscape(userID)
			pr, err := app.getPublicCached(cmd.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "leaderboard player", err)
			}
			if pr.Res.HTTPStatus == 404 {
				st := 404
				msg := "not_ranked_or_not_retained: player row is absent from bounded standings (not proof of zero score)"
				_ = app.emit(output.NewError("leaderboard player", &output.Source{
					Provider: "BIG Games", Classification: "official", BaseURL: api.ProductionAPIBase,
					Endpoint: path, HTTPStatus: st, RetrievedAt: app.nowUTC(), FromLocalCache: pr.FromLocalCache,
				}, output.DefaultFreshnessNull(), &output.ErrorBody{
					Code: "not_ranked_or_not_retained", Message: msg, Retryable: false, UpstreamStatus: &st,
				}))
				return &ExitError{Code: exitcode.NotFound, Msg: msg}
			}
			if err := app.mapPublicHTTP("leaderboard player", path, pr); err != nil {
				return err
			}
			return app.writePublicSuccess("leaderboard player", path, pr, map[string]any{
				"player": jsonutil.CloneRaw(pr.Res.Data),
			}, nil)
		},
	}
	cmd.Flags().StringVar(&source, "source", "live", "live or final")
	return cmd
}

func (a *App) nowUTC() time.Time {
	if a.Deps.Clock != nil {
		return a.Deps.Clock.Now()
	}
	if a.Deps.Now != nil {
		return a.Deps.Now()
	}
	return time.Now().UTC()
}
