package loadout

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

// EquippedSlot is one equipped pet or enchant reference as present in inventory.
// Official docs: pets use {uid,slot,id,...}; enchants use {uid,slot,id,paid,level,...}.
//
// UID is a save stack identity and may be shared by multiple equipped Butler Penguins
// (or other stacked pets). It is NOT a reliable unique instance key. Prefer Slot for
// distinguishing equipped entries; Quantity preserves how many units this list row
// represents (normally 1 per list entry).
type EquippedSlot struct {
	ID              string          `json:"id"`
	Slot            string          `json:"slot,omitempty"`
	UID             string          `json:"uid,omitempty"`
	UIDRole         string          `json:"uidRole,omitempty"` // "stack-identity" — never "unique-instance"
	Class           string          `json:"class,omitempty"`
	Quantity        int             `json:"quantity"` // equipped units for this list entry; never collapsed by uid
	RawLevel        json.RawMessage `json:"rawLevel,omitempty"`
	TierResolved    *string         `json:"tierResolved,omitempty"`
	TierStatus      string          `json:"tierStatus"` // resolved | unresolved | absent
	TierReason      string          `json:"tierReason,omitempty"`
	DisplayedPower  *string         `json:"displayedPower,omitempty"` // never labeled as DPS
	Variant         string          `json:"variant,omitempty"`
	Paid            *bool           `json:"paid,omitempty"`
	IsHugeCandidate bool            `json:"isHugeCandidate,omitempty"`
}

// EquipGroup is pets or enchants equipped payload.
type EquipGroup struct {
	List            []EquippedSlot `json:"list"`
	EquippedCount   int            `json:"equippedCount"` // sum of entry quantities; derived from list when API omits it
	MaxEquipped     *int           `json:"maxEquipped,omitempty"`
	MaxSource       string         `json:"maxEquippedSource,omitempty"`
	PaidCount       *int           `json:"paidCount,omitempty"`
	MaxPaidEnchants *int           `json:"maxPaidEnchants,omitempty"`
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
	out.Pets = parseEquipGroup(equipped["pets"], equipKindPets)
	out.Enchants = parseEquipGroup(equipped["enchants"], equipKindEnchants)
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
		out.Warnings = append(out.Warnings, "inventory did not provide equipped.enchants.maxEnchants (or legacy maxEquipped)")
	}
	return out, nil
}

type equipKind int

const (
	equipKindPets equipKind = iota
	equipKindEnchants
)

func parseEquipGroup(raw json.RawMessage, kind equipKind) EquipGroup {
	g := EquipGroup{}
	if len(raw) == 0 || string(raw) == "null" {
		return g
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return g
	}

	switch kind {
	case equipKindEnchants:
		// Official shape: { list, paidCount, maxEnchants, maxPaidEnchants } — no equippedCount/maxEquipped.
		if mx := parseOptionalInt(top["maxEnchants"]); mx != nil {
			g.MaxEquipped = mx
			g.MaxSource = "inventory.equipped.enchants.maxEnchants"
		} else if mx := parseOptionalInt(top["maxEquipped"]); mx != nil {
			// Legacy/fixture fallback only.
			g.MaxEquipped = mx
			g.MaxSource = "inventory.equipped.enchants.maxEquipped(legacy)"
		}
		g.PaidCount = parseOptionalInt(top["paidCount"])
		g.MaxPaidEnchants = parseOptionalInt(top["maxPaidEnchants"])
	default:
		if mx := parseOptionalInt(top["maxEquipped"]); mx != nil {
			g.MaxEquipped = mx
			g.MaxSource = "inventory.equipped.pets.maxEquipped"
		}
	}

	var list []json.RawMessage
	_ = json.Unmarshal(top["list"], &list)
	for _, e := range list {
		g.List = append(g.List, parseEquippedSlot(e))
	}

	// EquippedCount: prefer sum of per-entry quantities from the list. Official pet docs
	// say equippedCount equals len(list); enchant payloads omit equippedCount entirely.
	qtySum := 0
	for _, s := range g.List {
		qtySum += s.Quantity
	}
	g.EquippedCount = qtySum
	if ec := parseOptionalInt(top["equippedCount"]); ec != nil && *ec != qtySum && qtySum > 0 {
		// Keep list-derived count as source of truth when they disagree.
		g.MaxSource = strings.TrimSpace(g.MaxSource + " ;equippedCountIgnoredInFavorOfListQuantity")
	}
	return g
}

func parseOptionalInt(raw json.RawMessage) *int {
	n := snapshot.ParseExactInt(raw)
	if !n.ParseOK || n.Int == nil || !n.Int.IsInt64() {
		return nil
	}
	v := int(n.Int.Int64())
	return &v
}

func parseEquippedSlot(raw json.RawMessage) EquippedSlot {
	var m map[string]json.RawMessage
	_ = json.Unmarshal(raw, &m)
	s := EquippedSlot{
		TierStatus: "absent",
		TierReason: "no tier/level field present",
		Quantity:   1,
		UIDRole:    "stack-identity",
	}
	_ = json.Unmarshal(m["id"], &s.ID)
	_ = json.Unmarshal(m["uid"], &s.UID)
	_ = json.Unmarshal(m["slot"], &s.Slot)
	_ = json.Unmarshal(m["class"], &s.Class)
	if s.Class == "" {
		s.Class = "unknown"
	}
	// Optional count/_am on an equipped row — still one list entry identity via slot.
	if c := snapshot.ParseExactInt(m["count"]); c.ParseOK && c.Int != nil && c.Int.Sign() > 0 && c.Int.IsInt64() {
		s.Quantity = int(c.Int.Int64())
	} else if c := snapshot.ParseExactInt(m["_am"]); c.ParseOK && c.Int != nil && c.Int.Sign() > 0 && c.Int.IsInt64() {
		s.Quantity = int(c.Int.Int64())
	}
	v := output.DeriveItemVariant(m)
	s.Variant = v.Label
	if pt := firstBool(m["golden"]); pt {
		if s.Variant == "" || s.Variant == "Regular" {
			s.Variant = "Golden"
		}
	}
	if firstBool(m["rainbow"]) && (s.Variant == "" || s.Variant == "Regular") {
		s.Variant = "Rainbow"
	}
	if firstBool(m["shiny"]) {
		switch s.Variant {
		case "Golden":
			s.Variant = "Golden Shiny"
		case "Rainbow":
			s.Variant = "Rainbow Shiny"
		case "", "Regular":
			s.Variant = "Shiny"
		}
	}
	if strings.HasPrefix(strings.ToLower(s.ID), "huge ") || strings.EqualFold(s.ID, "Huge") || strings.Contains(strings.ToLower(s.ID), "huge") {
		s.IsHugeCandidate = true
	}
	if paid, ok := m["paid"]; ok && string(paid) != "null" {
		var b bool
		if json.Unmarshal(paid, &b) == nil {
			s.Paid = &b
		}
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

func firstBool(vals ...json.RawMessage) bool {
	for _, v := range vals {
		var b bool
		if json.Unmarshal(v, &b) == nil && b {
			return true
		}
	}
	return false
}

// EquippedQuantityByID sums equipped quantities by pet/enchant id without UID dedupe.
func EquippedQuantityByID(slots []EquippedSlot) map[string]int {
	out := map[string]int{}
	for _, s := range slots {
		q := s.Quantity
		if q < 1 {
			q = 1
		}
		out[s.ID] += q
	}
	return out
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
