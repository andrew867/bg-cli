package output

import (
	"encoding/json"
	"fmt"
	"strings"
)

// RAPIndex maps lower(category)+"\x00"+lower(id) → exact RAP value JSON token.
type RAPIndex map[string]json.RawMessage

// BuildRAPIndex indexes public /api/rap payloads (array or nested map forms).
func BuildRAPIndex(raw json.RawMessage) (RAPIndex, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return RAPIndex{}, nil
	}
	idx := RAPIndex{}
	var arr []json.RawMessage
	if err := json.Unmarshal(raw, &arr); err == nil {
		for _, entry := range arr {
			var fields map[string]json.RawMessage
			if json.Unmarshal(entry, &fields) != nil {
				continue
			}
			var category string
			_ = json.Unmarshal(fields["category"], &category)
			var cd map[string]json.RawMessage
			_ = json.Unmarshal(fields["configData"], &cd)
			var id string
			_ = json.Unmarshal(cd["id"], &id)
			val := fields["value"]
			if category == "" || id == "" || len(val) == 0 {
				continue
			}
			idx[rapKey(category, id)] = append(json.RawMessage(nil), val...)
		}
		return idx, nil
	}

	var asMap map[string]json.RawMessage
	if err := json.Unmarshal(raw, &asMap); err != nil {
		return nil, fmt.Errorf("rap index: unsupported shape: %w", err)
	}
	for category, v := range asMap {
		var inner map[string]json.RawMessage
		if json.Unmarshal(v, &inner) == nil {
			for id, val := range inner {
				// value may be a bare number or object with value field
				idx[rapKey(category, id)] = normalizeRAPValue(val)
			}
			continue
		}
		var nested []json.RawMessage
		if json.Unmarshal(v, &nested) == nil {
			for _, entry := range nested {
				var fields map[string]json.RawMessage
				_ = json.Unmarshal(entry, &fields)
				id := scalarCSV(fields["id"])
				if id == "" {
					var cd map[string]json.RawMessage
					_ = json.Unmarshal(fields["configData"], &cd)
					id = scalarCSV(cd["id"])
				}
				if id == "" {
					continue
				}
				idx[rapKey(category, id)] = normalizeRAPValue(fields["value"])
			}
		}
	}
	return idx, nil
}

func normalizeRAPValue(val json.RawMessage) json.RawMessage {
	if len(val) == 0 || string(val) == "null" {
		return nil
	}
	s := strings.TrimSpace(string(val))
	if len(s) > 0 && s[0] == '{' {
		var fields map[string]json.RawMessage
		if json.Unmarshal(val, &fields) == nil {
			if v, ok := fields["value"]; ok {
				return append(json.RawMessage(nil), v...)
			}
		}
	}
	return append(json.RawMessage(nil), val...)
}

func rapKey(category, id string) string {
	return strings.ToLower(category) + "\x00" + strings.ToLower(id)
}

// Lookup returns the exact market RAP token for class/category + id.
func (idx RAPIndex) Lookup(category, id string) (json.RawMessage, bool) {
	if idx == nil {
		return nil, false
	}
	v, ok := idx[rapKey(category, id)]
	return v, ok && len(v) > 0
}
