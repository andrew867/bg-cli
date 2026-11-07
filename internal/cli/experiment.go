package cli

import (
	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/loadout"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountExperimentCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "experiment",
		Short: "Controlled offline snapshot experiments for currency rates",
		Long: `Extends snapshot comparison with experiment metadata and hygiene checks.
Fully offline. Never labels net currency change as pure farming income.`,
	}
	cmd.AddCommand(newAccountExperimentCompareCmd(app))
	return cmd
}

func newAccountExperimentCompareCmd(app *App) *cobra.Command {
	var fromPath, toPath, currency, world, experimentID, activity, design, currencyCap string
	var pets, enchants, potions, confounders []string
	cmd := &cobra.Command{
		Use:   "compare",
		Short: "Compare two snapshot manifests for one currency under experiment metadata",
		RunE: func(cmd *cobra.Command, args []string) error {
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to are required"}
			}
			if currency == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--currency is required"}
			}
			fromB, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
			}
			toB, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
			}
			var fromL, toL *loadout.PlayerLoadout
			if fromB.Inventory != nil {
				l, err := loadout.ParseInventoryLoadout(fromB.Inventory)
				if err == nil {
					fromL = &l
				}
			}
			if toB.Inventory != nil {
				l, err := loadout.ParseInventoryLoadout(toB.Inventory)
				if err == nil {
					toL = &l
				}
			}
			rep, err := loadout.CompareExperiment(fromB, toB, loadout.ExperimentMeta{
				ExperimentID: experimentID, World: world, CurrencyID: currency,
				PetLoadout: pets, EnchantLoadout: enchants, PotionsBuffs: potions,
				Activity: activity, KnownConfounders: confounders, Design: design,
				CurrencyCap: currencyCap,
			}, fromL, toL)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account experiment compare", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "before snapshot manifest")
	cmd.Flags().StringVar(&toPath, "to", "", "after snapshot manifest")
	cmd.Flags().StringVar(&currency, "currency", "", "currency ID (required; never combined)")
	cmd.Flags().StringVar(&currencyCap, "currency-cap", "", "optional exact ceiling override for cap detection")
	cmd.Flags().StringVar(&world, "world", "halloween-games", "world/event id")
	cmd.Flags().StringVar(&experimentID, "id", "", "experiment id")
	cmd.Flags().StringVar(&activity, "activity", "", "activity performed")
	cmd.Flags().StringVar(&design, "design", "A/B", "A/B or A/B/A")
	cmd.Flags().StringSliceVar(&pets, "pets", nil, "declared pet loadout ids")
	cmd.Flags().StringSliceVar(&enchants, "enchants", nil, "declared enchant loadout ids")
	cmd.Flags().StringSliceVar(&potions, "buffs", nil, "known active potions/buffs")
	cmd.Flags().StringSliceVar(&confounders, "confounder", nil, "known confounders")
	return cmd
}
