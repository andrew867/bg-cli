package loadout

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// CollectionCacheMeta records provenance for a cached public collection dump.
type CollectionCacheMeta struct {
	URL         string `json:"url"`
	RetrievedAt string `json:"retrievedAt"`
	BodySHA256  string `json:"bodySha256"`
	ItemCount   int    `json:"itemCount"`
	Collection  string `json:"collection"`
}

// PetConfig is a normalized Pets collection row.
type PetConfig struct {
	ConfigName   string        `json:"configName"`
	HashShort    string        `json:"hashShort,omitempty"`
	DateModified string        `json:"dateModified,omitempty"`
	Name         string        `json:"name"`
	CachedPower  []json.Number `json:"cachedPower,omitempty"`
	Huge         bool          `json:"huge"`
	IndexDesc    string        `json:"indexDesc,omitempty"`
	// HalloweenExtraDamagePresentation is parsed from indexDesc only — not a verified mechanic.
	HalloweenExtraDamagePresentation *int64          `json:"halloweenExtraDamagePresentation,omitempty"`
	RawConfigData                    json.RawMessage `json:"rawConfigData,omitempty"`
}

// CharmConfig is a normalized Charms row with classified effect family.
type CharmConfig struct {
	ConfigName         string   `json:"configName"`
	HashShort          string   `json:"hashShort,omitempty"`
	DateModified       string   `json:"dateModified,omitempty"`
	DisplayName        string   `json:"displayName"`
	Family             string   `json:"family"` // damage_percent|currency_coins|currency_diamonds|reward_proc|movement|triggered|slots|mixed|unknown
	Desc               string   `json:"desc"`
	PowerField         *float64 `json:"powerField,omitempty"` // raw Tiers[0].Power — semantics depend on family
	Unique             bool     `json:"unique"`
	DamagePercentClaim *float64 `json:"damagePercentClaim,omitempty"` // from Desc when explicit; may differ from PowerField
}

// PublicConfigIndex is an offline-friendly subset of public collections.
type PublicConfigIndex struct {
	Meta   map[string]CollectionCacheMeta `json:"meta"`
	Pets   map[string]PetConfig           `json:"pets"`   // by name
	Charms map[string]CharmConfig         `json:"charms"` // by display short name
}

var halloweenExtraRE = regexp.MustCompile(`(?i)Deals\s+([\d,]+)\s+extra damage inside the Halloween Games`)

// ParsePetsCollection normalizes a /api/collection/Pets JSON document (full envelope or data array).
func ParsePetsCollection(raw []byte) ([]PetConfig, CollectionCacheMeta, error) {
	meta := CollectionCacheMeta{Collection: "Pets", BodySHA256: sha256Hex(raw)}
	items, err := extractCollectionItems(raw)
	if err != nil {
		return nil, meta, err
	}
	meta.ItemCount = len(items)
	out := make([]PetConfig, 0, len(items))
	for _, it := range items {
		var row struct {
			ConfigName   string          `json:"configName"`
			HashShort    string          `json:"hashShort"`
			DateModified string          `json:"dateModified"`
			ConfigData   json.RawMessage `json:"configData"`
		}
		if json.Unmarshal(it, &row) != nil {
			continue
		}
		var cd map[string]json.RawMessage
		_ = json.Unmarshal(row.ConfigData, &cd)
		pc := PetConfig{
			ConfigName: row.ConfigName, HashShort: row.HashShort, DateModified: row.DateModified,
			RawConfigData: row.ConfigData,
		}
		_ = json.Unmarshal(cd["name"], &pc.Name)
		_ = json.Unmarshal(cd["huge"], &pc.Huge)
		_ = json.Unmarshal(cd["indexDesc"], &pc.IndexDesc)
		_ = json.Unmarshal(cd["cachedPower"], &pc.CachedPower)
		if m := halloweenExtraRE.FindStringSubmatch(pc.IndexDesc); len(m) == 2 {
			n, err := strconv.ParseInt(strings.ReplaceAll(m[1], ",", ""), 10, 64)
			if err == nil {
				pc.HalloweenExtraDamagePresentation = &n
			}
		}
		if pc.Name == "" {
			pc.Name = row.ConfigName
		}
		out = append(out, pc)
	}
	return out, meta, nil
}

// ParseCharmsCollection normalizes Charms rows and classifies families.
func ParseCharmsCollection(raw []byte) ([]CharmConfig, CollectionCacheMeta, error) {
	meta := CollectionCacheMeta{Collection: "Charms", BodySHA256: sha256Hex(raw)}
	items, err := extractCollectionItems(raw)
	if err != nil {
		return nil, meta, err
	}
	meta.ItemCount = len(items)
	out := make([]CharmConfig, 0, len(items))
	for _, it := range items {
		var row struct {
			ConfigName   string          `json:"configName"`
			HashShort    string          `json:"hashShort"`
			DateModified string          `json:"dateModified"`
			ConfigData   json.RawMessage `json:"configData"`
		}
		if json.Unmarshal(it, &row) != nil {
			continue
		}
		var cd struct {
			Unique bool `json:"Unique"`
			Tiers  []struct {
				Desc        string   `json:"Desc"`
				DisplayName string   `json:"DisplayName"`
				Power       *float64 `json:"Power"`
			} `json:"Tiers"`
		}
		_ = json.Unmarshal(row.ConfigData, &cd)
		cc := CharmConfig{ConfigName: row.ConfigName, HashShort: row.HashShort, DateModified: row.DateModified, Unique: cd.Unique}
		if len(cd.Tiers) > 0 {
			cc.Desc = cd.Tiers[0].Desc
			cc.DisplayName = cd.Tiers[0].DisplayName
			cc.PowerField = cd.Tiers[0].Power
		}
		cc.Family, cc.DamagePercentClaim = classifyCharm(cc.DisplayName, cc.Desc, cc.PowerField)
		out = append(out, cc)
	}
	return out, meta, nil
}

func classifyCharm(display, desc string, power *float64) (family string, dmgClaim *float64) {
	d := strings.ToLower(desc)
	name := strings.ToLower(display)
	switch {
	case strings.Contains(name, "overload") || strings.Contains(d, "extra charm slots"):
		return "slots", nil
	case strings.Contains(name, "royalty") || (strings.Contains(d, "damage") && strings.Contains(d, "diamonds")):
		if m := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%\s*more damage`).FindStringSubmatch(desc); len(m) == 2 {
			v, _ := strconv.ParseFloat(m[1], 64)
			return "mixed", &v
		}
		return "mixed", nil
	case strings.Contains(name, "lightning") || strings.Contains(d, "shoots lightning"):
		return "triggered", nil
	case strings.Contains(name, "tnt") || strings.Contains(d, "spawns tnt"):
		return "triggered", nil
	case strings.Contains(name, "bonus") && strings.Contains(d, "bonus"):
		return "reward_proc", nil
	case strings.Contains(name, "coins") && strings.Contains(d, "coins"):
		return "currency_coins", nil
	case strings.Contains(name, "diamonds") && strings.Contains(d, "diamonds") && !strings.Contains(d, "damage"):
		return "currency_diamonds", nil
	case strings.Contains(name, "agility") || (strings.Contains(d, "moves") && strings.Contains(d, "faster") && !strings.Contains(d, "damage")):
		return "movement", nil
	case strings.Contains(d, "more damage") || strings.Contains(name, "strength"):
		if m := regexp.MustCompile(`(?i)(\d+(?:\.\d+)?)\s*%`).FindStringSubmatch(desc); len(m) == 2 {
			v, _ := strconv.ParseFloat(m[1], 64)
			return "damage_percent", &v
		}
		if power != nil {
			return "damage_percent", power
		}
		return "damage_percent", nil
	default:
		return "unknown", nil
	}
}

func extractCollectionItems(raw []byte) ([]json.RawMessage, error) {
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err == nil && len(env.Data) > 0 {
		raw = env.Data
	}
	var items []json.RawMessage
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("collection items: %w", err)
	}
	return items, nil
}

func sha256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// BuildPublicConfigIndex merges pets+charms into a lookup index.
func BuildPublicConfigIndex(pets []PetConfig, pmeta CollectionCacheMeta, charms []CharmConfig, cmeta CollectionCacheMeta) PublicConfigIndex {
	idx := PublicConfigIndex{
		Meta:   map[string]CollectionCacheMeta{"Pets": pmeta, "Charms": cmeta},
		Pets:   map[string]PetConfig{},
		Charms: map[string]CharmConfig{},
	}
	if pmeta.RetrievedAt == "" {
		pmeta.RetrievedAt = time.Now().UTC().Format(time.RFC3339)
		idx.Meta["Pets"] = pmeta
	}
	for _, p := range pets {
		idx.Pets[p.Name] = p
	}
	for _, c := range charms {
		short := strings.TrimSuffix(strings.TrimPrefix(c.DisplayName, ""), " Charm")
		short = strings.TrimSpace(strings.ReplaceAll(c.DisplayName, " Charm", ""))
		idx.Charms[short] = c
		idx.Charms[c.DisplayName] = c
	}
	return idx
}

// LoadPublicConfigIndexFile loads a previously saved index JSON.
func LoadPublicConfigIndexFile(path string) (PublicConfigIndex, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return PublicConfigIndex{}, err
	}
	var idx PublicConfigIndex
	if err := json.Unmarshal(b, &idx); err != nil {
		return PublicConfigIndex{}, err
	}
	if idx.Pets == nil {
		idx.Pets = map[string]PetConfig{}
	}
	if idx.Charms == nil {
		idx.Charms = map[string]CharmConfig{}
	}
	return idx, nil
}

// LookupPet finds by exact or case-insensitive name.
func (idx PublicConfigIndex) LookupPet(name string) (PetConfig, bool) {
	if p, ok := idx.Pets[name]; ok {
		return p, true
	}
	for k, p := range idx.Pets {
		if strings.EqualFold(k, name) {
			return p, true
		}
	}
	return PetConfig{}, false
}

func (idx PublicConfigIndex) LookupCharm(name string) (CharmConfig, bool) {
	if c, ok := idx.Charms[name]; ok {
		return c, true
	}
	for k, c := range idx.Charms {
		if strings.EqualFold(k, name) {
			return c, true
		}
	}
	return CharmConfig{}, false
}
