package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
)

// InventoryCSVColumns is the stable column order for inventory exports.
var InventoryCSVColumns = []string{
	"class",
	"id",
	"count",
	"displayName",
	"rap",
	"rapApproximate",
	"exists",
	"rarity",
	"rarityNumber",
	"collection",
}

// WriteInventoryCSV writes inventory item rows as CSV with exact numeric text.
// Counts, RAP, exists, and similar fields are copied from JSON number literals
// (or stringified scalars) without float64 conversion.
func WriteInventoryCSV(w io.Writer, inventory json.RawMessage) error {
	items, err := inventoryItems(inventory)
	if err != nil {
		return err
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(InventoryCSVColumns); err != nil {
		return err
	}
	for _, item := range items {
		row := make([]string, len(InventoryCSVColumns))
		for i, col := range InventoryCSVColumns {
			row[i] = scalarCSV(item[col])
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteCurrenciesCSV writes currency map entries with exact _am text.
func WriteCurrenciesCSV(w io.Writer, currencies map[string]json.RawMessage) error {
	cw := csv.NewWriter(w)
	if err := cw.Write([]string{"key", "id", "_am"}); err != nil {
		return err
	}
	keys := make([]string, 0, len(currencies))
	for k := range currencies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(currencies[k], &fields)
		if err := cw.Write([]string{k, scalarCSV(fields["id"]), scalarCSV(fields["_am"])}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func inventoryItems(inventory json.RawMessage) ([]map[string]json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inventory, &root); err != nil {
		return nil, fmt.Errorf("inventory CSV: %w", err)
	}
	rawItems, ok := root["items"]
	if !ok {
		return nil, fmt.Errorf("inventory CSV: missing items array")
	}
	var items []json.RawMessage
	if err := json.Unmarshal(rawItems, &items); err != nil {
		return nil, fmt.Errorf("inventory CSV: items: %w", err)
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	for _, it := range items {
		var m map[string]json.RawMessage
		if err := json.Unmarshal(it, &m); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, nil
}

func scalarCSV(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			return str
		}
	}
	// Numbers and booleans: keep exact JSON token text.
	return s
}
