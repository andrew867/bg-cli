package observatory

import (
	"encoding/json"
	"fmt"
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
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.UseNumber()
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return EggPetEntry{}, err
	}
	ent := EggPetEntry{
		JSONPath:  path,
		Semantics: WeightNotPercent,
		Extra:     map[string]json.RawMessage{},
	}
	// Standard keys in Pet Simulator eggs table: id + weight (numeric) + optional rarity label field.
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
		if doc.DisplayName == displayName || doc.ConfigName == displayName {
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
