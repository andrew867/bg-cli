package snapshot

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
)

// Bundle is a loaded snapshot (manifest + decoded payloads).
type Bundle struct {
	Manifest     *Manifest
	BaseDir      string
	InventoryEnv map[string]json.RawMessage // top-level envelope fields as raw
	ProfileEnv   map[string]json.RawMessage
	ExtEnv       map[string]json.RawMessage
	Inventory    json.RawMessage // data.inventory object
	ProfileData  json.RawMessage // data.profile object
	// ViewData holds decoded envelope data maps keyed by view id (itemIndex, trades, …).
	ViewData map[string]map[string]json.RawMessage
	// ViewRaw holds raw view payload objects when present under data.<view>.
	ViewRaw  map[string]json.RawMessage
	Warnings []string
}

// ProfileView is extracted profile analytics fields.
type ProfileView struct {
	Statistics    map[string]ExactInt
	PurchasedEggs map[string]ExactInt
	PetHatchCount map[string]ExactInt
	Currencies    map[string]ExactInt // key -> _am
	EggsHatched   ExactInt            // top-level EggsHatched if present
	HasStatistics bool
	HasPurchased  bool
	HasPetHatch   bool
	HasCurrencies bool
}

// PetStack is one owned pet inventory stack identity.
type PetStack struct {
	ID      string
	Variant string
	Count   ExactInt
	Class   string
}

func ExtractProfileView(profileData json.RawMessage) (ProfileView, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(profileData, &top); err != nil {
		return ProfileView{}, fmt.Errorf("profile data: %w", err)
	}
	pv := ProfileView{
		Statistics:    map[string]ExactInt{},
		PurchasedEggs: map[string]ExactInt{},
		PetHatchCount: map[string]ExactInt{},
		Currencies:    map[string]ExactInt{},
	}
	if raw, ok := top["Statistics"]; ok && string(raw) != "null" {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			pv.HasStatistics = true
			for k, v := range m {
				pv.Statistics[k] = ParseExactInt(v)
			}
		}
	}
	if raw, ok := top["PurchasedEggs"]; ok && string(raw) != "null" {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			pv.HasPurchased = true
			for k, v := range m {
				// value may be bare number or object
				pv.PurchasedEggs[k] = parseMaybeNestedCount(v)
			}
		}
	}
	if raw, ok := top["PetHatchCount"]; ok && string(raw) != "null" {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			pv.HasPetHatch = true
			for k, v := range m {
				pv.PetHatchCount[k] = parseMaybeNestedCount(v)
			}
		}
	}
	if raw, ok := top["EggsHatched"]; ok {
		pv.EggsHatched = ParseExactInt(raw)
	}
	if raw, ok := top["Currency"]; ok && string(raw) != "null" {
		var m map[string]json.RawMessage
		if json.Unmarshal(raw, &m) == nil {
			pv.HasCurrencies = true
			for k, v := range m {
				var fields map[string]json.RawMessage
				if json.Unmarshal(v, &fields) == nil {
					pv.Currencies[k] = ParseExactInt(fields["_am"])
				} else {
					pv.Currencies[k] = ParseExactInt(v)
				}
			}
		}
	}
	return pv, nil
}

func parseMaybeNestedCount(v json.RawMessage) ExactInt {
	s := strings.TrimSpace(string(v))
	if len(s) > 0 && s[0] == '{' {
		var fields map[string]json.RawMessage
		if json.Unmarshal(v, &fields) == nil {
			if am, ok := fields["_am"]; ok {
				return ParseExactInt(am)
			}
			if c, ok := fields["count"]; ok {
				return ParseExactInt(c)
			}
		}
	}
	return ParseExactInt(v)
}

// ExtractPetStacks reads Pet-class inventory items; equipped lists are ignored.
func ExtractPetStacks(inventoryObj json.RawMessage) ([]PetStack, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inventoryObj, &root); err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(root["items"], &items); err != nil {
		return nil, fmt.Errorf("inventory items: %w", err)
	}
	out := make([]PetStack, 0, len(items))
	for _, it := range items {
		var m map[string]json.RawMessage
		if json.Unmarshal(it, &m) != nil {
			continue
		}
		var class, id string
		_ = json.Unmarshal(m["class"], &class)
		_ = json.Unmarshal(m["id"], &id)
		if !strings.EqualFold(class, "Pet") {
			continue
		}
		v := output.DeriveItemVariant(m)
		label := v.Label
		if label == "" {
			label = "Regular"
		}
		out = append(out, PetStack{
			ID:      id,
			Variant: label,
			Count:   ParseExactInt(m["count"]),
			Class:   class,
		})
	}
	return out, nil
}

// AggregatePets sums counts by id and by id+variant.
func AggregatePets(stacks []PetStack, petFilter string) (byID map[string]ExactInt, byVariant map[string]map[string]ExactInt) {
	byID = map[string]ExactInt{}
	byVariant = map[string]map[string]ExactInt{}
	for _, s := range stacks {
		if petFilter != "" && !strings.EqualFold(s.ID, petFilter) {
			continue
		}
		byID[s.ID] = AddExact(zeroIfMissing(byID[s.ID]), s.Count)
		if byVariant[s.ID] == nil {
			byVariant[s.ID] = map[string]ExactInt{}
		}
		byVariant[s.ID][s.Variant] = AddExact(zeroIfMissing(byVariant[s.ID][s.Variant]), s.Count)
	}
	return byID, byVariant
}

func zeroIfMissing(e ExactInt) ExactInt {
	if e.ParseOK && e.Int != nil {
		return e
	}
	z := ExactInt{Int: bigInt0(), Text: "0", ParseOK: true, SourceText: "0"}
	return z
}

func bigInt0() *big.Int { return new(big.Int) }

// EnvelopeData extracts the data object from a CLI envelope JSON.
func EnvelopeData(envBytes []byte) (map[string]json.RawMessage, json.RawMessage, error) {
	var top map[string]json.RawMessage
	if err := json.Unmarshal(envBytes, &top); err != nil {
		return nil, nil, err
	}
	dataRaw, ok := top["data"]
	if !ok {
		return top, nil, fmt.Errorf("envelope missing data")
	}
	var data map[string]json.RawMessage
	if err := json.Unmarshal(dataRaw, &data); err != nil {
		return top, dataRaw, err
	}
	return data, dataRaw, nil
}
