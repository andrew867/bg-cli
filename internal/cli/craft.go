package cli

import (
	"encoding/json"
	"os"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/craft"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func newAccountCraftCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "craft",
		Short: "Offline Combine-o-Matic planning (NOT Halloween Forge)",
		Long: `Read-only Combine-o-Matic recipe BOM and planning. Distinct from account forge.
Never submits crafts. Protects valuable variants by default.`,
	}
	cmd.AddCommand(newAccountCraftRecipesCmd(app))
	cmd.AddCommand(newAccountCraftBOMCmd(app))
	cmd.AddCommand(newAccountCraftOptimizeCmd(app))
	cmd.AddCommand(&cobra.Command{
		Use:   "inventory",
		Short: "Describe stock JSON shape for craft optimize (offline helper)",
		RunE: func(c *cobra.Command, args []string) error {
			payload := map[string]any{
				"stockJsonExample": map[string]int{"Masquerade Agony": 100, "Spirit Shards": 500},
				"variantKeys":      "optional ItemID|Normal style keys; unprotected Normal preferred",
				"note":             "possession ≠ equipped; CLI never auto-crafts",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account craft inventory", nil, output.DefaultFreshnessNull(), payload))
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "ledger",
		Short: "Placeholder: record local craft attempts separately from Forge ledger",
		RunE: func(c *cobra.Command, args []string) error {
			payload := map[string]any{
				"methodId": "craft.combine-o-matic.ledger.v1",
				"note":     "use annotated item-flow / local files; Combine-o-Matic ledger is distinct from account forge ledger",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account craft ledger", nil, output.DefaultFreshnessNull(), payload))
		},
	})
	return cmd
}

func newAccountCraftRecipesCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "recipes",
		Short: "List observed Masquerade Combine-o-Matic recipes",
		RunE: func(c *cobra.Command, args []string) error {
			recipes := craft.DefaultMasqueradeRecipes()
			payload := map[string]any{
				"methodId": craft.MethodIDRecipes, "schemaVersion": craft.RecipeSchemaVersion,
				"recipes": recipes,
				"note":    "screenshot observations — not hardcoded production server rules",
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account craft recipes", nil, output.DefaultFreshnessNull(), payload))
		},
	}
}

func newAccountCraftBOMCmd(app *App) *cobra.Command {
	var target string
	var count int
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Expand recursive BOM for a Combine-o-Matic target",
		RunE: func(c *cobra.Command, args []string) error {
			if target == "" {
				target = "gargantuan-butler-penguin"
			}
			if count <= 0 {
				count = 1
			}
			bom, err := craft.ExpandBOM(craft.DefaultMasqueradeRecipes(), target, count)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account craft plan", nil, output.DefaultFreshnessNull(), bom))
		},
	}
	cmd.Flags().StringVar(&target, "target", "gargantuan-butler-penguin", "recipe id or output name")
	cmd.Flags().IntVar(&count, "count", 1, "number of target outputs")
	return cmd
}

func newAccountCraftOptimizeCmd(app *App) *cobra.Command {
	var target, objective, stockPath string
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Advisory craft plan from inventory stock JSON (never auto-crafts)",
		RunE: func(c *cobra.Command, args []string) error {
			stock := craft.InventoryStock{}
			if stockPath != "" {
				b, err := os.ReadFile(stockPath)
				if err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
				}
				if err := json.Unmarshal(b, &stock); err != nil {
					return &ExitError{Code: exitcode.Usage, Msg: "stock: " + err.Error()}
				}
			}
			obj := craft.OptimizeObjective(objective)
			if obj == "" {
				obj = craft.ObjEarliestFirstHuge
			}
			rep, err := craft.OptimizePlan(craft.OptimizeOptions{
				Objective: obj, TargetRecipe: target, Stock: stock,
			})
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account craft optimize", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&target, "target", "", "recipe id (default depends on objective)")
	cmd.Flags().StringVar(&objective, "objective", "earliest-first-huge", "earliest-first-huge|min-extra-agonies|min-extra-shards|max-progression")
	cmd.Flags().StringVar(&stockPath, "stock", "", "optional inventory stock JSON map")
	return cmd
}
