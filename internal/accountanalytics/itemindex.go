package accountanalytics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

const itemIndexMethodID = "accountanalytics.itemindex.v1"

// VariantRecord is one parsed index key for a countable bucket (e.g. Pet).
type VariantRecord struct {
	KeyJSON   string `json:"keyJson"`
	BaseID    string `json:"baseId"`
	PT        string `json:"pt,omitempty"`
	SH        string `json:"sh,omitempty"`
	Variant   string `json:"variant"`
	Count     string `json:"count"`
	CountExact snapshot.ExactInt `json:"-"`
}

// BoolRecord is one parsed index key for a boolean bucket (Egg, Charm, …).
type BoolRecord struct {
	KeyJSON string `json:"keyJson"`
	BaseID  string `json:"baseId,omitempty"`
	Owned   bool   `json:"owned"`
}

// ClassBucket holds either countable variants or boolean flags for one ItemIndex class.
type ClassBucket struct {
	Class     string          `json:"class"`
	Countable bool            `json:"countable"`
	Variants  []VariantRecord `json:"variants,omitempty"`
	Flags     []BoolRecord    `json:"flags,omitempty"`
	Total     string          `json:"total,omitempty"`
}

// ParsedItemIndex is a decoded account ItemIndex.
type ParsedItemIndex struct {
	Classes map[string]*ClassBucket `json:"classes"`
	MethodID string                 `json:"methodId"`
}

// IndexAnalysis summarizes one index.
type IndexAnalysis struct {
	MethodID       string        `json:"methodId"`
	Classes        []string      `json:"classes"`
	DiscoveredBase []string      `json:"discoveredBaseIds"`
	VariantCount   int           `json:"variantEntries"`
	BoolCount      int           `json:"boolEntries"`
	TotalsByClass  map[string]string `json:"totalsByClass,omitempty"`
	Buckets        []ClassBucket `json:"buckets"`
}

// IndexDiffRow is one key-level delta between two indexes.
type IndexDiffRow struct {
	Class   string `json:"class"`
	KeyJSON string `json:"keyJson"`
	BaseID  string `json:"baseId,omitempty"`
	Kind    string `json:"kind"` // count|bool
	Before  string `json:"before,omitempty"`
	After   string `json:"after,omitempty"`
	Delta   string `json:"delta,omitempty"`
	Changed bool   `json:"changed"`
}

// IndexDiff compares two parsed indexes.
type IndexDiff struct {
	MethodID string         `json:"methodId"`
	Rows     []IndexDiffRow `json:"rows"`
}

// CatalogPet is one public collection catalog pet used as completion denominator.
type CatalogPet struct {
	ID string `json:"id"`
}

// CompletionReportResult measures index coverage vs an explicit catalog list.
type CompletionReportResult struct {
	MethodID      string   `json:"methodId"`
	Denominator   int      `json:"denominator"`
	Discovered    int      `json:"discovered"`
	Missing       []string `json:"missing,omitempty"`
	Percent       float64  `json:"percent"`
	DenominatorID string   `json:"denominatorId"` // catalog-pet-list
}

// ParseItemIndex decodes ItemIndex JSON (bare ItemIndex object or envelope fragment).
func ParseItemIndex(raw json.RawMessage) (ParsedItemIndex, error) {
	idxRaw, err := unwrapItemIndex(raw)
	if err != nil {
		return ParsedItemIndex{}, err
	}
	var classes map[string]json.RawMessage
	if err := json.Unmarshal(idxRaw, &classes); err != nil {
		return ParsedItemIndex{}, fmt.Errorf("item index: %w", err)
	}
	out := ParsedItemIndex{
		Classes:  map[string]*ClassBucket{},
		MethodID: itemIndexMethodID,
	}
	for class, bucketRaw := range classes {
		b, err := parseClassBucket(class, bucketRaw)
		if err != nil {
			return ParsedItemIndex{}, fmt.Errorf("%s: %w", class, err)
		}
		out.Classes[class] = b
	}
	return out, nil
}

func unwrapItemIndex(raw json.RawMessage) (json.RawMessage, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("empty item index")
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return raw, nil
	}
	if v, ok := top["ItemIndex"]; ok && len(v) > 0 && string(v) != "null" {
		return v, nil
	}
	if v, ok := top["itemIndex"]; ok && len(v) > 0 && string(v) != "null" {
		return v, nil
	}
	// Heuristic: already a class map (Pet/Egg/…).
	for k := range top {
		if strings.EqualFold(k, "Pet") || strings.EqualFold(k, "Egg") || strings.EqualFold(k, "Charm") {
			return raw, nil
		}
	}
	return nil, fmt.Errorf("item index: missing ItemIndex object")
}

func parseClassBucket(class string, raw json.RawMessage) (*ClassBucket, error) {
	b := &ClassBucket{Class: class}
	// Try countable map first (values are numbers).
	var countMap map[string]json.RawMessage
	if json.Unmarshal(raw, &countMap) == nil && len(countMap) > 0 {
		allCount := true
		for _, v := range countMap {
			s := strings.TrimSpace(string(v))
			if s == "true" || s == "false" {
				allCount = false
				break
			}
			if len(s) > 0 && s[0] == '{' {
				allCount = false
				break
			}
			ei := snapshot.ParseExactInt(v)
			if !ei.ParseOK {
				allCount = false
				break
			}
		}
		if allCount {
			b.Countable = true
			sum := snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}
			for keyJSON, val := range countMap {
				ei := snapshot.ParseExactInt(val)
				vr, err := parseVariantKey(keyJSON, ei)
				if err != nil {
					return nil, err
				}
				b.Variants = append(b.Variants, vr)
				sum = snapshot.AddExact(sum, ei)
			}
			sort.Slice(b.Variants, func(i, j int) bool {
				if b.Variants[i].BaseID != b.Variants[j].BaseID {
					return b.Variants[i].BaseID < b.Variants[j].BaseID
				}
				return b.Variants[i].KeyJSON < b.Variants[j].KeyJSON
			})
			b.Total = sum.Text
			return b, nil
		}
	}
	// Heterogeneous or bool-only map.
	var het map[string]json.RawMessage
	if err := json.Unmarshal(raw, &het); err != nil {
		return nil, err
	}
	for keyJSON, val := range het {
		s := strings.TrimSpace(string(val))
		switch s {
		case "true", "false":
			br := BoolRecord{KeyJSON: keyJSON, Owned: s == "true"}
			if strings.HasPrefix(strings.TrimSpace(keyJSON), "{") {
				id, _, _ := parseKeyObject(keyJSON)
				br.BaseID = id
			} else {
				br.BaseID = keyJSON
			}
			b.Flags = append(b.Flags, br)
		default:
			ei := snapshot.ParseExactInt(val)
			if !ei.ParseOK {
				return nil, fmt.Errorf("unsupported value for key %q", keyJSON)
			}
			b.Countable = true
			vr, err := parseVariantKey(keyJSON, ei)
			if err != nil {
				return nil, err
			}
			b.Variants = append(b.Variants, vr)
		}
	}
	sort.Slice(b.Flags, func(i, j int) bool { return b.Flags[i].KeyJSON < b.Flags[j].KeyJSON })
	sort.Slice(b.Variants, func(i, j int) bool {
		if b.Variants[i].BaseID != b.Variants[j].BaseID {
			return b.Variants[i].BaseID < b.Variants[j].BaseID
		}
		return b.Variants[i].KeyJSON < b.Variants[j].KeyJSON
	})
	if b.Countable {
		sum := snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}
		for _, vr := range b.Variants {
			sum = snapshot.AddExact(sum, vr.CountExact)
		}
		b.Total = sum.Text
	}
	return b, nil
}

func parseVariantKey(keyJSON string, count snapshot.ExactInt) (VariantRecord, error) {
	vr := VariantRecord{KeyJSON: keyJSON, Count: count.Text, CountExact: count}
	keyJSON = strings.TrimSpace(keyJSON)
	if strings.HasPrefix(keyJSON, "{") {
		id, pt, sh := parseKeyObject(keyJSON)
		vr.BaseID = id
		vr.PT = pt
		vr.SH = sh
		vr.Variant = variantLabel(pt, sh)
		return vr, nil
	}
	// Plain id key (unusual for Pet but tolerated).
	vr.BaseID = keyJSON
	vr.Variant = "Regular"
	return vr, nil
}

func parseKeyObject(keyJSON string) (id, pt, sh string) {
	var obj map[string]json.RawMessage
	if json.Unmarshal([]byte(keyJSON), &obj) != nil {
		return "", "", ""
	}
	_ = json.Unmarshal(obj["id"], &id)
	pt = scalarJSON(obj["pt"])
	sh = scalarJSON(obj["sh"])
	return id, pt, sh
}

func variantLabel(pt, sh string) string {
	golden := pt == "1"
	rainbow := pt == "2"
	shiny := strings.EqualFold(sh, "true")
	switch {
	case rainbow && shiny:
		return "Rainbow Shiny"
	case golden && shiny:
		return "Golden Shiny"
	case rainbow:
		return "Rainbow"
	case golden:
		return "Golden"
	case shiny:
		return "Shiny"
	default:
		return "Regular"
	}
}

func scalarJSON(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			return str
		}
	}
	return s
}

// AnalyzeIndex reads itemIndex from a loaded snapshot bundle.
func AnalyzeIndex(bundle *snapshot.Bundle) (IndexAnalysis, error) {
	raw, ok := itemIndexRaw(bundle)
	if !ok {
		return IndexAnalysis{}, fmt.Errorf("bundle missing itemIndex view")
	}
	parsed, err := ParseItemIndex(raw)
	if err != nil {
		return IndexAnalysis{}, err
	}
	baseSet := map[string]struct{}{}
	var bases []string
	variantCount, boolCount := 0, 0
	totals := map[string]string{}
	var buckets []ClassBucket
	classNames := make([]string, 0, len(parsed.Classes))
	for k := range parsed.Classes {
		classNames = append(classNames, k)
	}
	sort.Strings(classNames)
	for _, cn := range classNames {
		b := parsed.Classes[cn]
		buckets = append(buckets, *b)
		if b.Total != "" {
			totals[cn] = b.Total
		}
		for _, vr := range b.Variants {
			variantCount++
			if vr.BaseID != "" {
				if _, ok := baseSet[vr.BaseID]; !ok {
					baseSet[vr.BaseID] = struct{}{}
					bases = append(bases, vr.BaseID)
				}
			}
		}
		for _, fl := range b.Flags {
			boolCount++
			if fl.BaseID != "" {
				if _, ok := baseSet[fl.BaseID]; !ok {
					baseSet[fl.BaseID] = struct{}{}
					bases = append(bases, fl.BaseID)
				}
			}
		}
	}
	sort.Strings(bases)
	return IndexAnalysis{
		MethodID:       itemIndexMethodID,
		Classes:        classNames,
		DiscoveredBase: bases,
		VariantCount:   variantCount,
		BoolCount:      boolCount,
		TotalsByClass:  totals,
		Buckets:        buckets,
	}, nil
}

func itemIndexRaw(bundle *snapshot.Bundle) (json.RawMessage, bool) {
	if bundle == nil {
		return nil, false
	}
	if raw, ok := bundle.ViewRaw["itemIndex"]; ok && len(raw) > 0 {
		return normalizeItemIndexView(raw), true
	}
	if bundle.ViewData != nil {
		if data, ok := bundle.ViewData["itemIndex"]; ok {
			if raw, ok := data["ItemIndex"]; ok {
				return raw, true
			}
		}
	}
	return nil, false
}

func normalizeItemIndexView(raw json.RawMessage) json.RawMessage {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return raw
	}
	if v, ok := top["ItemIndex"]; ok {
		return v
	}
	return raw
}

// DiffIndexes compares two parsed item indexes key-by-key.
func DiffIndexes(from, to ParsedItemIndex) IndexDiff {
	type key struct{ class, keyJSON, kind string }
	rows := map[key]IndexDiffRow{}
	merge := func(idx ParsedItemIndex, after bool) {
		for class, b := range idx.Classes {
			for _, vr := range b.Variants {
				k := key{class: class, keyJSON: vr.KeyJSON, kind: "count"}
				row := rows[k]
				row.Class, row.KeyJSON, row.Kind = class, vr.KeyJSON, "count"
				row.BaseID = vr.BaseID
				if after {
					row.After = vr.Count
				} else {
					row.Before = vr.Count
				}
				rows[k] = row
			}
			for _, fl := range b.Flags {
				k := key{class: class, keyJSON: fl.KeyJSON, kind: "bool"}
				row := rows[k]
				row.Class, row.KeyJSON, row.Kind = class, fl.KeyJSON, "bool"
				row.BaseID = fl.BaseID
				val := "false"
				if fl.Owned {
					val = "true"
				}
				if after {
					row.After = val
				} else {
					row.Before = val
				}
				rows[k] = row
			}
		}
	}
	merge(from, false)
	merge(to, true)
	out := make([]IndexDiffRow, 0, len(rows))
	for _, row := range rows {
		if row.Kind == "count" {
			before := snapshot.ParseExactIntString(defaultZero(row.Before))
			after := snapshot.ParseExactIntString(defaultZero(row.After))
			d, err := snapshot.SubExact(after, before)
			if err == nil {
				row.Delta = d.Text
			}
			row.Changed = row.Before != row.After
		} else {
			row.Changed = row.Before != row.After
		}
		out = append(out, row)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Class != out[j].Class {
			return out[i].Class < out[j].Class
		}
		return out[i].KeyJSON < out[j].KeyJSON
	})
	return IndexDiff{MethodID: itemIndexMethodID, Rows: out}
}

func defaultZero(s string) string {
	if s == "" {
		return "0"
	}
	return s
}

// CompletionReport counts catalog pets present in the Pet index (base id match).
func CompletionReport(index ParsedItemIndex, catalogPets []CatalogPet) CompletionReportResult {
	denom := len(catalogPets)
	petBucket := index.Classes["Pet"]
	found := map[string]struct{}{}
	if petBucket != nil {
		for _, vr := range petBucket.Variants {
			found[strings.ToLower(vr.BaseID)] = struct{}{}
		}
		for _, fl := range petBucket.Flags {
			if fl.Owned && fl.BaseID != "" {
				found[strings.ToLower(fl.BaseID)] = struct{}{}
			}
		}
	}
	discovered := 0
	var missing []string
	for _, cp := range catalogPets {
		if _, ok := found[strings.ToLower(cp.ID)]; ok {
			discovered++
		} else {
			missing = append(missing, cp.ID)
		}
	}
	pct := 0.0
	if denom > 0 {
		pct = float64(discovered) / float64(denom) * 100
	}
	return CompletionReportResult{
		MethodID:      itemIndexMethodID,
		Denominator:   denom,
		Discovered:    discovered,
		Missing:       missing,
		Percent:       pct,
		DenominatorID: "catalog-pet-list",
	}
}
