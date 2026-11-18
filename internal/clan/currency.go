package clan

import (
	"encoding/json"
	"strings"
)

// CurrencyAmount is one current balance. Amount is exact integer text when
// the API literal is an integer, otherwise the raw literal.
type CurrencyAmount struct {
	Amount      string `json:"amount"`
	DisplayName string `json:"displayName"`
}

// CurrenciesFromProfile reads the profile Currency map. known is false when
// the profile is missing or the Currency field itself is absent. An empty
// object is a known empty list. Missing currencies are not treated as zero.
func CurrenciesFromProfile(profile json.RawMessage) (map[string]CurrencyAmount, bool) {
	if len(profile) == 0 || string(profile) == "null" {
		return map[string]CurrencyAmount{}, false
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(profile, &top) != nil {
		return map[string]CurrencyAmount{}, false
	}
	raw, ok := top["Currency"]
	if !ok || string(raw) == "null" {
		return map[string]CurrencyAmount{}, false
	}
	var entries map[string]json.RawMessage
	if json.Unmarshal(raw, &entries) != nil || entries == nil {
		return map[string]CurrencyAmount{}, false
	}
	out := make(map[string]CurrencyAmount, len(entries))
	for key, body := range entries {
		var fields map[string]json.RawMessage
		if json.Unmarshal(body, &fields) != nil {
			continue
		}
		id := key
		if s, ok := jsonString(fields["id"]); ok && s != "" {
			id = s
		}
		amount := ""
		if am, ok := fields["_am"]; ok && string(am) != "null" {
			amount = CanonicalAmount(rawNumberText(am))
		}
		name := id
		for _, k := range []string{"displayName", "DisplayName", "name", "Name"} {
			if s, ok := jsonString(fields[k]); ok && s != "" {
				name = s
				break
			}
		}
		out[id] = CurrencyAmount{Amount: amount, DisplayName: name}
	}
	return out, true
}

func rawNumberText(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			return str
		}
	}
	return s
}

func jsonString(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s, true
	}
	return "", false
}
