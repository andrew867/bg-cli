package cli

import (
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/jsonutil"
	"github.com/andrew867/bg-cli/internal/output"
)

func newPlayerCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "player",
		Short: "Read opted-in public player views",
		Long: `Public player commands read only views a player has published.
Authentication does not unlock another player's private account data.`,
	}
	cmd.AddCommand(newPlayerViewsCmd(app))
	cmd.AddCommand(newPlayerShowCmd(app))
	return cmd
}

func newPlayerViewsCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "views <username-or-user-id>",
		Short: "Show which public views are available for a player",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			path := "/v1/players/" + url.PathEscape(slug)
			pr, err := app.getPublicCached(cmd.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "player views", err)
			}
			if err := app.mapPublicHTTP("player views", path, pr); err != nil {
				return err
			}
			return app.writePublicSuccess("player views", path, pr, map[string]any{
				"player": jsonutil.CloneRaw(pr.Res.Data),
			}, nil)
		},
	}
}

func newPlayerShowCmd(app *App) *cobra.Command {
	var include string
	cmd := &cobra.Command{
		Use:   "show <username-or-user-id>",
		Short: "Fetch selected public player views",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			slug := args[0]
			views := splitCSV(include)
			if len(views) == 0 {
				return exitf(exitcode.Usage, "--include is required (e.g. profile,inventory)")
			}
			q := url.Values{}
			q.Set("include", strings.Join(views, ","))
			path := "/v1/players/" + url.PathEscape(slug)
			pr, err := app.getPublicCached(cmd.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "player show", err)
			}
			if err := app.mapPublicHTTP("player show", path, pr); err != nil {
				return err
			}

			var root map[string]json.RawMessage
			_ = json.Unmarshal(pr.Res.Data, &root)
			viewsObj := map[string]any{}
			unavailable := []map[string]any{}
			if raw, ok := root["views"]; ok {
				var vm map[string]json.RawMessage
				_ = json.Unmarshal(raw, &vm)
				for name, vraw := range vm {
					var meta map[string]any
					_ = jsonutil.Decode(vraw, &meta)
					avail, _ := meta["available"].(bool)
					if !avail {
						reason, _ := meta["reason"].(string)
						if reason == "" {
							reason = "unavailable"
						}
						unavailable = append(unavailable, map[string]any{"view": name, "reason": reason})
						continue
					}
					viewsObj[name] = jsonutil.CloneRaw(vraw)
				}
			}
			if len(unavailable) > 0 && len(viewsObj) == 0 {
				st := pr.Res.HTTPStatus
				msg := "requested public view is not available for this player"
				_ = app.emit(output.NewError("player show", &output.Source{
					Provider: "BIG Games", Classification: "official", BaseURL: api.ProductionAPIBase,
					Endpoint: path, HTTPStatus: st, RetrievedAt: time.Now().UTC(), FromLocalCache: pr.FromLocalCache,
				}, output.DefaultFreshnessNull(), &output.ErrorBody{
					Code: "view_not_public", Message: msg, Retryable: false, UpstreamStatus: &st,
					Details: map[string]any{"unavailable": unavailable},
				}))
				return &ExitError{Code: exitcode.NotFound, Msg: msg}
			}

			var snap *time.Time
			// Prefer explicit fetchedAt inside a view when present.
			for _, v := range viewsObj {
				raw, ok := v.(json.RawMessage)
				if !ok {
					continue
				}
				var m map[string]any
				if jsonutil.Decode(raw, &m) != nil {
					continue
				}
				s, _ := m["fetchedAt"].(string)
				if s == "" {
					continue
				}
				if t, err := time.Parse(time.RFC3339Nano, s); err == nil {
					snap = &t
					break
				}
				if t, err := time.Parse(time.RFC3339, s); err == nil {
					snap = &t
					break
				}
			}

			var account any
			if raw, ok := root["account"]; ok {
				account = jsonutil.CloneRaw(raw)
			}
			return app.writePublicSuccess("player show", path, pr, map[string]any{
				"account":     account,
				"views":       viewsObj,
				"unavailable": unavailable,
			}, snap)
		},
	}
	cmd.Flags().StringVar(&include, "include", "", "comma-separated public views to request")
	_ = cmd.MarkFlagRequired("include")
	return cmd
}

func splitCSV(raw string) []string {
	parts := strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' })
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func mapNet(app *App, command string, err error) error {
	msg := err.Error()
	code := exitcode.Network
	if strings.Contains(msg, "invalid JSON") {
		code = exitcode.Upstream
	}
	_ = app.emit(output.NewError(command, nil, output.DefaultFreshnessNull(), &output.ErrorBody{
		Code: "request_failed", Message: msg, Retryable: false,
	}))
	return &ExitError{Code: code, Msg: msg}
}
