package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/forge"
	"github.com/andrew867/bg-cli/internal/loadout"
	"github.com/andrew867/bg-cli/internal/output"
	snappkg "github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountForgeCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "forge",
		Short: "Offline Halloween Forge probability analysis and recipe optimization",
		Long: `Read-only Forge tooling. Ingest UI probability observations, fit displayed
curves without inventing RNG, and optimize recipes from inventory snapshots.

Never automates forging or mutates accounts. All calculations are offline.`,
	}
	cmd.AddCommand(newAccountForgeObserveCmd(app))
	cmd.AddCommand(newAccountForgeAnalyzeCmd(app))
	cmd.AddCommand(newAccountForgeOptimizeCmd(app))
	cmd.AddCommand(newAccountForgeLedgerCmd(app))
	return cmd
}

func newAccountForgeObserveCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "observe",
		Short: "Validate and summarize a Forge observation file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--file is required"}
			}
			f, err := forge.LoadObservations(file)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			summary := map[string]any{
				"schemaVersion":    f.SchemaVersion,
				"observationCount": len(f.Observations),
				"notes":            f.Notes,
				"byRecipeKind":     map[string]int{},
			}
			by := summary["byRecipeKind"].(map[string]int)
			for _, o := range f.Observations {
				by[o.RecipeKind]++
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account forge observe", nil, output.DefaultFreshnessNull(), summary))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to forge observations JSON")
	return cmd
}

func newAccountForgeAnalyzeCmd(app *App) *cobra.Command {
	var observations string
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Fit displayed Forge probability curves from comparable observations",
		RunE: func(cmd *cobra.Command, args []string) error {
			if observations == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--observations is required"}
			}
			f, err := forge.LoadObservations(observations)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rep := forge.AnalyzeObservations(f)
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account forge analyze", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&observations, "observations", "", "path to forge observations JSON")
	return cmd
}

func newAccountForgeOptimizeCmd(app *App) *cobra.Command {
	var observations, snapshotPath, weightsPath, objective, mode string
	var butlerBudget, maxButlers, shardBudget, minButlers int
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Recommend Forge recipes from fitted displayed probabilities + inventory",
		RunE: func(cmd *cobra.Command, args []string) error {
			if observations == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--observations is required"}
			}
			f, err := forge.LoadObservations(observations)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			if mode == "" {
				mode = "butler-only"
			}
			if mode != "butler-only" && mode != "observed-mixed" {
				return &ExitError{Code: exitcode.Usage, Msg: "--mode must be butler-only or observed-mixed"}
			}
			weights := forge.DefaultButlerWeights()
			if weightsPath != "" {
				b, err := os.ReadFile(weightsPath)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
				if err := json.Unmarshal(b, &weights); err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: "weights: " + err.Error()}
				}
			}
			inv := forge.InventoryAvailability{}
			if snapshotPath != "" {
				b, err := snappkg.LoadBundle(snapshotPath)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: "snapshot: " + err.Error()}
				}
				if b.Inventory != nil {
					pl, err := loadout.ParseInventoryLoadout(b.Inventory)
					if err != nil {
						return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
					}
					for _, s := range pl.OwnedPets {
						key := s.ID
						if s.Variant != "" && s.Variant != "Regular" {
							key = s.ID + "|" + s.Variant
						}
						ei := snappkg.ParseExactIntString(s.Count)
						n := 0
						if ei.ParseOK && ei.Int != nil && ei.Int.IsInt64() {
							n = int(ei.Int.Int64())
						}
						inv[key] += n
						if key != s.ID {
							inv[s.ID] += n
						}
					}
				}
			}
			obj := forge.ObjectiveID(objective)
			if obj == "" {
				if mode == "observed-mixed" {
					obj = forge.ObjMinShards
				} else {
					obj = forge.ObjMinButlers
				}
			}
			var rep forge.OptimizeReport
			switch mode {
			case "observed-mixed":
				rep, err = forge.OptimizeObservedMixed(f, forge.ObservedMixedOptions{
					Objective: obj, Weights: weights, Inventory: inv,
					ShardBudget: shardBudget, MinButlers: minButlers, ProtectVariants: true,
				})
			default:
				an := forge.AnalyzeObservations(f)
				var model *forge.CappedLinearModel
				for _, g := range an.Groups {
					if g.Model != nil && (model == nil || g.Model.NPoints > model.NPoints) {
						model = g.Model
					}
				}
				if model == nil {
					return &ExitError{Code: exitcode.Usage, Msg: "no fitted butler-only model available from observations"}
				}
				if maxButlers <= 0 {
					maxButlers = 30
				}
				rep, err = forge.OptimizeButlerOnly(forge.OptimizeOptions{
					Objective: obj, Weights: weights, Inventory: inv,
					ButlerBudget: butlerBudget, Model: *model,
					ShardsPerAttempt: 100, MaxButlers: maxButlers, MinButlers: 1, ProtectVariants: true,
				})
			}
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account forge optimize", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&observations, "observations", "", "forge observations JSON")
	cmd.Flags().StringVar(&snapshotPath, "snapshot", "", "optional snapshot manifest for inventory availability")
	cmd.Flags().StringVar(&weightsPath, "weights", "", "optional resource weights JSON")
	cmd.Flags().StringVar(&mode, "mode", "butler-only", "butler-only|observed-mixed")
	cmd.Flags().StringVar(&objective, "objective", "", "min-butlers|min-shards|max-success|max-success-budget|weighted-cost")
	cmd.Flags().IntVar(&butlerBudget, "butler-budget", 0, "budget for max-success-budget (butler-only)")
	cmd.Flags().IntVar(&shardBudget, "shard-budget", 0, "max spirit shards per attempt (observed-mixed)")
	cmd.Flags().IntVar(&minButlers, "min-butlers", 0, "minimum butlers required (observed-mixed)")
	cmd.Flags().IntVar(&maxButlers, "max-butlers", 30, "search upper bound (butler-only)")
	return cmd
}

func newAccountForgeLedgerCmd(app *App) *cobra.Command {
	var file string
	cmd := &cobra.Command{
		Use:   "ledger",
		Short: "Summarize a local Forge attempt ledger (no telemetry)",
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--file is required"}
			}
			f, err := forge.LoadLedger(file)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			sum := forge.SummarizeLedger(f)
			payload := map[string]any{"ledger": f, "frequency": sum}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account forge ledger", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&file, "file", "", "path to forge attempts ledger JSON")
	return cmd
}
