package loadout

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

// EquippedSlot is one equipped pet or enchant reference as present in inventory.
type EquippedSlot struct {
	ID              string          `json:"id"`
	UID             string          `json:"uid,omitempty"`
	Class           string          `json:"class,omitempty"`
	RawLevel        json.RawMessage `json:"rawLevel,omitempty"`
	TierResolved    *string         `json:"tierResolved,omitempty"`
	TierStatus      string          `json:"tierStatus"` // resolved | unresolved | absent
	TierReason      string          `json:"tierReason,omitempty"`
	DisplayedPower  *string         `json:"displayedPower,omitempty"` // never labeled as DPS
	Variant         string          `json:"variant,omitempty"`
	IsHugeCandidate bool            `json:"isHugeCandidate,omitempty"`
}

// EquipGroup is pets or enchants equipped payload.
type EquipGroup struct {
	List          []EquippedSlot `json:"list"`
	EquippedCount int            `json:"equippedCount"`
	MaxEquipped   *int           `json:"maxEquipped,omitempty"`
	MaxSource     string         `json:"maxEquippedSource,omitempty"`
}

// OwnedStack is an owned inventory stack usable for substitutions.
type OwnedStack struct {
	Class   string `json:"class"`
	ID      string `json:"id"`
	Count   string `json:"count"`
	Variant string `json:"variant,omitempty"`
}

// PlayerLoadout is the parsed current loadout + owned pools.
type PlayerLoadout struct {
	Pets          EquipGroup      `json:"pets"`
	Enchants      EquipGroup      `json:"enchants"`
	Ultimate      json.RawMessage `json:"ultimate,omitempty"`
	Hoverboard    json.RawMessage `json:"hoverboard,omitempty"`
	OwnedPets     []OwnedStack    `json:"ownedPets"`
	OwnedEnchants []OwnedStack    `json:"ownedEnchants"`
	OwnedPotions  []OwnedStack    `json:"ownedPotions"`
	SnapshotAt    *string         `json:"snapshotAt,omitempty"`
	Warnings      []string        `json:"warnings"`
}

// ParseInventoryLoadout extracts equipped slots and owned pools from inventory object JSON.
func ParseInventoryLoadout(inventoryObj json.RawMessage) (PlayerLoadout, error) {
	out := PlayerLoadout{Warnings: []string{}}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inventoryObj, &root); err != nil {
		return out, fmt.Errorf("inventory: %w", err)
	}
	if fa, ok := root["fetchedAt"]; ok {
		var s string
		if json.Unmarshal(fa, &s) == nil && s != "" {
			out.SnapshotAt = &s
		}
	}

	var equipped map[string]json.RawMessage
	_ = json.Unmarshal(root["equipped"], &equipped)
	out.Pets = parseEquipGroup(equipped["pets"], "inventory.equipped.pets.maxEquipped")
	out.Enchants = parseEquipGroup(equipped["enchants"], "inventory.equipped.enchants.maxEquipped")
	out.Ultimate = equipped["ultimate"]
	out.Hoverboard = equipped["hoverboard"]

	var items []json.RawMessage
	if err := json.Unmarshal(root["items"], &items); err != nil {
		return out, fmt.Errorf("inventory items: %w", err)
	}
	for _, it := range items {
		var m map[string]json.RawMessage
		if json.Unmarshal(it, &m) != nil {
			continue
		}
		var class, id string
		_ = json.Unmarshal(m["class"], &class)
		_ = json.Unmarshal(m["id"], &id)
		count := snapshot.ParseExactInt(m["count"])
		countText := count.String()
		v := output.DeriveItemVariant(m)
		stack := OwnedStack{Class: class, ID: id, Count: countText, Variant: v.Label}
		switch {
		case strings.EqualFold(class, "Pet"):
			out.OwnedPets = append(out.OwnedPets, stack)
		case strings.EqualFold(class, "Enchant"), strings.EqualFold(class, "Enchant Book"), strings.Contains(strings.ToLower(class), "enchant"):
			out.OwnedEnchants = append(out.OwnedEnchants, stack)
		case strings.EqualFold(class, "Potion"), strings.Contains(strings.ToLower(class), "potion"):
			out.OwnedPotions = append(out.OwnedPotions, stack)
		}
	}

	if out.Pets.MaxEquipped == nil {
		out.Warnings = append(out.Warnings, "inventory did not provide equipped.pets.maxEquipped")
	}
	if out.Enchants.MaxEquipped == nil {
		out.Warnings = append(out.Warnings, "inventory did not provide equipped.enchants.maxEquipped")
	}
	return out, nil
}

func parseEquipGroup(raw json.RawMessage, maxSource string) EquipGroup {
	g := EquipGroup{MaxSource: maxSource}
	if len(raw) == 0 || string(raw) == "null" {
		return g
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return g
	}
	if mx := snapshot.ParseExactInt(top["maxEquipped"]); mx.ParseOK && mx.Int != nil && mx.Int.IsInt64() {
		v := int(mx.Int.Int64())
		g.MaxEquipped = &v
	}
	if ec := snapshot.ParseExactInt(top["equippedCount"]); ec.ParseOK && ec.Int != nil && ec.Int.IsInt64() {
		g.EquippedCount = int(ec.Int.Int64())
	}
	var list []json.RawMessage
	_ = json.Unmarshal(top["list"], &list)
	for _, e := range list {
		g.List = append(g.List, parseEquippedSlot(e))
	}
	return g
}

func parseEquippedSlot(raw json.RawMessage) EquippedSlot {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	s := EquippedSlot{TierStatus: "absent", TierReason: "no tier/level field present"}
	_ = json.Unmarshal(m["id"], &s.ID)
	_ = json.Unmarshal(m["uid"], &s.UID)
	_ = json.Unmarshal(m["class"], &s.Class)
	if s.Class == "" {
		s.Class = "unknown"
	}
	v := output.DeriveItemVariant(m)
	s.Variant = v.Label
	if strings.HasPrefix(strings.ToLower(s.ID), "huge ") || strings.EqualFold(s.ID, "Huge") || strings.Contains(strings.ToLower(s.ID), "huge") {
		s.IsHugeCandidate = true
	}

	// Displayed power only — never DPS.
	for _, key := range []string{"power", "displayedPower", "damage"} {
		if raw, ok := m[key]; ok && string(raw) != "null" {
			p := snapshot.ParseExactInt(raw)
			if p.ParseOK {
				txt := p.Text
				s.DisplayedPower = &txt
				break
			}
			// keep raw text if not exact int
			var str string
			if json.Unmarshal(raw, &str) == nil && str != "" {
				s.DisplayedPower = &str
				break
			}
			t := strings.TrimSpace(string(raw))
			if t != "" {
				s.DisplayedPower = &t
				break
			}
		}
	}

	if lvl, ok := m["level"]; ok && string(lvl) != "null" {
		s.RawLevel = append(json.RawMessage(nil), lvl...)
		s.TierStatus = "unresolved"
		s.TierReason = "equipped level present but tier mapping is not documented; level 0 is not tier I"
		// Only resolve when an explicit tier string exists.
	}
	if tier, ok := m["tier"]; ok && string(tier) != "null" {
		var tierStr string
		if json.Unmarshal(tier, &tierStr) == nil && tierStr != "" {
			s.TierResolved = &tierStr
			s.TierStatus = "resolved"
			s.TierReason = "explicit tier field present"
		} else {
			n := snapshot.ParseExactInt(tier)
			if n.ParseOK {
				s.TierStatus = "unresolved"
				s.TierReason = "numeric tier without documented mapping to roman/enchant book tier"
			}
		}
	}
	return s
}

// OwnedCount returns total owned quantity for class+id (case-insensitive).
func (p PlayerLoadout) OwnedCount(class, id string) (snapshot.ExactInt, bool) {
	var stacks []OwnedStack
	switch {
	case strings.EqualFold(class, "Pet"):
		stacks = p.OwnedPets
	case strings.Contains(strings.ToLower(class), "enchant"):
		stacks = p.OwnedEnchants
	case strings.Contains(strings.ToLower(class), "potion"):
		stacks = p.OwnedPotions
	default:
		stacks = append(append(p.OwnedPets, p.OwnedEnchants...), p.OwnedPotions...)
	}
	sum := snapshot.ExactInt{}
	found := false
	for _, s := range stacks {
		if !strings.EqualFold(s.ID, id) {
			continue
		}
		found = true
		c := snapshot.ParseExactIntString(s.Count)
		if sum.ParseOK {
			sum = snapshot.AddExact(sum, c)
		} else {
			sum = c
		}
	}
	return sum, found
}
