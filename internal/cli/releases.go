package cli

import (
	"encoding/json"
	"net/url"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/publicintel"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newReleasesCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "releases",
		Short: "Public game release and item-change intelligence (no auth)",
	}
	cmd.AddCommand(newReleasesListCmd(app))
	cmd.AddCommand(newReleasesShowCmd(app))
	cmd.AddCommand(newReleasesChangesCmd(app))
	cmd.AddCommand(newReleasesDiffCmd(app))
	cmd.AddCommand(newReleasesAffectedCmd(app))
	return cmd
}

func newReleasesListCmd(app *App) *cobra.Command {
	var page, pageSize int
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List published releases",
		RunE: func(c *cobra.Command, args []string) error {
			q := url.Values{}
			q.Set("page", strconv.Itoa(page))
			q.Set("pageSize", strconv.Itoa(pageSize))
			path := "/v1/releases"
			pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "releases list", err)
			}
			if err := app.mapPublicHTTP("releases list", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseReleasesList(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("releases list", path, pr, rep, nil)
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "page size (max 100)")
	return cmd
}

func newReleasesShowCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "show VERSION|latest",
		Short: "Show one release detail",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			path := "/v1/releases/" + url.PathEscape(args[0])
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "releases show", err)
			}
			if err := app.mapPublicHTTP("releases show", path, pr); err != nil {
				return err
			}
			rep, err := publicintel.ParseReleaseDetail(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("releases show", path, pr, rep, nil)
		},
	}
}

func newReleasesChangesCmd(app *App) *cobra.Command {
	var since string
	var allPages bool
	var pageSize int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Item changes feed since a version or time",
		RunE: func(c *cobra.Command, args []string) error {
			if since == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--since is required"}
			}
			var pages []publicintel.ItemChangesPage
			page := 1
			for {
				q := url.Values{}
				q.Set("since", since)
				q.Set("page", strconv.Itoa(page))
				q.Set("pageSize", strconv.Itoa(pageSize))
				path := "/v1/items/changes"
				pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
				if err != nil {
					return mapNet(app, "releases changes", err)
				}
				if err := app.mapPublicHTTP("releases changes", path, pr); err != nil {
					return err
				}
				p, err := publicintel.ParseItemChanges(pr.Res.Body)
				if err != nil {
					return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
				}
				pages = append(pages, p)
				if !allPages || !p.HasMore {
					break
				}
				page++
				if page > 1000 {
					return &ExitError{Code: exitcode.Upstream, Msg: "pagination safety stop"}
				}
			}
			rep := publicintel.BuildItemChangesReport(pages)
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("releases changes", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "version or supported since value")
	cmd.Flags().BoolVar(&allPages, "all-pages", false, "follow hasMore until complete")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "page size")
	return cmd
}

func newReleasesDiffCmd(app *App) *cobra.Command {
	var fromV, toV string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two release detail payloads by version",
		RunE: func(c *cobra.Command, args []string) error {
			if fromV == "" || toV == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			fetch := func(ver string) (publicintel.ReleaseDetailReport, error) {
				path := "/v1/releases/" + url.PathEscape(ver)
				pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
				if err != nil {
					return publicintel.ReleaseDetailReport{}, err
				}
				if err := app.mapPublicHTTP("releases diff", path, pr); err != nil {
					return publicintel.ReleaseDetailReport{}, err
				}
				return publicintel.ParseReleaseDetail(pr.Res.Body)
			}
			aRep, err := fetch(fromV)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: "from: " + err.Error()}
			}
			bRep, err := fetch(toV)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: "to: " + err.Error()}
			}
			payload := map[string]any{
				"from": aRep, "to": bRep,
				"methodId": "releases-diff.v1",
				"note":     "field-level historical reconstruction is unavailable from current item URLs; compare release detail payloads only",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("releases diff", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&fromV, "from", "", "from version")
	cmd.Flags().StringVar(&toV, "to", "", "to version")
	return cmd
}

func newReleasesAffectedCmd(app *App) *cobra.Command {
	var since, snap string
	cmd := &cobra.Command{
		Use:   "affected",
		Short: "Intersect item changes since a version with owned inventory IDs",
		RunE: func(c *cobra.Command, args []string) error {
			if since == "" || snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--since and --snapshot are required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			q := url.Values{}
			q.Set("since", since)
			q.Set("page", "1")
			q.Set("pageSize", "100")
			path := "/v1/items/changes"
			pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "releases affected", err)
			}
			if err := app.mapPublicHTTP("releases affected", path, pr); err != nil {
				return err
			}
			page, err := publicintel.ParseItemChanges(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			owned := map[string]bool{}
			if b.Inventory != nil {
				var items []map[string]any
				collectInventoryItemMaps(b.Inventory, &items)
				for _, it := range items {
					if id, ok := it["id"].(string); ok {
						owned[id] = true
					}
				}
			}
			var hit []publicintel.ItemChangeRow
			for _, row := range page.Changes {
				if owned[row.ConfigName] {
					hit = append(hit, row)
				}
			}
			payload := map[string]any{
				"since": since, "ownedMatches": hit, "pageHasMore": page.HasMore,
				"note": "single page; ownership match by configName only",
			}
			return app.writePublicSuccess("releases affected", path, pr, payload, nil)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "since version")
	cmd.Flags().StringVar(&snap, "snapshot", "", "offline snapshot for owned IDs")
	return cmd
}

func collectInventoryItemMaps(inv json.RawMessage, out *[]map[string]any) {
	var top map[string]any
	if err := json.Unmarshal(inv, &top); err == nil {
		if items, ok := top["items"].([]any); ok {
			for _, it := range items {
				if m, ok := it.(map[string]any); ok {
					*out = append(*out, m)
				}
			}
		}
	}
}
