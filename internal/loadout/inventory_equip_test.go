package loadout

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnchantMaxEnchantsAndListDerivedCount(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-equipped-realistic.json"))
	if err != nil {
		t.Fatal(err)
	}
	pl, err := ParseInventoryLoadout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Enchants.EquippedCount != 5 {
		t.Fatalf("equippedCount=%d want 5 (from list; API omits equippedCount)", pl.Enchants.EquippedCount)
	}
	if pl.Enchants.MaxEquipped == nil || *pl.Enchants.MaxEquipped != 6 {
		t.Fatalf("maxEnchants=%v source=%s", pl.Enchants.MaxEquipped, pl.Enchants.MaxSource)
	}
	if pl.Enchants.MaxSource != "inventory.equipped.enchants.maxEnchants" {
		t.Fatalf("source=%s", pl.Enchants.MaxSource)
	}
	if pl.Enchants.PaidCount == nil || *pl.Enchants.PaidCount != 0 {
		t.Fatalf("paidCount=%v", pl.Enchants.PaidCount)
	}
	if len(pl.Enchants.List) != 5 {
		t.Fatalf("list len=%d", len(pl.Enchants.List))
	}
}

func TestSharedUIDPreservesEquippedButlerQuantities(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-equipped-realistic.json"))
	if err != nil {
		t.Fatal(err)
	}
	pl, err := ParseInventoryLoadout(raw)
	if err != nil {
		t.Fatal(err)
	}
	if pl.Pets.EquippedCount != 5 {
		t.Fatalf("equippedCount=%d", pl.Pets.EquippedCount)
	}
	if len(pl.Pets.List) != 5 {
		t.Fatalf("list collapsed? len=%d", len(pl.Pets.List))
	}
	uids := map[string]int{}
	slots := map[string]int{}
	for _, s := range pl.Pets.List {
		if s.UIDRole != "stack-identity" {
			t.Fatalf("uidRole=%q", s.UIDRole)
		}
		uids[s.UID]++
		slots[s.Slot]++
		if s.Quantity != 1 {
			t.Fatalf("quantity=%d", s.Quantity)
		}
	}
	if uids["stack-butler-shared"] != 5 {
		t.Fatalf("expected shared uid across 5 slots, got %v", uids)
	}
	if len(slots) != 5 {
		t.Fatalf("slots collapsed: %v", slots)
	}
	byID := EquippedQuantityByID(pl.Pets.List)
	if byID["Butler Penguin"] != 5 {
		t.Fatalf("quantity by id=%v", byID)
	}
	expanded := idsOf(pl.Pets.List)
	if len(expanded) != 5 {
		t.Fatalf("idsOf deduped by uid? %v", expanded)
	}
}

func TestOptimizeUsesMaxEnchantsSlotLimit(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-equipped-realistic.json"))
	if err != nil {
		t.Fatal(err)
	}
	pl, err := ParseInventoryLoadout(raw)
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := ParseObjective("max-pet-damage")
	res, err := Optimize(DefaultRegistry(), pl, OptimizeOptions{WorldID: "general", Objective: obj, MaxCandidates: 3})
	if err != nil {
		t.Fatal(err)
	}
	if res.EnchantSlotLimit == nil || *res.EnchantSlotLimit != 6 {
		t.Fatalf("enchant slot limit=%v", res.EnchantSlotLimit)
	}
	if len(res.Candidates[0].Pets) != 5 {
		t.Fatalf("baseline pets should preserve 5 Butler entries, got %v", res.Candidates[0].Pets)
	}
}
