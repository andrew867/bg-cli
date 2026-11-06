package cli

import (
	"net/url"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/jsonutil"
)

func newCollectionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "collection",
		Short: "Read public collection metadata",
	}
	cmd.AddCommand(newCollectionListCmd(app))
	cmd.AddCommand(newCollectionShowCmd(app))
	return cmd
}

func newCollectionListCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List public collection names",
		RunE: func(cmd *cobra.Command, args []string) error {
			path := "/api/collections"
			pr, err := app.getPublicCached(cmd.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "collection list", err)
			}
			if err := app.mapPublicHTTP("collection list", path, pr); err != nil {
				return err
			}
			return app.writePublicSuccess("collection list", path, pr, map[string]any{
				"collections": jsonutil.CloneRaw(pr.Res.Data),
			}, nil)
		},
	}
}

func newCollectionShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show <name>",
		Short: "Show one public collection document",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			path := "/api/collection/" + url.PathEscape(name)
			pr, err := app.getPublicCached(cmd.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "collection show", err)
			}
			if err := app.mapPublicHTTP("collection show", path, pr); err != nil {
				return err
			}
			return app.writePublicSuccess("collection show", path, pr, map[string]any{
				"collection": jsonutil.CloneRaw(pr.Res.Data),
			}, nil)
		},
	}
}
