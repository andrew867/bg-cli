package observatory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// TieredConfig is one public collection row with a Tiers array (Enchants, Potions, Charms, …).
type TieredConfig struct {
	Collection string          `json:"collection"`
	ConfigName string          `json:"configName"`
	ID         string          `json:"id"` // short id used by inventory (e.g. Coins)
	BaseTier   int             `json:"baseTier"`
	MaxTier    int             `json:"maxTier,omitempty"`
	Tiers      []TierEntry     `json:"tiers"`
	RawJSON    json.RawMessage `json:"rawJson,omitempty"`
}

// TierEntry is one public Tiers[] element. DisplayName already includes the roman tier when present.
type TierEntry struct {
	Index       int      `json:"index"` // 0-based array index
	TierNumber  int      `json:"tierNumber"`
	DisplayName string   `json:"displayName"`
	Desc        string   `json:"desc,omitempty"`
	Power       *float64 `json:"power,omitempty"`
}

// TierCatalog indexes tiered configs by short inventory id (case-insensitive)
// within a single public collection (Enchants vs Potions can share short ids like "Coins").
type TierCatalog struct {
	Collection string                  `json:"collection"`
	ByID       map[string]TieredConfig `json:"byId"`
	Meta       CollectionSnapshotMeta  `json:"meta,omitempty"`
}

// TierCatalogSet holds one catalog per public collection name.
type TierCatalogSet struct {
	ByCollection map[string]TierCatalog `json:"byCollection"`
}

// ParseTieredCollection parses Enchants/Potions/Charms-style public collection JSON.
func ParseTieredCollection(collection string, raw []byte) (TierCatalog, error) {
	cat := TierCatalog{Collection: collection, ByID: map[string]TieredConfig{}}
	items, err := extractCollectionItems(raw)
	if err != nil {
		return cat, err
	}
	for _, it := range items {
		cfg, err := parseTieredItem(collection, it)
		if err != nil || cfg.ID == "" {
			continue
		}
		key := strings.ToLower(cfg.ID)
		cat.ByID[key] = cfg
	}
	return cat, nil
}

func parseTieredItem(collection string, raw json.RawMessage) (TieredConfig, error) {
	var row struct {
		ConfigName string          `json:"configName"`
		ConfigData json.RawMessage `json:"configData"`
		Collection string          `json:"collection"`
	}
	if err := json.Unmarshal(raw, &row); err != nil {
		return TieredConfig{}, err
	}
	col := firstNonEmpty(row.Collection, collection)
	var cd map[string]json.RawMessage
	if err := json.Unmarshal(row.ConfigData, &cd); err != nil {
		return TieredConfig{}, err
	}
	cfg := TieredConfig{
		Collection: col,
		ConfigName: row.ConfigName,
		BaseTier:   1,
		RawJSON:    append(json.RawMessage(nil), raw...),
	}
	if v, ok := cd["BaseTier"]; ok {
		if n := parseJSONInt(v); n != nil {
			cfg.BaseTier = *n
		}
	}
	if v, ok := cd["MaxTier"]; ok {
		if n := parseJSONInt(v); n != nil {
			cfg.MaxTier = *n
		}
	}
	cfg.ID = shortIDFromConfigName(row.ConfigName)
	if v, ok := cd["DisplayName"]; ok {
		var dn string
		_ = json.Unmarshal(v, &dn)
		if dn != "" && !strings.Contains(row.ConfigName, "|") {
			cfg.ID = dn
		}
	}
	if tiersRaw, ok := cd["Tiers"]; ok {
		var tiers []json.RawMessage
		if err := json.Unmarshal(tiersRaw, &tiers); err != nil {
			return cfg, fmt.Errorf("Tiers: %w", err)
		}
		for i, tr := range tiers {
			var te struct {
				DisplayName string   `json:"DisplayName"`
				Desc        string   `json:"Desc"`
				Power       *float64 `json:"Power"`
			}
			_ = json.Unmarshal(tr, &te)
			tn := cfg.BaseTier + i
			cfg.Tiers = append(cfg.Tiers, TierEntry{
				Index: i, TierNumber: tn, DisplayName: te.DisplayName, Desc: te.Desc, Power: te.Power,
			})
		}
	}
	return cfg, nil
}

func shortIDFromConfigName(configName string) string {
	configName = strings.TrimSpace(configName)
	if i := strings.Index(configName, " | "); i >= 0 {
		return strings.TrimSpace(configName[i+3:])
	}
	return configName
}

func parseJSONInt(raw json.RawMessage) *int {
	var n int
	if json.Unmarshal(raw, &n) == nil {
		return &n
	}
	var f float64
	if json.Unmarshal(raw, &f) == nil {
		i := int(f)
		return &i
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil {
			return &v
		}
	}
	return nil
}

// ResolveTierDisplay looks up Tiers[tn] display name. tn is the inventory rawData.tn / equipped level.
// Returns unresolved when tn is missing, zero, or out of range — never invents tier I from absent tn.
func ResolveTierDisplay(cat TierCatalog, id string, tn *int) (display string, cfg TieredConfig, tier TierEntry, ok bool, reason string) {
	cfg, found := cat.ByID[strings.ToLower(strings.TrimSpace(id))]
	if !found {
		return "", TieredConfig{}, TierEntry{}, false, "no public config for id"
	}
	if tn == nil {
		return "", cfg, TierEntry{}, false, "tn absent"
	}
	if *tn <= 0 {
		return "", cfg, TierEntry{}, false, "tn/level <= 0 is not tier I"
	}
	idx := *tn - cfg.BaseTier
	if idx < 0 || idx >= len(cfg.Tiers) {
		return "", cfg, TierEntry{}, false, fmt.Sprintf("tn=%d out of range for BaseTier=%d len(Tiers)=%d", *tn, cfg.BaseTier, len(cfg.Tiers))
	}
	te := cfg.Tiers[idx]
	if te.DisplayName == "" {
		return "", cfg, te, false, "public Tier DisplayName empty"
	}
	return te.DisplayName, cfg, te, true, "public Tiers[tn-BaseTier].DisplayName"
}

// NewTierCatalogSet groups catalogs by collection name.
func NewTierCatalogSet(cats ...TierCatalog) TierCatalogSet {
	out := TierCatalogSet{ByCollection: map[string]TierCatalog{}}
	for _, c := range cats {
		name := firstNonEmpty(c.Collection, "unknown")
		out.ByCollection[name] = c
	}
	return out
}

// CatalogForClass maps inventory class → public collection catalog.
func (s TierCatalogSet) CatalogForClass(class string) (TierCatalog, bool) {
	col, ok := tieredClasses[strings.ToLower(strings.TrimSpace(class))]
	if !ok {
		return TierCatalog{}, false
	}
	cat, found := s.ByCollection[col]
	return cat, found
}

// CatalogIDs returns sorted short ids.
func (c TierCatalog) CatalogIDs() []string {
	ids := make([]string, 0, len(c.ByID))
	for _, v := range c.ByID {
		ids = append(ids, v.ID)
	}
	sort.Strings(ids)
	return ids
}
