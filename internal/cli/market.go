package cli

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
)

func newMarketCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "market",
		Short: "Read public RAP and exists datasets",
		Long: `RAP is a market index, not a guaranteed sale price.
Exists counts are global tallies, not per-player ownership counts.`,
	}
	cmd.AddCommand(newMarketRAPCmd(app))
	cmd.AddCommand(newMarketExistsCmd(app))
	return cmd
}

func newMarketRAPCmd(app *App) *cobra.Command {
	var category, id string
	cmd := &cobra.Command{
		Use:   "rap",
		Short: "Fetch public RAP data",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if category != "" {
				q.Set("category", category)
			}
			if id != "" {
				q.Set("id", id)
			}
			path := "/api/rap"
			pr, err := app.getPublicCached(cmd.Context(), path, q, api.BodyLimitRAP)
			if err != nil {
				return mapNet(app, "market rap", err)
			}
			if err := app.mapPublicHTTP("market rap", path, pr); err != nil {
				return err
			}
			data, err := filterMarketMap(pr.Res.Data, category, id)
			if err != nil {
				return mapNet(app, "market rap", err)
			}
			return app.writePublicSuccess("market rap", path, pr, map[string]any{
				"rap": data,
			}, nil)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "exact category filter when applicable")
	cmd.Flags().StringVar(&id, "id", "", "exact id filter when applicable")
	return cmd
}

func newMarketExistsCmd(app *App) *cobra.Command {
	var category, id string
	cmd := &cobra.Command{
		Use:   "exists",
		Short: "Fetch public exists tallies",
		RunE: func(cmd *cobra.Command, args []string) error {
			q := url.Values{}
			if category != "" {
				q.Set("category", category)
			}
			if id != "" {
				q.Set("id", id)
			}
			path := "/api/exists"
			pr, err := app.getPublicCached(cmd.Context(), path, q, api.BodyLimitExists)
			if err != nil {
				return mapNet(app, "market exists", err)
			}
			if err := app.mapPublicHTTP("market exists", path, pr); err != nil {
				return err
			}
			data, err := filterMarketMap(pr.Res.Data, category, id)
			if err != nil {
				return mapNet(app, "market exists", err)
			}
			return app.writePublicSuccess("market exists", path, pr, map[string]any{
				"exists": data,
			}, nil)
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "exact category filter when applicable")
	cmd.Flags().StringVar(&id, "id", "", "exact id filter when applicable")
	return cmd
}

func filterMarketMap(raw json.RawMessage, category, id string) (any, error) {
	if category == "" && id == "" {
		if !json.Valid(raw) {
			return nil, fmt.Errorf("invalid JSON")
		}
		out := make(json.RawMessage, len(raw))
		copy(out, raw)
		return out, nil
	}
	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		out := make(json.RawMessage, len(raw))
		copy(out, raw)
		return out, nil
	}
	out := map[string]json.RawMessage{}
	for k, v := range asMap {
		if category != "" && !strings.EqualFold(k, category) {
			// also allow nested structures; keep exact top-level key match only
			continue
		}
		if id == "" {
			out[k] = v
			continue
		}
		// If value is an object map of ids, filter inner keys.
		var inner map[string]json.RawMessage
		if json.Unmarshal(v, &inner) == nil {
			kept := map[string]json.RawMessage{}
			for ik, iv := range inner {
				if strings.EqualFold(ik, id) {
					kept[ik] = iv
				}
			}
			b, _ := json.Marshal(kept)
			out[k] = b
			continue
		}
		if strings.EqualFold(k, id) {
			out[k] = v
		}
	}
	return out, nil
}
