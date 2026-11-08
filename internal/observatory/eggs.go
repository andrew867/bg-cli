package observatory

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

// WeightSemantics documents how a raw weight field should be read.
type WeightSemantics string

const (
	WeightRawDecimal      WeightSemantics = "raw_decimal_field"
	WeightNotPercent      WeightSemantics = "not_silent_percent"
	WeightHypotheticalPct WeightSemantics = "hypothetical_percent_points"
)

// EggPetEntry is one pets-table row with lossless weight string.
type EggPetEntry struct {
	ID        string                     `json:"id"`
	RawWeight string                     `json:"rawWeight"`
	Label     string                     `json:"label,omitempty"`
	JSONPath  string                     `json:"jsonPath"`
	Semantics WeightSemantics            `json:"semantics"`
	Extra     map[string]json.RawMessage `json:"extra,omitempty"`
}

// EggDocument is a parsed egg config overlay; RawJSON is the full source record.
type EggDocument struct {
	ConfigName  string          `json:"configName"`
	DisplayName string          `json:"displayName"`
	Pets        []EggPetEntry   `json:"pets"`
	OddsFFlags  []string        `json:"oddsFFlags,omitempty"` // named modifiers only; magnitudes unknown
	RawJSON     json.RawMessage `json:"rawJson"`
}

// ParseEggDocument parses one Eggs collection item, preserving unknown fields.
func ParseEggDocument(raw json.RawMessage) (EggDocument, error) {
	var doc EggDocument
	doc.RawJSON = append(json.RawMessage(nil), raw...)

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var top map[string]json.RawMessage
	if err := dec.Decode(&top); err != nil {
		return doc, err
	}
	_ = json.Unmarshal(top["configName"], &doc.ConfigName)
	if cd, ok := top["configData"]; ok {
		if err := overlayConfigData(cd, &doc); err != nil {
			return doc, err
		}
	} else {
		if err := overlayConfigData(raw, &doc); err != nil {
			return doc, err
		}
	}
	if doc.DisplayName == "" {
		doc.DisplayName = doc.ConfigName
	}
	return doc, nil
}

func overlayConfigData(raw json.RawMessage, doc *EggDocument) error {
	var cd map[string]json.RawMessage
	if err := json.Unmarshal(raw, &cd); err != nil {
		return err
	}
	if v, ok := cd["name"]; ok {
		_ = json.Unmarshal(v, &doc.DisplayName)
	}
	if v, ok := cd["oddsFFlags"]; ok {
		var flags []string
		if json.Unmarshal(v, &flags) == nil {
			doc.OddsFFlags = flags
		} else {
			// Live Eggs often use a map of petId → named flag (magnitudes unknown).
			var m map[string]string
			if json.Unmarshal(v, &m) == nil {
				keys := make([]string, 0, len(m))
				for k := range m {
					keys = append(keys, k)
				}
				sort.Strings(keys)
				for _, k := range keys {
					flags = append(flags, k+"="+m[k])
				}
				doc.OddsFFlags = flags
			}
		}
	}
	petsRaw, ok := cd["pets"]
	if !ok {
		return nil
	}
	var pets []json.RawMessage
	if err := json.Unmarshal(petsRaw, &pets); err != nil {
		return fmt.Errorf("pets table: %w", err)
	}
	for i, pr := range pets {
		entry, err := parsePetEntry(pr, fmt.Sprintf("configData.pets[%d]", i))
		if err != nil {
			return err
		}
		doc.Pets = append(doc.Pets, entry)
	}
	return nil
}

func parsePetEntry(raw json.RawMessage, path string) (EggPetEntry, error) {
	ent := EggPetEntry{
		JSONPath:  path,
		Semantics: WeightNotPercent,
		Extra:     map[string]json.RawMessage{},
	}

	// Live Eggs often encode pets as [id, weight] or [id, weight, label] tuples.
	decArr := json.NewDecoder(strings.NewReader(string(raw)))
	decArr.UseNumber()
	var arr []any
	if err := decArr.Decode(&arr); err == nil && len(arr) >= 2 {
		ent.ID = fmt.Sprintf("%v", arr[0])
		ent.RawWeight = numberToExactString(arr[1])
		ent.Semantics = WeightRawDecimal
		if len(arr) >= 3 {
			ent.Label = fmt.Sprintf("%v", arr[2])
		}
		return ent, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return EggPetEntry{}, err
	}
	// Object form: id + weight (+ optional rarity label).
	if v, ok := m["id"]; ok {
		ent.ID = fmt.Sprintf("%v", v)
	}
	if v, ok := m["petId"]; ok && ent.ID == "" {
		ent.ID = fmt.Sprintf("%v", v)
	}
	if v, ok := m["name"]; ok && ent.ID == "" {
		ent.ID = fmt.Sprintf("%v", v)
	}
	weightKeys := []string{"weight", "Weight", "chance", "Chance", "odds", "Odds"}
	for _, wk := range weightKeys {
		if v, ok := m[wk]; ok {
			ent.RawWeight = numberToExactString(v)
			ent.Semantics = WeightRawDecimal
			delete(m, wk)
			break
		}
	}
	labelKeys := []string{"label", "Label", "rarity", "Rarity", "type", "Type"}
	for _, lk := range labelKeys {
		if v, ok := m[lk]; ok {
			ent.Label = fmt.Sprintf("%v", v)
			delete(m, lk)
			break
		}
	}
	for k := range m {
		if k == "id" || k == "petId" || k == "name" {
			continue
		}
		b, _ := json.Marshal(m[k])
		ent.Extra[k] = b
	}
	return ent, nil
}

// eggNameMatches reports whether a user query matches config/display names.
// Live configs often use "N | Display Name" prefixes.
func eggNameMatches(configName, displayName, query string) bool {
	q := strings.TrimSpace(query)
	if q == "" {
		return false
	}
	cands := []string{configName, displayName}
	for _, c := range cands {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if strings.EqualFold(c, q) {
			return true
		}
		if i := strings.LastIndex(c, " | "); i >= 0 {
			if strings.EqualFold(strings.TrimSpace(c[i+3:]), q) {
				return true
			}
		}
		if strings.EqualFold(c, "1 | "+q) || strings.HasSuffix(strings.ToLower(c), strings.ToLower(" | "+q)) {
			return true
		}
	}
	return false
}

func numberToExactString(v any) string {
	switch x := v.(type) {
	case json.Number:
		return x.String()
	case float64:
		// Decoder without UseNumber only — avoid scientific noise when possible.
		return fmt.Sprintf("%v", x)
	case string:
		return x
	default:
		return fmt.Sprintf("%v", v)
	}
}

// FindEggByDisplayName scans a collection envelope for one egg document.
func FindEggByDisplayName(collectionRaw []byte, displayName string) (EggDocument, error) {
	items, err := extractCollectionItems(collectionRaw)
	if err != nil {
		return EggDocument{}, err
	}
	for _, it := range items {
		doc, err := ParseEggDocument(it)
		if err != nil {
			continue
		}
		if eggNameMatches(doc.ConfigName, doc.DisplayName, displayName) {
			return doc, nil
		}
	}
	return EggDocument{}, fmt.Errorf("egg %q not found in collection", displayName)
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
