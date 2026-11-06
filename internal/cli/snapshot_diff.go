package cli

import (
	"encoding/csv"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountSnapshotDiffCmd(app *App) *cobra.Command {
	var fromPath, toPath, egg, pet, eggPrice, currency string
	var futureEggs, batchSize int64
	cmd := &cobra.Command{
		Use:   "diff",
		Short: "Compare two offline snapshot manifests (no network)",
		Long: `Diff two account snapshot manifests and their payload JSON files completely offline.

Never authenticates, never contacts BIG Games, and never consumes refresh quota.

Optional --egg / --pet enable observed hatch-rate analysis under an explicitly
user-specified association (not inferred causal metadata).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to manifests are required"}
			}
			fromBundle, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "load --from: " + err.Error()}
			}
			toBundle, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "load --to: " + err.Error()}
			}
			diff, err := snapshot.DiffBundles(fromBundle, toBundle, snapshot.DiffOptions{
				EggID:      egg,
				PetID:      pet,
				EggPrice:   eggPrice,
				CurrencyID: currency,
				FutureEggs: futureEggs,
				BatchSize:  batchSize,
			})
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			diff.FromManifest = filepath.Base(fromPath)
			diff.ToManifest = filepath.Base(toPath)

			switch app.Flags.Output {
			case "csv":
				return writeDiffCSV(app, diff)
			case "table":
				return writeDiffTable(app, diff)
			case "raw":
				return &ExitError{Code: exitcode.Usage, Msg: "account snapshot diff does not support --output raw"}
			default:
				w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
				return w.Write(output.NewSuccess("account snapshot diff", nil, output.DefaultFreshnessNull(), diff))
			}
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "path to older snapshot manifest JSON")
	cmd.Flags().StringVar(&toPath, "to", "", "path to newer snapshot manifest JSON")
	cmd.Flags().StringVar(&egg, "egg", "", "egg id for purchase/hatch-rate analysis (user-specified association)")
	cmd.Flags().StringVar(&pet, "pet", "", "pet id for hatch/inventory focus")
	cmd.Flags().StringVar(&eggPrice, "egg-price", "", "explicit egg price in currency units (exact integer text)")
	cmd.Flags().StringVar(&currency, "currency", "", "currency id for spend/balance analysis")
	cmd.Flags().Int64Var(&futureEggs, "future-eggs", 0, "N for expected pets / P(at least one) under observed-rate model")
	cmd.Flags().Int64Var(&batchSize, "batch-size", 0, "explicit eggs per batch for timing estimates")
	return cmd
}

func writeDiffTable(app *App, diff snapshot.DiffResult) error {
	w := app.stdout()
	fmt.Fprintf(w, "command\taccount snapshot diff\n")
	fmt.Fprintf(w, "from\t%s\n", diff.FromManifest)
	fmt.Fprintf(w, "to\t%s\n", diff.ToManifest)
	fmt.Fprintf(w, "reconciliation\t%s\n", diff.Reconciliation.Status)
	if diff.HatchRate != nil && diff.HatchRate.ObservedPercent != nil {
		fmt.Fprintf(w, "observedPetPercent\t%.6f\n", *diff.HatchRate.ObservedPercent)
	}
	fmt.Fprintln(w, "---")
	fmt.Fprintln(w, "section\tid\tprevious\tcurrent\tdelta")
	for _, c := range diff.Statistics {
		fmt.Fprintf(w, "statistics\t%s\t%s\t%s\t%s\n", c.ID, c.Previous, c.Current, c.Delta)
	}
	for _, c := range diff.PurchasedEggs {
		fmt.Fprintf(w, "purchasedEggs\t%s\t%s\t%s\t%s\n", c.ID, c.Previous, c.Current, c.Delta)
	}
	for _, c := range diff.PetHatchCounts {
		fmt.Fprintf(w, "petHatchCounts\t%s\t%s\t%s\t%s\n", c.ID, c.Previous, c.Current, c.Delta)
	}
	for _, c := range diff.InventoryByPet {
		fmt.Fprintf(w, "inventoryByPet\t%s\t%s\t%s\t%s\n", c.ID, c.Previous, c.Current, c.Delta)
	}
	for _, c := range diff.Currencies {
		fmt.Fprintf(w, "currencies\t%s\t%s\t%s\t%s\n", c.ID, c.Previous, c.Current, c.Delta)
	}
	for _, wmsg := range diff.Warnings {
		fmt.Fprintf(w, "warning\t%s\n", wmsg)
	}
	return nil
}

func writeDiffCSV(app *App, diff snapshot.DiffResult) error {
	cw := csv.NewWriter(app.stdout())
	_ = cw.Write([]string{"section", "id", "variant", "previous", "current", "delta", "negative"})
	write := func(section string, rows []snapshot.CounterDelta) {
		for _, c := range rows {
			neg := "false"
			if c.Negative {
				neg = "true"
			}
			_ = cw.Write([]string{section, c.ID, "", c.Previous, c.Current, c.Delta, neg})
		}
	}
	write("statistics", diff.Statistics)
	write("purchasedEggs", diff.PurchasedEggs)
	write("petHatchCounts", diff.PetHatchCounts)
	write("inventoryByPet", diff.InventoryByPet)
	write("currencies", diff.Currencies)
	for _, row := range diff.InventoryByVariant {
		id, _ := row["id"].(string)
		variant, _ := row["variant"].(string)
		prev, _ := row["previous"].(string)
		cur, _ := row["current"].(string)
		delta, _ := row["delta"].(string)
		neg := "false"
		if b, ok := row["negative"].(bool); ok && b {
			neg = "true"
		}
		_ = cw.Write([]string{"inventoryByVariant", id, variant, prev, cur, delta, neg})
	}
	cw.Flush()
	return cw.Error()
}
