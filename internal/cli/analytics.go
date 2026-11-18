package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/accountanalytics"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountIndexCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "index",
		Short: "Offline ItemIndex collection analytics",
	}
	cmd.AddCommand(newAccountIndexAnalyzeCmd(app))
	cmd.AddCommand(newAccountIndexDiffCmd(app))
	cmd.AddCommand(newAccountIndexCompletionCmd(app))
	return cmd
}

func newAccountIndexAnalyzeCmd(app *App) *cobra.Command {
	var snap string
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Summarize discovered pets/variants from a snapshot itemIndex view",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot is required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rep, err := accountanalytics.AnalyzeIndex(b)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account index analyze", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot manifest including itemIndex")
	return cmd
}

func newAccountIndexDiffCmd(app *App) *cobra.Command {
	var fromPath, toPath string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff ItemIndex discoveries between two snapshots",
		RunE: func(c *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			fb, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
			}
			tb, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
			}
			fa, err := accountanalytics.AnalyzeIndex(fb)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: "from: " + err.Error()}
			}
			ta, err := accountanalytics.AnalyzeIndex(tb)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: "to: " + err.Error()}
			}
			diff := accountanalytics.DiffIndexes(fa.Index, ta.Index)
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account index diff", nil, output.DefaultFreshnessNull(), diff))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "older snapshot manifest")
	cmd.Flags().StringVar(&toPath, "to", "", "newer snapshot manifest")
	return cmd
}

func newAccountIndexCompletionCmd(app *App) *cobra.Command {
	var snap, catalog, collection string
	cmd := &cobra.Command{
		Use:   "completion",
		Short: "Scoped completion vs an explicit public Pets catalog denominator",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" || catalog == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot and --catalog are required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			an, err := accountanalytics.AnalyzeIndex(b)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			raw, err := os.ReadFile(catalog)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			pets, err := parseCatalogPets(raw)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "catalog: " + err.Error()}
			}
			rep := accountanalytics.CompletionReport(an.Index, pets)
			rep.Collection = collection
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account index completion", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot with itemIndex")
	cmd.Flags().StringVar(&catalog, "catalog", "", "public Pets collection JSON file")
	cmd.Flags().StringVar(&collection, "collection", "Pets", "catalog label")
	return cmd
}

func parseCatalogPets(raw []byte) ([]accountanalytics.CatalogPet, error) {
	// Accept either {configData:{...}} map or [{configName/name}] array.
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err == nil {
		if cd, ok := top["configData"]; ok {
			var m map[string]json.RawMessage
			if json.Unmarshal(cd, &m) == nil {
				out := make([]accountanalytics.CatalogPet, 0, len(m))
				for id := range m {
					out = append(out, accountanalytics.CatalogPet{ID: id})
				}
				return out, nil
			}
		}
		if data, ok := top["data"]; ok {
			return parseCatalogPets(data)
		}
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err != nil {
		return nil, err
	}
	out := make([]accountanalytics.CatalogPet, 0, len(arr))
	for _, row := range arr {
		id, _ := row["configName"].(string)
		if id == "" {
			id, _ = row["name"].(string)
		}
		if id == "" {
			id, _ = row["id"].(string)
		}
		if id != "" {
			out = append(out, accountanalytics.CatalogPet{ID: id})
		}
	}
	return out, nil
}

func newAccountTransactionsCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "transactions", Short: "Offline trade/booth/mail item-flow analytics"}
	cmd.AddCommand(newAccountTransactionsReportCmd(app))
	cmd.AddCommand(newAccountTransactionsLedgerCmd(app))
	return cmd
}

func newAccountTransactionsReportCmd(app *App) *cobra.Command {
	var snap string
	var redactCP bool
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Normalize trade/booth/mail movements from a multi-view snapshot",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot is required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rep, err := accountanalytics.AnalyzeTransactions(b, redactCP)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account transactions report", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot with trades/booth/mail views")
	cmd.Flags().BoolVar(&redactCP, "redact-counterparties", false, "omit counterparty identifiers in output")
	return cmd
}

func newAccountTransactionsLedgerCmd(app *App) *cobra.Command {
	var snap string
	var redactCP bool
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "List normalized movements (same report, ledger-oriented)",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot is required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			mov, err := accountanalytics.ParseMovements(b)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			sum := accountanalytics.SummarizeMovements(mov, redactCP)
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account transactions ledger", nil, output.DefaultFreshnessNull(), sum))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot manifest")
	cmd.Flags().BoolVar(&redactCP, "redact-counterparties", true, "omit counterparty identifiers")
	return cmd
}

func newAccountHatchCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "hatch", Short: "Offline complete hatch distribution analysis"}
	cmd.AddCommand(newAccountHatchReportCmd(app))
	return cmd
}

func newAccountHatchReportCmd(app *App) *cobra.Command {
	var fromPath, toPath, egg string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Discover all pet hatch deltas and observed frequencies between snapshots",
		RunE: func(c *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			fb, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
			}
			tb, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
			}
			rep, err := accountanalytics.HatchReport(fb, tb, egg)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account hatch report", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "before snapshot manifest")
	cmd.Flags().StringVar(&toPath, "to", "", "after snapshot manifest")
	cmd.Flags().StringVar(&egg, "egg", "", "optional egg ID for purchase attribution")
	return cmd
}

func newAccountProgressionCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "progression", Short: "Offline rank/goals/mastery progression analytics"}
	cmd.AddCommand(newAccountProgressionDiffCmd(app))
	cmd.AddCommand(newAccountProgressionReportCmd(app))
	return cmd
}

func newAccountProgressionDiffCmd(app *App) *cobra.Command {
	var fromPath, toPath string
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Diff progression fields between two snapshot profiles",
		RunE: func(c *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			fb, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
			}
			tb, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
			}
			rep, err := accountanalytics.DiffProgressionBundles(fb, tb)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account progression diff", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "before snapshot")
	cmd.Flags().StringVar(&toPath, "to", "", "after snapshot")
	return cmd
}

func newAccountProgressionReportCmd(app *App) *cobra.Command {
	var snap string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Summarize progression fields present in one snapshot (no invented curves)",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot is required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			// Self-diff against empty yields presence-oriented report via Diff with identical → use DiffProgression with same data as "summary"
			rep, err := accountanalytics.DiffProgressionBundles(b, b)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			payload := map[string]any{"snapshot": snap, "fieldsPresent": rep, "note": "single-snapshot report; use progression diff for changes"}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account progression report", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot manifest")
	return cmd
}

func newAccountPortfolioCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{Use: "portfolio", Short: "Offline RAP portfolio estimates (not sale proceeds)"}
	cmd.AddCommand(newAccountPortfolioReportCmd(app))
	return cmd
}

func newAccountPortfolioReportCmd(app *App) *cobra.Command {
	var snap, rapPath string
	cmd := &cobra.Command{
		Use:   "report",
		Short: "Matched RAP estimate with coverage; missing RAP excluded from totals",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" || rapPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot and --rap are required"}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rap, err := os.ReadFile(rapPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rep, err := accountanalytics.BuildPortfolioReport(b.Inventory, rap)
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account portfolio report", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot manifest")
	cmd.Flags().StringVar(&rapPath, "rap", "", "public /api/rap JSON payload file")
	return cmd
}

func newAccountReconcileCmd(app *App) *cobra.Command {
	var fromPath, toPath, snap string
	cmd := &cobra.Command{
		Use:   "reconcile",
		Short: "Cross-source evidence checks across snapshots",
		RunE: func(c *cobra.Command, args []string) error {
			var fromB, toB *snapshot.Bundle
			var err error
			if snap != "" && fromPath == "" && toPath == "" {
				fromB, err = snapshot.LoadBundle(snap)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
				toB = fromB
			} else {
				if fromPath == "" || toPath == "" {
					return &ExitError{Code: exitcode.Usage, Msg: "--from/--to or --snapshot required"}
				}
				fromB, err = snapshot.LoadBundle(fromPath)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
				}
				toB, err = snapshot.LoadBundle(toPath)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
				}
			}
			checks := accountanalytics.RunReconciliation(fromB, toB, accountanalytics.ReconcileOptions{})
			payload := map[string]any{"checks": checks, "methodId": "account-reconcile.v1"}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account reconcile", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "before snapshot")
	cmd.Flags().StringVar(&toPath, "to", "", "after snapshot")
	cmd.Flags().StringVar(&snap, "snapshot", "", "single snapshot for integrity-oriented checks")
	return cmd
}

func newAccountAuditCmd(app *App) *cobra.Command {
	var snap string
	cmd := &cobra.Command{
		Use:   "audit",
		Short: "Manifest integrity + reconciliation summary for one snapshot",
		RunE: func(c *cobra.Command, args []string) error {
			if snap == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--snapshot is required"}
			}
			integ, err := snapshot.ValidateIntegrity(snap, false)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			b, err := snapshot.LoadBundle(snap)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			checks := accountanalytics.RunReconciliation(b, b, accountanalytics.ReconcileOptions{})
			payload := map[string]any{"integrity": integ, "checks": checks, "methodId": "account-audit.v1"}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			if err := w.Write(output.NewSuccess("account audit", nil, output.DefaultFreshnessNull(), payload)); err != nil {
				return err
			}
			if !integ.OK {
				return &ExitError{Code: exitcode.Upstream, Msg: "audit integrity failed"}
			}
			return nil
		},
	}
	cmd.Flags().StringVar(&snap, "snapshot", "", "snapshot manifest")
	return cmd
}
