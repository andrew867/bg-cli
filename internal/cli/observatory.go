package cli

import (
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/observatory"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/publicintel"
)

func newPublicObservatoryCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "public",
		Short: "Offline-first public catalog observatory (no account refresh quota)",
		Long: `Discover, sync, inspect, and diff official public collections.
Catalog sync uses public endpoints only and never spends account inventory refresh quota.`,
	}
	cmd.AddCommand(newPublicDiscoverCmd(app))
	cmd.AddCommand(newPublicSyncCmd(app))
	cmd.AddCommand(newPublicCollectionsCmd(app))
	cmd.AddCommand(newPublicStatusCmd(app))
	cmd.AddCommand(newPublicInspectCmd(app))
	cmd.AddCommand(newPublicDiffCmd(app))
	cmd.AddCommand(newPublicExportCmd(app))
	cmd.AddCommand(newPublicChangesCmd(app))
	return cmd
}

func newPublicChangesCmd(app *App) *cobra.Command {
	// Same official /v1/items/changes surface as releases changes — no invented feed.
	var since string
	var allPages bool
	var pageSize int
	cmd := &cobra.Command{
		Use:   "changes",
		Short: "Official item-change feed (same as releases changes)",
		Long:  "Uses GET /v1/items/changes only. Does not invent historic full egg snapshots from change records.",
		RunE: func(c *cobra.Command, args []string) error {
			if since == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--since is required"}
			}
			var pages []publicintel.ItemChangesPage
			page := 1
			var lastPR publicResult
			for {
				q := url.Values{}
				q.Set("since", since)
				q.Set("page", strconv.Itoa(page))
				q.Set("pageSize", strconv.Itoa(pageSize))
				path := "/v1/items/changes"
				pr, err := app.getPublicCached(c.Context(), path, q, api.BodyLimitDefault)
				if err != nil {
					return mapNet(app, "public changes", err)
				}
				if err := app.mapPublicHTTP("public changes", path, pr); err != nil {
					return err
				}
				lastPR = pr
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
			return app.writePublicSuccess("public changes", "/v1/items/changes", lastPR, rep, nil)
		},
	}
	cmd.Flags().StringVar(&since, "since", "", "version or supported since value")
	cmd.Flags().BoolVar(&allPages, "all-pages", false, "follow hasMore until complete")
	cmd.Flags().IntVar(&pageSize, "page-size", 100, "page size")
	return cmd
}

func newPublicDiscoverCmd(app *App) *cobra.Command {
	var fixture string
	cmd := &cobra.Command{
		Use:   "discover",
		Short: "List public collection names from API or a local fixture",
		RunE: func(c *cobra.Command, args []string) error {
			if fixture != "" {
				b, err := os.ReadFile(fixture)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
				list, err := observatory.ParseCollectionsList(b)
				if err != nil {
					return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
				}
				w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
				return w.Write(output.NewSuccess("public discover", nil, output.DefaultFreshnessNull(), map[string]any{
					"collections": list.Names, "source": "fixture",
				}))
			}
			path := "/api/collections"
			pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
			if err != nil {
				return mapNet(app, "public discover", err)
			}
			if err := app.mapPublicHTTP("public discover", path, pr); err != nil {
				return err
			}
			list, err := observatory.ParseCollectionsList(pr.Res.Body)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			return app.writePublicSuccess("public discover", path, pr, map[string]any{
				"collections": list.Names,
				"note":        "names only; presence ≠ completeness of joinable fields",
			}, nil)
		},
	}
	cmd.Flags().StringVar(&fixture, "fixture", "", "optional local /api/collections JSON (offline)")
	return cmd
}

func newPublicSyncCmd(app *App) *cobra.Command {
	var dir, collections string
	var all, incremental bool
	cmd := &cobra.Command{
		Use:   "sync",
		Short: "Fetch public collections into a local catalog store (never uses account tokens)",
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				dir = filepath.Join(".", ".bg-cli-catalog")
			}
			var names []string
			if collections != "" {
				for _, n := range strings.Split(collections, ",") {
					n = strings.TrimSpace(n)
					if n != "" {
						names = append(names, n)
					}
				}
			} else {
				path := "/api/collections"
				pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
				if err != nil {
					return mapNet(app, "public sync", err)
				}
				if err := app.mapPublicHTTP("public sync", path, pr); err != nil {
					return err
				}
				list, err := observatory.ParseCollectionsList(pr.Res.Body)
				if err != nil {
					return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
				}
				names = list.Names
				if !all {
					// default curated set when --all not set and no --collections
					want := map[string]bool{"Eggs": true, "Pets": true, "Zones": true, "Worlds": true, "ZoneFlags": true, "SecretRooms": true}
					filtered := names[:0]
					for _, n := range names {
						if want[n] {
							filtered = append(filtered, n)
						}
					}
					names = filtered
				}
			}
			saved := []string{}
			failures := []string{}
			for _, name := range names {
				path := "/api/collection/" + url.PathEscape(name)
				pr, err := app.getPublicCached(c.Context(), path, nil, api.BodyLimitDefault)
				if err != nil {
					failures = append(failures, name+": "+err.Error())
					continue
				}
				if err := app.mapPublicHTTP("public sync", path, pr); err != nil {
					failures = append(failures, name+": "+err.Error())
					continue
				}
				meta := observatory.CollectionSnapshotMeta{
					ID:         name + "-" + time.Now().UTC().Format("20060102T150405Z"),
					FetchedAt:  time.Now().UTC().Format(time.RFC3339Nano),
					OriginURL:  api.ProductionAPIBase + path,
					Collection: name,
				}
				if err := observatory.SaveCollection(dir, name, pr.Res.Body, meta); err != nil {
					failures = append(failures, name+": "+err.Error())
					continue
				}
				saved = append(saved, name)
			}
			_ = incremental
			payload := map[string]any{
				"catalogDir": dir, "saved": saved, "failures": failures,
				"accountQuotaSpent": false,
				"note":              "public catalog sync never attaches account bearer tokens",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			if err := w.Write(output.NewSuccess("public sync", nil, output.DefaultFreshnessNull(), payload)); err != nil {
				return err
			}
			if len(failures) > 0 && len(saved) == 0 {
				return &ExitError{Code: exitcode.Upstream, Msg: "public sync failed for all collections"}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "local catalog directory (default ./.bg-cli-catalog)")
	cmd.Flags().StringVar(&collections, "collections", "", "comma-separated collection names")
	cmd.Flags().BoolVar(&all, "all", false, "sync every name from /api/collections")
	cmd.Flags().BoolVar(&incremental, "incremental", true, "prefer cheap incremental sync (default)")
	return cmd
}

func newPublicCollectionsCmd(app *App) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "collections",
		Short: "List collections present in the local catalog store",
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				dir = filepath.Join(".", ".bg-cli-catalog")
			}
			entries, err := observatory.ListLocalCollections(dir)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("public collections", nil, output.DefaultFreshnessNull(), map[string]any{
				"catalogDir": dir, "collections": entries,
			}))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "local catalog directory")
	return cmd
}

func newPublicStatusCmd(app *App) *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Coverage manifest for the local catalog",
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				dir = filepath.Join(".", ".bg-cli-catalog")
			}
			man, err := observatory.BuildCoverageManifest(dir, []string{"Eggs", "Pets", "Zones", "Worlds", "ZoneFlags", "SecretRooms"})
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("public status", nil, output.DefaultFreshnessNull(), man))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "local catalog directory")
	return cmd
}

func newPublicInspectCmd(app *App) *cobra.Command {
	var dir string
	var raw bool
	cmd := &cobra.Command{
		Use:   "inspect COLLECTION [NAME]",
		Short: "Inspect a local catalog collection or one egg by display/config name",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				dir = filepath.Join(".", ".bg-cli-catalog")
			}
			col := args[0]
			stored, err := observatory.LoadCollection(dir, col)
			if err != nil {
				if b, rerr := os.ReadFile(col); rerr == nil {
					stored.Raw = b
					stored.Meta.Collection = filepath.Base(col)
				} else {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
			}
			if len(args) == 1 {
				payload := map[string]any{"meta": stored.Meta, "bytes": len(stored.Raw)}
				if raw {
					payload["raw"] = json.RawMessage(stored.Raw)
				}
				w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
				return w.Write(output.NewSuccess("public inspect", nil, output.DefaultFreshnessNull(), payload))
			}
			name := args[1]
			doc, err := observatory.FindEggByDisplayName(stored.Raw, name)
			if err != nil {
				return &ExitError{Code: exitcode.NotFound, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("public inspect", nil, output.DefaultFreshnessNull(), doc))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "local catalog directory")
	cmd.Flags().BoolVar(&raw, "raw", false, "include raw JSON")
	return cmd
}

func newPublicDiffCmd(app *App) *cobra.Command {
	var fromPath, toPath string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Semantic diff of two collection JSON snapshots",
		RunE: func(c *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			a, err := os.ReadFile(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			b, err := os.ReadFile(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			diff, err := observatory.DiffCollections(a, b)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("public diff", nil, output.DefaultFreshnessNull(), diff))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "older collection JSON")
	cmd.Flags().StringVar(&toPath, "to", "", "newer collection JSON")
	return cmd
}

func newPublicExportCmd(app *App) *cobra.Command {
	var dir, out, format string
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Export local catalog coverage + collection list",
		RunE: func(c *cobra.Command, args []string) error {
			if dir == "" {
				dir = filepath.Join(".", ".bg-cli-catalog")
			}
			entries, err := observatory.ListLocalCollections(dir)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			man, _ := observatory.BuildCoverageManifest(dir, nil)
			payload := map[string]any{"collections": entries, "coverage": man}
			if out != "" {
				b, _ := json.MarshalIndent(payload, "", "  ")
				if err := os.WriteFile(out, b, 0o644); err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
			}
			_ = format
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("public export", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&dir, "dir", "", "catalog directory")
	cmd.Flags().StringVar(&out, "output", "", "optional write path")
	cmd.Flags().StringVar(&format, "format", "json", "json")
	return cmd
}

func newEggCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "egg",
		Short: "Public egg inspect and hypothetical odds models",
	}
	cmd.AddCommand(newEggShowCmd(app))
	cmd.AddCommand(newEggOddsCmd(app))
	cmd.AddCommand(newEggFlagsCmd(app))
	cmd.AddCommand(newEggCostCmd(app))
	return cmd
}

func newEggShowCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "show NAME",
		Short: "Show parsed egg document from a local Eggs collection file",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if file == "" {
				file = filepath.Join("testdata", "fixtures", "observatory", "eggs-secret-room.json")
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			doc, err := observatory.FindEggByDisplayName(b, args[0])
			if err != nil {
				doc, err = observatory.ParseEggDocument(b)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("egg show", nil, output.DefaultFreshnessNull(), doc))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Eggs collection JSON (default Secret Room fixture)")
	return cmd
}

func newEggOddsCmd(app *App) *cobra.Command {
	var file, model string
	var rolls int
	cmd := &cobra.Command{
		Use:   "odds NAME",
		Short: "Hypothetical odds interpretations for an egg pets table",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if file == "" {
				file = filepath.Join("testdata", "fixtures", "observatory", "eggs-secret-room.json")
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			doc, err := observatory.FindEggByDisplayName(b, args[0])
			if err != nil {
				doc, err = observatory.ParseEggDocument(b)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
			}
			m := observatory.OddsModel(model)
			if m == "" {
				m = observatory.ModelNormalizedWeight
			}
			rep, err := observatory.ComputeEggOdds(doc, m)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			extra := map[string]any{"odds": rep}
			if rolls > 0 {
				lines := []map[string]any{}
				for _, pet := range rep.Lines {
					p := pet.EffectiveProb
					lines = append(lines, map[string]any{
						"petId": pet.PetID, "p": p, "oneInN": observatory.OneInN(p),
						"pAtLeastOne":         observatory.PAtLeastOne(p, float64(rolls)),
						"expected":            p * float64(rolls),
						"interpretation":      pet.Interpretation,
						"notVerifiedLiveOdds": pet.NotVerifiedLiveOdds,
					})
				}
				extra["rolls"] = rolls
				extra["perPet"] = lines
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("egg odds", nil, output.DefaultFreshnessNull(), extra))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Eggs JSON file")
	cmd.Flags().StringVar(&model, "model", "normalized-weight", "direct-percent|normalized-weight|independent-rare-gate")
	cmd.Flags().IntVar(&rolls, "rolls", 0, "optional trial count (e.g. 51724)")
	return cmd
}

func newEggFlagsCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "flags NAME",
		Short: "List named odds/feature flags on an egg without inventing magnitudes",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if file == "" {
				file = filepath.Join("testdata", "fixtures", "observatory", "eggs-secret-room.json")
			}
			b, err := os.ReadFile(file)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			doc, err := observatory.FindEggByDisplayName(b, args[0])
			if err != nil {
				doc, err = observatory.ParseEggDocument(b)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
			}
			payload := map[string]any{
				"egg": doc.DisplayName, "configName": doc.ConfigName,
				"oddsFFlags": doc.OddsFFlags,
				"note":       "named flags are unknown modifiers until public numeric evidence exists — magnitudes not invented",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("egg flags", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "Eggs JSON")
	return cmd
}

func newEggCostCmd(app *App) *cobra.Command {
	var balance, batchCost string
	var batchSize int
	cmd := &cobra.Command{
		Use:   "cost NAME",
		Short: "Nominal batch affordability from balance strings (not gross mining income)",
		Args:  cobra.ExactArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			if balance == "" || batchCost == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--balance and --batch-cost required"}
			}
			if batchSize <= 0 {
				batchSize = 67
			}
			bal, err1 := strconv.ParseFloat(balance, 64)
			cost, err2 := strconv.ParseFloat(batchCost, 64)
			payload := map[string]any{
				"egg": args[0], "balance": balance, "batchCost": batchCost, "batchSize": batchSize,
				"note": "net currency decline ≠ gross hatch spend; do not label residual as mining revenue",
			}
			if err1 == nil && err2 == nil && cost > 0 {
				payload["nominalBatches"] = bal / cost
				payload["nominalEggs"] = (bal / cost) * float64(batchSize)
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("egg cost", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&balance, "balance", "", "exact currency balance string")
	cmd.Flags().StringVar(&batchCost, "batch-cost", "", "displayed batch cost string")
	cmd.Flags().IntVar(&batchSize, "batch-size", 67, "eggs per purchase when evidenced")
	return cmd
}

func newGraphCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "graph", Short: "Public entity-graph unresolved joins"}
	cmd.AddCommand(&cobra.Command{
		Use:   "unresolved",
		Short: "Report unresolved / heuristic egg-zone joins",
		RunE: func(c *cobra.Command, args []string) error {
			edges := []observatory.Edge{
				{
					SourceCollection: "Eggs", SourceKey: "Secret Room Egg",
					FromCollection: "Eggs", FromKey: "Secret Room Egg",
					Relation: "placed_in", Evidence: observatory.EvidenceUnresolved,
					Notes: "SecretRooms mapping not assumed from egg name alone",
				},
			}
			rep := observatory.ReportUnresolved(edges)
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("graph unresolved", nil, output.DefaultFreshnessNull(), rep))
		},
	})
	return cmd
}
