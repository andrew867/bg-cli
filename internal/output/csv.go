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
	"variant",
	"golden",
	"rainbow",
	"shiny",
	"pt",
	"sh",
	"stackKey",
	"rap",
	"rapApproximate",
	"exists",
	"rarity",
	"rarityNumber",
	"collection",
}

// InventoryCSVOptions controls optional inventory CSV enrichment.
type InventoryCSVOptions struct {
	RAPIndex RAPIndex
	Excel    bool // UTF-8 BOM for Excel
}

// CurrencyCSVOptions controls optional currency CSV polish.
type CurrencyCSVOptions struct {
	Excel bool // UTF-8 BOM + amount column mirrored from _am
}

// WriteInventoryCSV writes inventory item rows as CSV with exact numeric text.
// Counts, RAP, exists, and similar fields are copied from JSON number literals
// (or stringified scalars) without float64 conversion.
// Variant columns are derived from top-level / rawData pt+sh fields.
func WriteInventoryCSV(w io.Writer, inventory json.RawMessage, opts InventoryCSVOptions) error {
	items, err := inventoryItems(inventory)
	if err != nil {
		return err
	}
	if opts.Excel {
		if _, err := io.WriteString(w, "\ufeff"); err != nil {
			return err
		}
	}
	cols := append([]string{}, InventoryCSVColumns...)
	joinRAP := opts.RAPIndex != nil
	if joinRAP {
		cols = append(cols, "marketRap", "marketRapMatched")
	}
	cw := csv.NewWriter(w)
	if err := cw.Write(cols); err != nil {
		return err
	}
	for _, item := range items {
		v := DeriveItemVariant(item)
		row := make([]string, len(cols))
		for i, col := range InventoryCSVColumns {
			switch col {
			case "variant":
				row[i] = v.Label
			case "golden":
				row[i] = boolCSV(v.Golden)
			case "rainbow":
				row[i] = boolCSV(v.Rainbow)
			case "shiny":
				row[i] = boolCSV(v.Shiny)
			case "pt":
				row[i] = v.PT
			case "sh":
				row[i] = v.SH
			default:
				row[i] = scalarCSV(item[col])
			}
		}
		if joinRAP {
			class := scalarCSV(item["class"])
			id := scalarCSV(item["id"])
			if val, ok := opts.RAPIndex.Lookup(class, id); ok {
				row[len(InventoryCSVColumns)] = scalarCSV(val)
				row[len(InventoryCSVColumns)+1] = "true"
			} else {
				row[len(InventoryCSVColumns)] = ""
				row[len(InventoryCSVColumns)+1] = "false"
			}
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

func boolCSV(v bool) string {
	if v {
		return "true"
	}
	return "false"
}

// WriteCurrenciesCSV writes currency map entries with exact _am text.
// When Excel polish is on, also writes a mirrored amount column and a UTF-8 BOM.
func WriteCurrenciesCSV(w io.Writer, currencies map[string]json.RawMessage, opts CurrencyCSVOptions) error {
	if opts.Excel {
		if _, err := io.WriteString(w, "\ufeff"); err != nil {
			return err
		}
	}
	cw := csv.NewWriter(w)
	header := []string{"key", "id", "_am", "amount"}
	if err := cw.Write(header); err != nil {
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
		am := scalarCSV(fields["_am"])
		if err := cw.Write([]string{k, scalarCSV(fields["id"]), am, am}); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// WriteQuotaCSV writes a single quota/rate-limit summary row.
func WriteQuotaCSV(w io.Writer, data map[string]any, excel bool) error {
	if excel {
		if _, err := io.WriteString(w, "\ufeff"); err != nil {
			return err
		}
	}
	cw := csv.NewWriter(w)
	cols := []string{
		"used", "limit", "remaining", "retryAfter",
		"quotaExhausted", "resetsAt", "nextRefreshEligibleAt",
		"consumedThisCall", "httpStatus",
	}
	if err := cw.Write(cols); err != nil {
		return err
	}
	row := make([]string, len(cols))
	for i, c := range cols {
		row[i] = stringifyCSVAny(data[c])
	}
	if err := cw.Write(row); err != nil {
		return err
	}
	cw.Flush()
	return cw.Error()
}

func stringifyCSVAny(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case json.Number:
		return string(t)
	case json.RawMessage:
		return scalarCSV(t)
	case string:
		return t
	case bool:
		if t {
			return "true"
		}
		return "false"
	case float64:
		return fmt.Sprintf("%.0f", t)
	default:
		return fmt.Sprint(t)
	}
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
