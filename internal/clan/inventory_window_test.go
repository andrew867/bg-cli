package clan

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestInventoryWindowDecreaseHasNoRate(t *testing.T) {
	now := time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC)
	prior := Prior{
		File:           "old-ps99-summary.json",
		ExportedAt:     now.Add(-time.Hour),
		ExportedAtText: now.Add(-time.Hour).Format(time.RFC3339),
		SessionID:      "s",
		Profile:        "main",
		Inventory: &InventoryState{
			Known: true, PetQuantity: "10", PetRows: "1", Golden: "1", Rainbow: "0", Shiny: "0",
			InventoryRows: "2", HugeKnown: true, Huge: "2",
		},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "2026-11-06-160000", Prefix: "ps99", Profile: "main", SessionID: "s",
		Inventory: &InventoryState{
			Known: true, PetQuantity: "7", PetRows: "1", Golden: "0", Rainbow: "0", Shiny: "0",
			InventoryRows: "1", HugeKnown: true, Huge: "1",
		},
		Priors: []Prior{prior},
	})
	w := sum.Windows.Previous
	if !w.Valid || w.Inventory == nil || w.Inventory.PetQuantity == nil || w.Inventory.PetQuantity.Delta != "-3" {
		t.Fatalf("pet window %+v", w.Inventory)
	}
	if w.Inventory.Huge == nil || w.Inventory.Huge.Delta != "-1" {
		t.Fatalf("huge %+v", w.Inventory.Huge)
	}
	raw, err := json.Marshal(w.Inventory)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "netPerHour") || strings.Contains(string(raw), "rateStatus") || strings.Contains(string(raw), "spend") {
		t.Fatalf("pet decrease stored as a rate: %s", raw)
	}
}
