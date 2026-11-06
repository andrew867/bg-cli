package cli

import (
	"context"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/loadout"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountLoadoutCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "loadout",
		Short: "Analyze and optimize equipped pets/enchants for a world and objective",
		Long: `Read-only loadout analysis. Prefers offline snapshot manifests; never invents
effective DPS or unverified currency multipliers. Currency channels stay separate.

When magnitudes are unknown, candidates include uncertainty and a suggested controlled test.`,
	}
	cmd.AddCommand(newAccountLoadoutAnalyzeCmd(app))
	cmd.AddCommand(newAccountLoadoutOptimizeCmd(app))
	cmd.AddCommand(newAccountLoadoutRecommendCmd(app))
	cmd.AddCommand(newAccountLoadoutCalibrateCmd(app))
	cmd.AddCommand(newAccountLoadoutObjectivesCmd(app))
	return cmd
}

func newAccountLoadoutRecommendCmd(app *App) *cobra.Command {
	var f loadoutCommonFlags
	var maxCand, eventTeamLimit int
	var annotationsPath, publicPets, publicCharms, publicIndex, snapshotPath string
	var hasTeamLimit bool
	cmd := &cobra.Command{
		Use:   "recommend",
		Short: "Recommend owned-only loadouts with evidence and uncertainty",
		RunE: func(cmd *cobra.Command, args []string) error {
			if snapshotPath != "" && f.manifest == "" {
				f.manifest = snapshotPath
			}
			obj, err := loadout.ParseObjective(f.objective)
			if err != nil {
				// allow aliases handled inside Recommend
				obj = loadout.Objective{ID: loadout.ObjectiveID(f.objective)}
			}
			pl, reg, err := app.resolveLoadoutInput(cmd.Context(), f)
			if err != nil {
				return err
			}
			ann, err := loadout.LoadAnnotations(annotationsPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			pub, err := loadPublicIndex(publicIndex, publicPets, publicCharms)
			if err != nil {
				return err
			}
			ro := loadout.RecommendOptions{
				OptimizeOptions: loadout.OptimizeOptions{
					WorldID: f.world, Objective: obj, CurrencyID: f.currency, MaxCandidates: maxCand,
				},
				Annotations: ann, Public: pub,
			}
			if hasTeamLimit {
				ro.EventTeamLimit = &eventTeamLimit
			}
			res, err := loadout.Recommend(reg, pl, ro)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account loadout recommend", nil, output.DefaultFreshnessNull(), res))
		},
	}
	bindLoadoutFlags(cmd, &f, "max-pet-damage")
	cmd.Flags().StringVar(&snapshotPath, "snapshot", "", "alias for --manifest")
	cmd.Flags().IntVar(&maxCand, "max-candidates", 5, "maximum candidate loadouts")
	cmd.Flags().IntVar(&eventTeamLimit, "event-team-limit", 0, "independent event pet team size (do not assume account maxEquipped)")
	cmd.Flags().StringVar(&annotationsPath, "annotations", "", "optional annotations JSON")
	cmd.Flags().StringVar(&publicPets, "pets-collection", "", "optional Pets collection JSON")
	cmd.Flags().StringVar(&publicCharms, "charms-collection", "", "optional Charms collection JSON")
	cmd.Flags().StringVar(&publicIndex, "public-index", "", "optional public config index JSON")
	_ = cmd.Flags().SetAnnotation("event-team-limit", "has", []string{"1"})
	cmd.PreRun = func(c *cobra.Command, args []string) {
		hasTeamLimit = c.Flags().Changed("event-team-limit")
	}
	return cmd
}

func newAccountLoadoutObjectivesCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "objectives",
		Short: "List supported optimization objectives",
		RunE: func(cmd *cobra.Command, args []string) error {
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account loadout objectives", nil, output.DefaultFreshnessNull(), map[string]any{
				"objectives": loadout.KnownObjectives(),
			}))
		},
	}
}

type loadoutCommonFlags struct {
	manifest  string
	registry  string
	world     string
	objective string
	currency  string
	refresh   bool
	yes       bool
}

func (a *App) resolveLoadoutInput(ctx context.Context, flags loadoutCommonFlags) (loadout.PlayerLoadout, loadout.Registry, error) {
	reg, err := loadout.LoadRegistryJSON(flags.registry)
	if err != nil {
		return loadout.PlayerLoadout{}, loadout.Registry{}, &ExitError{Code: exitcode.Usage, Msg: "registry: " + err.Error()}
	}
	if flags.manifest != "" {
		b, err := snapshot.LoadBundle(flags.manifest)
		if err != nil {
			return loadout.PlayerLoadout{}, reg, &ExitError{Code: exitcode.Usage, Msg: "manifest: " + err.Error()}
		}
		if b.Inventory == nil {
			return loadout.PlayerLoadout{}, reg, &ExitError{Code: exitcode.Usage, Msg: "snapshot missing inventory payload"}
		}
		pl, err := loadout.ParseInventoryLoadout(b.Inventory)
		if err != nil {
			return loadout.PlayerLoadout{}, reg, &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
		}
		return pl, reg, nil
	}
	af := accountFlags{refresh: flags.refresh, yes: flags.yes}
	if err := a.confirmRefresh(af, "account loadout"); err != nil {
		return loadout.PlayerLoadout{}, reg, err
	}
	_, tok, secrets, err := a.resolveProfileToken(api.ScopeInventory)
	if err != nil {
		return loadout.PlayerLoadout{}, reg, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetInventory(ctx, tok, api.AccountGetOptions{Refresh: flags.refresh})
	if err != nil {
		return loadout.PlayerLoadout{}, reg, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return loadout.PlayerLoadout{}, reg, err
	}
	pl, err := loadout.ParseInventoryLoadout(res.Data)
	if err != nil {
		return loadout.PlayerLoadout{}, reg, err
	}
	return pl, reg, nil
}

func newAccountLoadoutAnalyzeCmd(app *App) *cobra.Command {
	var f loadoutCommonFlags
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Model current loadout for a world/objective without inventing multipliers",
		RunE: func(cmd *cobra.Command, args []string) error {
			obj, err := loadout.ParseObjective(f.objective)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			pl, reg, err := app.resolveLoadoutInput(cmd.Context(), f)
			if err != nil {
				return err
			}
			est, err := loadout.AnalyzeLoadout(reg, pl, loadout.AnalyzeOptions{
				WorldID: f.world, Objective: obj, CurrencyID: f.currency,
			})
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			payload := map[string]any{
				"registryVersion": reg.Version,
				"currentLoadout":  pl,
				"estimate":        est,
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account loadout analyze", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	bindLoadoutFlags(cmd, &f, "max-pet-damage")
	return cmd
}

func newAccountLoadoutOptimizeCmd(app *App) *cobra.Command {
	var f loadoutCommonFlags
	var maxCand int
	cmd := &cobra.Command{
		Use:   "optimize",
		Short: "Propose owned-only loadout candidates with uncertainty and test plan",
		RunE: func(cmd *cobra.Command, args []string) error {
			obj, err := loadout.ParseObjective(f.objective)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			pl, reg, err := app.resolveLoadoutInput(cmd.Context(), f)
			if err != nil {
				return err
			}
			res, err := loadout.Optimize(reg, pl, loadout.OptimizeOptions{
				WorldID: f.world, Objective: obj, CurrencyID: f.currency, MaxCandidates: maxCand,
			})
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account loadout optimize", nil, output.DefaultFreshnessNull(), res))
		},
	}
	bindLoadoutFlags(cmd, &f, "halloween-coins-per-min")
	cmd.Flags().IntVar(&maxCand, "max-candidates", 5, "maximum candidate loadouts to return")
	return cmd
}

func newAccountLoadoutCalibrateCmd(app *App) *cobra.Command {
	var fromPath, toPath, registry, world, objective, currency, note string
	cmd := &cobra.Command{
		Use:   "calibrate",
		Short: "Measure currency rates from two offline snapshots (no invented multipliers)",
		Long: `Fully offline. Records observed per-minute rates, changed variables, freshness,
confidence, and confounders. Does not auto-write registry magnitudes.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			obj, err := loadout.ParseObjective(objective)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			if fromPath == "" || toPath == "" {
				return &ExitError{Code: exitcode.Usage, Msg: "--from and --to manifests are required"}
			}
			fromB, err := snapshot.LoadBundle(fromPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "from: " + err.Error()}
			}
			toB, err := snapshot.LoadBundle(toPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "to: " + err.Error()}
			}
			if _, err := loadout.LoadRegistryJSON(registry); err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: "registry: " + err.Error()}
			}
			rep, err := loadout.CalibrateFromSnapshots(loadout.CalibrationInput{
				From: fromB, To: toB, CurrencyID: currency, Objective: obj, WorldID: world, Note: note,
			})
			if err != nil {
				return &ExitError{Code: exitcode.Upstream, Msg: err.Error()}
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account loadout calibrate", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&fromPath, "from", "", "older snapshot manifest")
	cmd.Flags().StringVar(&toPath, "to", "", "newer snapshot manifest")
	cmd.Flags().StringVar(&registry, "registry", "", "optional effect registry JSON (default built-in)")
	cmd.Flags().StringVar(&world, "world", "halloween-games", "world/event id")
	cmd.Flags().StringVar(&objective, "objective", "halloween-coins-per-min", "objective id")
	cmd.Flags().StringVar(&currency, "currency", "", "currency id override")
	cmd.Flags().StringVar(&note, "note", "", "experiment note recorded in the calibration report")
	return cmd
}

func bindLoadoutFlags(cmd *cobra.Command, f *loadoutCommonFlags, defaultObj string) {
	cmd.Flags().StringVar(&f.manifest, "manifest", "", "offline snapshot manifest path (preferred; no network)")
	cmd.Flags().StringVar(&f.registry, "registry", "", "optional effect registry JSON (default built-in)")
	cmd.Flags().StringVar(&f.world, "world", "general", "world/event id (e.g. general, halloween-games)")
	cmd.Flags().StringVar(&f.objective, "objective", defaultObj, "optimization objective id")
	cmd.Flags().StringVar(&f.currency, "currency", "", "currency id override for currency objectives")
	cmd.Flags().BoolVar(&f.refresh, "refresh", false, "when not using --manifest, request inventory refresh=true")
	cmd.Flags().BoolVar(&f.yes, "yes", false, "confirm --refresh when non-interactive")
}
