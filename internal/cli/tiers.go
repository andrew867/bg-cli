package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/observatory"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountTiersCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tiers",
		Short: "Resolve inventory id+tn against public Enchants/Potions/Charms Tiers",
	}
	cmd.AddCommand(newAccountTiersResolveCmd(app))
	return cmd
}

func newAccountTiersResolveCmd(app *App) *cobra.Command {
	var inventoryPath, manifestPath, catalogDir string
	var enchantsPath, potionsPath, charmsPath, ultimatesPath string
	var livePublic bool
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Join owned/equipped tiered items to public Tiers[].DisplayName (offline-first)",
		Long: `Resolves inventory stacks with rawData.tn (and equipped.level when > 0) against
official public collection Tiers arrays. Never invents equipped loadouts from ownership.
Does not spend account refresh quota. Public fetches use GetPublic only.`,
		RunE: func(c *cobra.Command, args []string) error {
			invObj, snapPath, err := loadInventoryObject(inventoryPath, manifestPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			cat, sources, err := loadTierCatalogs(app, c, catalogDir, enchantsPath, potionsPath, charmsPath, ultimatesPath, livePublic)
			if err != nil {
				return err
			}
			rep, err := observatory.ResolveInventoryTiers(invObj, cat, snapPath)
			if err != nil {
				return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			payload := map[string]any{
				"report":            rep,
				"catalogSources":    sources,
				"accountQuotaSpent": false,
			}
			w := output.Writer{Out: app.stdout(), Pretty: app.Flags.Pretty, Clock: app.Deps.Clock}
			return w.Write(output.NewSuccess("account tiers resolve", nil, output.DefaultFreshnessNull(), payload))
		},
	}
	cmd.Flags().StringVar(&inventoryPath, "inventory", "", "inventory object JSON or account inventory envelope")
	cmd.Flags().StringVar(&manifestPath, "manifest", "", "snapshot manifest (loads companion inv.json offline)")
	cmd.Flags().StringVar(&catalogDir, "catalog-dir", "", "local observatory catalog dir with Enchants.json etc")
	cmd.Flags().StringVar(&enchantsPath, "enchants", "", "Enchants collection JSON file")
	cmd.Flags().StringVar(&potionsPath, "potions", "", "Potions collection JSON file")
	cmd.Flags().StringVar(&charmsPath, "charms", "", "Charms collection JSON file")
	cmd.Flags().StringVar(&ultimatesPath, "ultimates", "", "Ultimates collection JSON file")
	cmd.Flags().BoolVar(&livePublic, "live-public", false, "fetch missing collections from official /api/collection/* (no account token)")
	return cmd
}

func loadInventoryObject(inventoryPath, manifestPath string) (json.RawMessage, string, error) {
	if inventoryPath == "" && manifestPath == "" {
		return nil, "", errString("--inventory or --manifest is required (offline; will not invent equipped loadouts)")
	}
	if manifestPath != "" {
		b, err := snapshot.LoadBundle(manifestPath)
		if err != nil {
			return nil, "", err
		}
		if len(b.Inventory) == 0 {
			return nil, "", errString("manifest has no inventory-json payload")
		}
		return b.Inventory, manifestPath, nil
	}
	raw, err := os.ReadFile(inventoryPath)
	if err != nil {
		return nil, "", err
	}
	// Accept raw inventory object, {data:{...}}, or CLI envelope with data.inventory.
	if data, _, err := snapshot.EnvelopeData(raw); err == nil {
		if inv, ok := data["inventory"]; ok && len(inv) > 0 {
			return inv, inventoryPath, nil
		}
		if items, ok := data["items"]; ok {
			// envelope data IS the inventory object
			_ = items
			b, _ := json.Marshal(data)
			return b, inventoryPath, nil
		}
	}
	var probe map[string]json.RawMessage
	if json.Unmarshal(raw, &probe) == nil {
		if _, ok := probe["items"]; ok {
			return raw, inventoryPath, nil
		}
		if data, ok := probe["data"]; ok {
			var inner map[string]json.RawMessage
			if json.Unmarshal(data, &inner) == nil {
				if _, ok := inner["items"]; ok {
					return data, inventoryPath, nil
				}
				if inv, ok := inner["inventory"]; ok {
					return inv, inventoryPath, nil
				}
			}
		}
	}
	return nil, "", errString("could not find inventory items object in " + inventoryPath)
}

type errString string

func (e errString) Error() string { return string(e) }

func loadTierCatalogs(app *App, c *cobra.Command, catalogDir, enchants, potions, charms, ultimates string, live bool) (observatory.TierCatalogSet, []string, error) {
	type src struct {
		name string
		path string
	}
	want := []src{
		{"Enchants", enchants},
		{"Potions", potions},
		{"Charms", charms},
		{"Ultimates", ultimates},
	}
	var cats []observatory.TierCatalog
	var sources []string
	for _, w := range want {
		path := w.path
		if path == "" && catalogDir != "" {
			candidate := filepath.Join(catalogDir, w.name+".json")
			if _, err := os.Stat(candidate); err == nil {
				path = candidate
			}
		}
		var body []byte
		var err error
		if path != "" {
			body, err = os.ReadFile(path)
			if err != nil {
				return observatory.TierCatalogSet{}, nil, &ExitError{Code: exitcode.Usage, Msg: err.Error()}
			}
			sources = append(sources, path)
		} else if live {
			apiPath := "/api/collection/" + w.name
			pr, gerr := app.getPublicCached(c.Context(), apiPath, nil, api.BodyLimitDefault)
			if gerr != nil {
				return observatory.TierCatalogSet{}, nil, mapNet(app, "account tiers resolve", gerr)
			}
			if herr := app.mapPublicHTTP("account tiers resolve", apiPath, pr); herr != nil {
				return observatory.TierCatalogSet{}, nil, herr
			}
			body = pr.Res.Body
			sources = append(sources, api.ProductionAPIBase+apiPath)
		} else {
			continue
		}
		cat, perr := observatory.ParseTieredCollection(w.name, body)
		if perr != nil {
			return observatory.TierCatalogSet{}, nil, &ExitError{Code: exitcode.Upstream, Msg: w.name + ": " + perr.Error()}
		}
		cats = append(cats, cat)
	}
	if len(cats) == 0 {
		return observatory.TierCatalogSet{}, nil, &ExitError{Code: exitcode.Usage, Msg: "provide --enchants/--potions/--charms/--catalog-dir and/or --live-public"}
	}
	_ = strings.TrimSpace
	return observatory.NewTierCatalogSet(cats...), sources, nil
}
