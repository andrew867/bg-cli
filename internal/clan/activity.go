package clan

import (
	"encoding/json"
	"strings"
)

// StatisticsFromProfile reads the profile Statistics object. known is false
// when the profile or the Statistics field is absent. Values stay exact text.
// A JSON number is copied from its raw literal, never through float64.
func StatisticsFromProfile(profile json.RawMessage) (map[string]string, bool) {
	if len(profile) == 0 || string(profile) == "null" {
		return map[string]string{}, false
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(profile, &top) != nil {
		return map[string]string{}, false
	}
	raw, ok := top["Statistics"]
	if !ok || string(raw) == "null" {
		return map[string]string{}, false
	}
	var stats map[string]json.RawMessage
	if json.Unmarshal(raw, &stats) != nil || stats == nil {
		return map[string]string{}, false
	}
	out := make(map[string]string, len(stats))
	for k, v := range stats {
		s := strings.TrimSpace(string(v))
		if s == "" || s == "null" {
			continue
		}
		if len(s) >= 2 && s[0] == '"' {
			var str string
			if json.Unmarshal(v, &str) == nil {
				if strings.TrimSpace(str) != "" {
					out[k] = strings.TrimSpace(str)
				}
				continue
			}
		}
		if n, ok := ParseExactInt(s); ok {
			out[k] = n.String()
			continue
		}
		out[k] = s
	}
	return out, true
}
