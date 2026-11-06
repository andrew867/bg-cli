package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/loadout"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountPowerCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "power",
		Short: "Inspect layered pet power models by world/event",
		Long: `Evidence-based pet power inspection. Prefers offline --snapshot manifests.
Never treats displayed power as post-boost DPS. Unknown multipliers stay unknown.`,
	}
	cmd.AddCommand(newAccountPowerInspectCmd(app))
	return cmd
}

func newAccountPowerInspectCmd(app *App) *cobra.Command {
	var snapshotPath, annotationsPath, publicPets, publicCharms, publicIndex, world string
	var refresh, yes bool
	var bestEventRel float64
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "Layered power breakdown for equipped pets in a world context",
		RunE: func(cmd *cobra.Command, args []string) error {
			pl, pub, err := app.loadLoadoutAndPublic(cmd, snapshotPath, publicPets, publicCharms, publicIndex, refresh, yes)
			if err != nil {
				return err
			}
			ann, err := loadout.LoadAnnotations(annotationsPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			rep := loadout.InspectPower(pl, loadout.InspectOptions{
				WorldID: world, Annotations: ann, Public: pub, BestEventRel: bestEventRel,
				EventTeamLimit: ann.EventTeamLimit,
			})
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account power inspect", nil, output.DefaultFreshnessNull(), rep))
		},
	}
	cmd.Flags().StringVar(&snapshotPath, "snapshot", "", "offline snapshot manifest path")
	cmd.Flags().StringVar(&annotationsPath, "annotations", "", "optional local pet level/charm annotations JSON")
	cmd.Flags().StringVar(&publicPets, "pets-collection", "", "optional cached /api/collection/Pets JSON")
	cmd.Flags().StringVar(&publicCharms, "charms-collection", "", "optional cached /api/collection/Charms JSON")
	cmd.Flags().StringVar(&publicIndex, "public-index", "", "optional prebuilt public config index JSON")
	cmd.Flags().StringVar(&world, "world", "halloween-games", "world/event context id")
	cmd.Flags().Float64Var(&bestEventRel, "best-event-relative", 1, "relative scale of best event pet for Halloween model predictions")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "live inventory refresh=true when not using --snapshot")
	cmd.Flags().BoolVar(&yes, "yes", false, "confirm --refresh when non-interactive")
	return cmd
}

func (a *App) loadLoadoutAndPublic(cmd *cobra.Command, snapshotPath, petsPath, charmsPath, indexPath string, refresh, yes bool) (loadout.PlayerLoadout, loadout.PublicConfigIndex, error) {
	pub, err := loadPublicIndex(indexPath, petsPath, charmsPath)
	if err != nil {
		return loadout.PlayerLoadout{}, pub, err
	}
	if snapshotPath != "" {
		b, err := snapshot.LoadBundle(snapshotPath)
		if err != nil {
			return loadout.PlayerLoadout{}, pub, &ExitError{Code: exitcode.Usage, Msg: "snapshot: " + err.Error()}
		}
		if b.Inventory == nil {
			return loadout.PlayerLoadout{}, pub, &ExitError{Code: exitcode.Usage, Msg: "snapshot missing inventory"}
		}
		pl, err := loadout.ParseInventoryLoadout(b.Inventory)
		return pl, pub, err
	}
	af := accountFlags{refresh: refresh, yes: yes}
	if err := a.confirmRefresh(af, "account power"); err != nil {
		return loadout.PlayerLoadout{}, pub, err
	}
	_, tok, secrets, err := a.resolveProfileToken(api.ScopeInventory)
	if err != nil {
		return loadout.PlayerLoadout{}, pub, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetInventory(cmd.Context(), tok, api.AccountGetOptions{Refresh: refresh})
	if err != nil {
		return loadout.PlayerLoadout{}, pub, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return loadout.PlayerLoadout{}, pub, err
	}
	pl, err := loadout.ParseInventoryLoadout(res.Data)
	return pl, pub, err
}

func loadPublicIndex(indexPath, petsPath, charmsPath string) (loadout.PublicConfigIndex, error) {
	if indexPath != "" {
		pub, err := loadout.LoadPublicConfigIndexFile(indexPath)
		if err != nil {
			return pub, &ExitError{Code: exitcode.Usage, Msg: "public-index: " + err.Error()}
		}
		return pub, nil
	}
	if petsPath == "" && charmsPath == "" {
		return loadout.PublicConfigIndex{}, nil
	}
	var pets []loadout.PetConfig
	var charms []loadout.CharmConfig
	var pm, cm loadout.CollectionCacheMeta
	if petsPath != "" {
		b, err := os.ReadFile(petsPath)
		if err != nil {
			return loadout.PublicConfigIndex{}, &ExitError{Code: exitcode.Usage, Msg: err.Error()}
		}
		var err2 error
		pets, pm, err2 = loadout.ParsePetsCollection(b)
		if err2 != nil {
			return loadout.PublicConfigIndex{}, &ExitError{Code: exitcode.Usage, Msg: err2.Error()}
		}
		pm.URL = "file://" + petsPath
	}
	if charmsPath != "" {
		b, err := os.ReadFile(charmsPath)
		if err != nil {
			return loadout.PublicConfigIndex{}, &ExitError{Code: exitcode.Usage, Msg: err.Error()}
		}
		var err2 error
		charms, cm, err2 = loadout.ParseCharmsCollection(b)
		if err2 != nil {
			return loadout.PublicConfigIndex{}, &ExitError{Code: exitcode.Usage, Msg: err2.Error()}
		}
		cm.URL = "file://" + charmsPath
	}
	return loadout.BuildPublicConfigIndex(pets, pm, charms, cm), nil
}
