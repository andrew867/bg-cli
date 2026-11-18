package observatory

import (
	"encoding/json"
	"fmt"
	"strings"
)

// TieredItemRow is one owned or equipped tiered stack after public join.
type TieredItemRow struct {
	Role                string `json:"role"` // owned | equipped
	Class               string `json:"class"`
	ID                  string `json:"id"`
	TN                  *int   `json:"tn,omitempty"`
	TNSource            string `json:"tnSource,omitempty"` // rawData.tn | equipped.level | top-level tier
	Count               string `json:"count,omitempty"`
	Slot                string `json:"slot,omitempty"`
	APIDisplayName      string `json:"apiDisplayName,omitempty"`
	ResolvedDisplayName string `json:"resolvedDisplayName,omitempty"`
	ResolvedOK          bool   `json:"resolvedOk"`
	ResolveReason       string `json:"resolveReason"`
	PublicConfigName    string `json:"publicConfigName,omitempty"`
	PublicCollection    string `json:"publicCollection,omitempty"`
	EvidenceSnapshot    string `json:"evidenceSnapshot,omitempty"`
	EvidencePublic      string `json:"evidencePublic,omitempty"`
}

// InventoryTiersReport is owned vs equipped tier resolution.
type InventoryTiersReport struct {
	SnapshotPath string          `json:"snapshotPath,omitempty"`
	SnapshotAt   *string         `json:"snapshotAt,omitempty"`
	Owned        []TieredItemRow `json:"owned"`
	Equipped     []TieredItemRow `json:"equipped"`
	Notes        []string        `json:"notes"`
}

var tieredClasses = map[string]string{
	"enchant": "Enchants", "enchant book": "Enchants",
	"potion":     "Potions",
	"charm":      "Charms",
	"ultimate":   "Ultimates",
	"consumable": "Consumables",
}

// ResolveInventoryTiers joins inventory items (id + tn) and equipped slots against public catalogs.
// Ownership is never treated as equipped. Equipped potions/buffs are not invented when absent.
func ResolveInventoryTiers(inventoryObj json.RawMessage, cats TierCatalogSet, snapshotPath string) (InventoryTiersReport, error) {
	rep := InventoryTiersReport{
		SnapshotPath: snapshotPath,
		Notes: []string{
			"resolvedDisplayName comes from official public collection Tiers[].DisplayName via id+tn",
			"owning an enchant/potion is not evidence it is equipped or active",
			"equipped.level is used as tn only when > 0; level 0 stays unresolved (not tier I)",
		},
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inventoryObj, &root); err != nil {
		return rep, fmt.Errorf("inventory: %w", err)
	}
	if fa, ok := root["fetchedAt"]; ok {
		var s string
		if json.Unmarshal(fa, &s) == nil && s != "" {
			rep.SnapshotAt = &s
		}
	}

	var items []json.RawMessage
	_ = json.Unmarshal(root["items"], &items)
	for _, it := range items {
		row, ok := resolveOwnedItem(it, cats, snapshotPath)
		if ok {
			rep.Owned = append(rep.Owned, row)
		}
	}

	var equipped map[string]json.RawMessage
	_ = json.Unmarshal(root["equipped"], &equipped)
	if equipped == nil {
		rep.Notes = append(rep.Notes, "inventory.equipped object missing")
	} else {
		rep.Equipped = append(rep.Equipped, resolveEquippedGroup(equipped["enchants"], "Enchant", cats, snapshotPath)...)
		if u := equipped["ultimate"]; len(u) > 0 && string(u) != "null" {
			if row, ok := resolveEquippedUltimate(u, cats, snapshotPath); ok {
				rep.Equipped = append(rep.Equipped, row)
			}
		}
		if _, hasPotions := equipped["potions"]; !hasPotions {
			rep.Notes = append(rep.Notes, "inventory.equipped has no potions field — owned potions are not treated as active")
		}
	}
	return rep, nil
}

func resolveOwnedItem(raw json.RawMessage, cats TierCatalogSet, snapshotPath string) (TieredItemRow, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return TieredItemRow{}, false
	}
	var class, id, apiDN string
	_ = json.Unmarshal(m["class"], &class)
	_ = json.Unmarshal(m["id"], &id)
	_ = json.Unmarshal(m["displayName"], &apiDN)
	if !isTieredClass(class) {
		return TieredItemRow{}, false
	}
	tn, src := extractTN(m)
	count := ""
	if c, ok := m["count"]; ok {
		count = strings.TrimSpace(string(c))
	}
	row := TieredItemRow{
		Role: "owned", Class: class, ID: id, TN: tn, TNSource: src, Count: count,
		APIDisplayName: apiDN, EvidenceSnapshot: snapshotPath + "#items",
	}
	cat, found := cats.CatalogForClass(class)
	if !found {
		row.ResolveReason = "no public catalog loaded for class " + class
		return row, true
	}
	display, cfg, _, ok, reason := ResolveTierDisplay(cat, id, tn)
	row.ResolvedOK = ok
	row.ResolveReason = reason
	row.ResolvedDisplayName = display
	row.PublicConfigName = cfg.ConfigName
	row.PublicCollection = cfg.Collection
	if ok {
		row.EvidencePublic = cfg.ConfigName + " Tiers[" + itoa(*tn-cfg.BaseTier) + "].DisplayName"
	}
	return row, true
}

func resolveEquippedGroup(raw json.RawMessage, class string, cats TierCatalogSet, snapshotPath string) []TieredItemRow {
	if len(raw) == 0 || string(raw) == "null" {
		return nil
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return nil
	}
	var list []json.RawMessage
	_ = json.Unmarshal(top["list"], &list)
	var out []TieredItemRow
	for _, e := range list {
		var m map[string]json.RawMessage
		if json.Unmarshal(e, &m) != nil {
			continue
		}
		var id, apiDN, slot string
		_ = json.Unmarshal(m["id"], &id)
		_ = json.Unmarshal(m["displayName"], &apiDN)
		_ = json.Unmarshal(m["slot"], &slot)
		tn, src := extractEquippedTN(m)
		row := TieredItemRow{
			Role: "equipped", Class: class, ID: id, TN: tn, TNSource: src, Slot: slot,
			APIDisplayName: apiDN, EvidenceSnapshot: snapshotPath + "#equipped.enchants.list",
		}
		cat, found := cats.CatalogForClass(class)
		if !found {
			row.ResolveReason = "no public catalog loaded for class " + class
			out = append(out, row)
			continue
		}
		display, cfg, _, ok, reason := ResolveTierDisplay(cat, id, tn)
		row.ResolvedOK = ok
		row.ResolveReason = reason
		row.ResolvedDisplayName = display
		row.PublicConfigName = cfg.ConfigName
		row.PublicCollection = cfg.Collection
		if ok {
			row.EvidencePublic = cfg.ConfigName + " Tiers[" + itoa(*tn-cfg.BaseTier) + "].DisplayName"
		}
		out = append(out, row)
	}
	return out
}

func resolveEquippedUltimate(raw json.RawMessage, cats TierCatalogSet, snapshotPath string) (TieredItemRow, bool) {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return TieredItemRow{}, false
	}
	var id, apiDN string
	_ = json.Unmarshal(m["id"], &id)
	_ = json.Unmarshal(m["displayName"], &apiDN)
	if id == "" {
		return TieredItemRow{}, false
	}
	tn, src := extractEquippedTN(m)
	row := TieredItemRow{
		Role: "equipped", Class: "Ultimate", ID: id, TN: tn, TNSource: src,
		APIDisplayName: apiDN, EvidenceSnapshot: snapshotPath + "#equipped.ultimate",
	}
	cat, found := cats.CatalogForClass("Ultimate")
	if found {
		if cfg, ok := cat.ByID[strings.ToLower(id)]; ok && len(cfg.Tiers) > 0 {
			display, _, _, rok, reason := ResolveTierDisplay(cat, id, tn)
			row.ResolvedOK = rok
			row.ResolveReason = reason
			row.ResolvedDisplayName = display
			row.PublicConfigName = cfg.ConfigName
			row.PublicCollection = cfg.Collection
			return row, true
		}
		if cfg, ok := cat.ByID[strings.ToLower(id)]; ok {
			row.PublicConfigName = cfg.ConfigName
		}
	}
	row.ResolvedOK = false
	row.ResolveReason = "Ultimates public config has no Tiers[] join for equipped.level (LevelToTier left unresolved here)"
	if row.PublicConfigName == "" {
		row.PublicConfigName = "Ultimate | " + id
	}
	row.PublicCollection = "Ultimates"
	return row, true
}

func isTieredClass(class string) bool {
	_, ok := tieredClasses[strings.ToLower(strings.TrimSpace(class))]
	return ok
}

func extractTN(m map[string]json.RawMessage) (*int, string) {
	if raw, ok := m["rawData"]; ok {
		var rd map[string]json.RawMessage
		if json.Unmarshal(raw, &rd) == nil {
			if tn := parseJSONInt(rd["tn"]); tn != nil {
				return tn, "rawData.tn"
			}
		}
	}
	if tn := parseJSONInt(m["tn"]); tn != nil {
		return tn, "tn"
	}
	if tn := parseJSONInt(m["tier"]); tn != nil {
		return tn, "tier"
	}
	return nil, ""
}

func extractEquippedTN(m map[string]json.RawMessage) (*int, string) {
	if tn, src := extractTN(m); tn != nil {
		return tn, src
	}
	if lvl := parseJSONInt(m["level"]); lvl != nil {
		return lvl, "equipped.level"
	}
	return nil, ""
}

func itoa(n int) string { return strconvItoa(n) }

func strconvItoa(n int) string {
	return fmt.Sprintf("%d", n)
}
