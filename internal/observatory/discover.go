package observatory

import (
	"encoding/json"
	"fmt"
)

// CollectionsList is the parsed /api/collections response (wrapper or bare array).
type CollectionsList struct {
	Status string   `json:"status,omitempty"`
	Names  []string `json:"names"`
}

// ParseCollectionsList ingests the public collections list without hard-coding names.
func ParseCollectionsList(raw []byte) (CollectionsList, error) {
	var env struct {
		Status string          `json:"status"`
		Data   json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &env); err != nil {
		return CollectionsList{}, err
	}
	payload := raw
	if len(env.Data) > 0 {
		payload = env.Data
	}
	var names []string
	if err := json.Unmarshal(payload, &names); err != nil {
		// Some deployments may return an object map — collect keys.
		var m map[string]json.RawMessage
		if err2 := json.Unmarshal(payload, &m); err2 != nil {
			return CollectionsList{}, fmt.Errorf("collections list: %w", err)
		}
		for k := range m {
			names = append(names, k)
		}
	}
	out := CollectionsList{Status: env.Status, Names: names}
	if out.Status == "" {
		out.Status = "ok"
	}
	return out, nil
}

// HasCollection reports whether name is in the list (exact match).
func (c CollectionsList) HasCollection(name string) bool {
	for _, n := range c.Names {
		if n == name {
			return true
		}
	}
	return false
}
