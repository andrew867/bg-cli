package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// WriteTable renders a richer terminal table when the payload shape is known,
// otherwise a compact key/value dump. Numeric values use exact JSON text.
func WriteTable(w io.Writer, env Envelope) error {
	if !env.OK {
		if env.Error != nil {
			_, err := fmt.Fprintf(w, "ERROR\t%s\t%s\n", env.Error.Code, env.Error.Message)
			return err
		}
		_, err := fmt.Fprintln(w, "ERROR\tunknown")
		return err
	}

	_, _ = fmt.Fprintf(w, "command\t%s\n", env.Command)
	if env.Source != nil {
		_, _ = fmt.Fprintf(w, "endpoint\t%s\thttp\t%d\n", env.Source.Endpoint, env.Source.HTTPStatus)
		_, _ = fmt.Fprintf(w, "retrievedAt\t%s\tcache\t%t\n", env.Source.RetrievedAt.UTC().Format("2006-01-02T15:04:05Z"), env.Source.FromLocalCache)
	}
	if env.Freshness != nil {
		_, _ = fmt.Fprintf(w, "freshness\t%s\n", env.Freshness.Meaning)
		if env.Freshness.SnapshotAt != nil {
			_, _ = fmt.Fprintf(w, "snapshotAt\t%s\n", env.Freshness.SnapshotAt.UTC().Format("2006-01-02T15:04:05Z"))
		}
	}
	_, _ = fmt.Fprintln(w, "---")

	switch env.Command {
	case "account inventory":
		if writeInventoryTable(w, env.Data) == nil {
			return nil
		}
	case "account currencies":
		if writeCurrenciesTable(w, env.Data) == nil {
			return nil
		}
	case "leaderboard list":
		if writeLeaderboardListTable(w, env.Data) == nil {
			return nil
		}
	case "auth status", "auth profiles", "auth login", "auth logout":
		return writeDataTable(w, "data", env.Data, 0)
	}
	return writeDataTable(w, "data", env.Data, 0)
}

func writeInventoryTable(w io.Writer, data any) error {
	m, ok := asMap(data)
	if !ok {
		return fmt.Errorf("not map")
	}
	invRaw, ok := m["inventory"].(json.RawMessage)
	if !ok {
		// After JSON round-trip in tests data may already be nested maps; re-encode.
		b, err := json.Marshal(m["inventory"])
		if err != nil {
			return err
		}
		invRaw = b
	}
	items, err := inventoryItems(invRaw)
	if err != nil {
		return err
	}
	if rc, ok := m["rowCount"]; ok {
		_, _ = fmt.Fprintf(w, "rowCount\t%v\n", stringifyCell(rc))
	}
	if qs, ok := m["quantitySum"]; ok {
		_, _ = fmt.Fprintf(w, "quantitySum\t%s\n", stringifyCell(qs))
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	cols := []string{"class", "id", "count", "displayName", "rap", "exists", "rarity", "collection"}
	_, _ = fmt.Fprintln(tw, strings.Join(cols, "\t"))
	for _, item := range items {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = scalarCSV(item[c])
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	return tw.Flush()
}

func writeCurrenciesTable(w io.Writer, data any) error {
	m, ok := asMap(data)
	if !ok {
		return fmt.Errorf("not map")
	}
	present, _ := m["currencyFieldPresent"].(bool)
	_, _ = fmt.Fprintf(w, "currencyFieldPresent\t%t\n", present)
	if !present {
		return nil
	}
	currencies, err := currencyMap(m["currencies"])
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "key\tid\t_am")
	keys := make([]string, 0, len(currencies))
	for k := range currencies {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(currencies[k], &fields)
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\n", k, scalarCSV(fields["id"]), scalarCSV(fields["_am"]))
	}
	return tw.Flush()
}

func writeLeaderboardListTable(w io.Writer, data any) error {
	m, ok := asMap(data)
	if !ok {
		return fmt.Errorf("not map")
	}
	raw, err := json.Marshal(m["leaderboards"])
	if err != nil {
		return err
	}
	var boards []map[string]json.RawMessage
	if err := json.Unmarshal(raw, &boards); err != nil {
		return err
	}
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintln(tw, "id\tstate\tlistSize\tcodec")
	for _, b := range boards {
		_, _ = fmt.Fprintf(tw, "%s\t%s\t%s\t%s\n",
			scalarCSV(b["id"]), scalarCSV(b["state"]), scalarCSV(b["listSize"]), scalarCSV(b["codec"]))
	}
	return tw.Flush()
}

func currencyMap(v any) (map[string]json.RawMessage, error) {
	switch t := v.(type) {
	case map[string]json.RawMessage:
		return t, nil
	case map[string]any:
		b, err := json.Marshal(t)
		if err != nil {
			return nil, err
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return out, nil
	case json.RawMessage:
		var out map[string]json.RawMessage
		if err := json.Unmarshal(t, &out); err != nil {
			return nil, err
		}
		return out, nil
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, err
		}
		var out map[string]json.RawMessage
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
}

func asMap(v any) (map[string]any, bool) {
	switch t := v.(type) {
	case map[string]any:
		return t, true
	default:
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil, false
		}
		return m, true
	}
}

func stringifyCell(v any) string {
	switch t := v.(type) {
	case json.Number:
		return string(t)
	case json.RawMessage:
		return scalarCSV(t)
	case string:
		return t
	case nil:
		return ""
	default:
		return fmt.Sprint(t)
	}
}

func writeDataTable(w io.Writer, prefix string, v any, depth int) error {
	if depth > 4 {
		_, err := fmt.Fprintf(w, "%s\t…\n", prefix)
		return err
	}
	switch t := v.(type) {
	case nil:
		_, err := fmt.Fprintf(w, "%s\t\n", prefix)
		return err
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if err := writeDataTable(w, prefix+"."+k, t[k], depth+1); err != nil {
				return err
			}
		}
		return nil
	case []any:
		if len(t) > 0 {
			if _, ok := t[0].(map[string]any); ok {
				return writeObjectArrayTable(w, prefix, t)
			}
		}
		_, err := fmt.Fprintf(w, "%s.count\t%d\n", prefix, len(t))
		return err
	case string:
		_, err := fmt.Fprintf(w, "%s\t%s\n", prefix, sanitizeTableCell(t))
		return err
	case json.Number:
		_, err := fmt.Fprintf(w, "%s\t%s\n", prefix, string(t))
		return err
	case json.RawMessage:
		var nested any
		decErr := json.Unmarshal(t, &nested)
		if decErr == nil {
			// Prefer number-preserving decode for nested objects.
			var asNum any
			if err := decodeUseNumber(t, &asNum); err == nil {
				return writeDataTable(w, prefix, asNum, depth)
			}
			return writeDataTable(w, prefix, nested, depth)
		}
		_, err := fmt.Fprintf(w, "%s\t%s\n", prefix, sanitizeTableCell(string(t)))
		return err
	case float64:
		if t == float64(int64(t)) && t < 1e15 && t > -1e15 {
			_, err := fmt.Fprintf(w, "%s\t%.0f\n", prefix, t)
			return err
		}
		_, err := fmt.Fprintf(w, "%s\t%f\n", prefix, t)
		return err
	case bool:
		_, err := fmt.Fprintf(w, "%s\t%t\n", prefix, t)
		return err
	default:
		_, err := fmt.Fprintf(w, "%s\t%v\n", prefix, t)
		return err
	}
}

func writeObjectArrayTable(w io.Writer, prefix string, rows []any) error {
	colSet := map[string]struct{}{}
	parsed := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		m, ok := row.(map[string]any)
		if !ok {
			_, err := fmt.Fprintf(w, "%s.count\t%d\n", prefix, len(rows))
			return err
		}
		parsed = append(parsed, m)
		for k := range m {
			colSet[k] = struct{}{}
		}
	}
	cols := make([]string, 0, len(colSet))
	for k := range colSet {
		cols = append(cols, k)
	}
	sort.Strings(cols)
	tw := tabwriter.NewWriter(w, 0, 4, 2, ' ', 0)
	_, _ = fmt.Fprintf(tw, "%s\n", strings.Join(cols, "\t"))
	for _, m := range parsed {
		cells := make([]string, len(cols))
		for i, c := range cols {
			cells[i] = sanitizeTableCell(stringifyCell(m[c]))
		}
		_, _ = fmt.Fprintln(tw, strings.Join(cells, "\t"))
	}
	_, _ = fmt.Fprintf(w, "%s.count\t%d\n", prefix, len(rows))
	return tw.Flush()
}

func sanitizeTableCell(s string) string {
	s = strings.ReplaceAll(s, "\t", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return s
}

func decodeUseNumber(b []byte, v any) error {
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.UseNumber()
	return dec.Decode(v)
}
