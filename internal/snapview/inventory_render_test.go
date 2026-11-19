package snapview

import (
	"strings"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestRenderInventoryNetChanges(t *testing.T) {
	huge := true
	diff := &snapshot.InventoryDiff{
		Status: snapshot.DiffStatusOK,
		Aggregates: snapshot.InventoryAggregateSet{
			PetQuantity:   snapshot.CountPair{Previous: "10", Current: "15", Delta: "5"},
			PetRows:       snapshot.CountPair{Previous: "2", Current: "3", Delta: "1"},
			Golden:        snapshot.CountPair{Previous: "1", Current: "4", Delta: "3"},
			Rainbow:       snapshot.CountPair{Previous: "0", Current: "0", Delta: "0"},
			Shiny:         snapshot.CountPair{Previous: "1", Current: "2", Delta: "1"},
			Huge:          snapshot.CountPair{Status: snapshot.DiffStatusUnavailable, Reason: "huge classification is absent"},
			InventoryRows: snapshot.CountPair{Previous: "4", Current: "5", Delta: "1"},
		},
		PetSpeciesChanges: []snapshot.SpeciesChange{
			{ID: "Orchard Pony", DisplayName: "Orchard Pony", Previous: "100", Current: "125", Delta: "25", Classification: snapshot.ClassNetInventoryChange},
			{ID: "Huge Cat", DisplayName: "Huge Cat", Previous: "0", Current: "1", Delta: "1", Classification: snapshot.ClassAddedToInventory},
			{ID: "Six", DisplayName: "Six", Delta: "1", Classification: snapshot.ClassAddedToInventory},
			{ID: "Five", DisplayName: "Five", Delta: "1", Classification: snapshot.ClassAddedToInventory},
			{ID: "Four", DisplayName: "Four", Delta: "1", Classification: snapshot.ClassAddedToInventory},
			{ID: "Three", DisplayName: "Three", Delta: "1", Classification: snapshot.ClassAddedToInventory},
		},
		ItemChanges: []snapshot.ItemChange{
			{Class: "Currency", ID: "TestCoins", DisplayName: "Test Coins", Previous: "5", Current: "9", Delta: "4", Classification: snapshot.ClassNetInventoryChange},
			{Class: "Fruit", ID: "Apple", DisplayName: "Apple", Previous: "2", Current: "0", Delta: "-2", Classification: snapshot.ClassRemovedFromInventory},
			{Class: "Pet", ID: "Huge Cat", DisplayName: "Huge Cat", Previous: "0", Current: "1", Delta: "1", Classification: snapshot.ClassAddedToInventory, Huge: &huge},
		},
		Diagnostics: &snapshot.InventoryDiagnostics{PetQuantityDelta: "5", EggsOpenedDelta: "9", PetMinusEggs: "-4"},
	}
	text := Render(View{InventoryDiff: diff})
	inv := section(t, text, "\nInventory\n", "\nData\n")
	for _, want := range []string{"Orchard Pony", "+25", "net inventory change", "Huge Cat", "added to inventory", "Apple", "removed from inventory", "1 more changed entries"} {
		if !strings.Contains(inv, want) {
			t.Fatalf("missing %q\n%s", want, inv)
		}
	}
	if strings.Contains(inv, "Test Coins") || strings.Contains(inv, "TestCoins") {
		t.Fatalf("currency leaked into inventory changes:\n%s", inv)
	}
	for _, banned := range []string{"sold", "hatched", "deleted", "traded", "consumed", "earned", "spent", "gained from egg"} {
		if strings.Contains(strings.ToLower(inv), banned) {
			t.Fatalf("inventory wording inferred a cause %q\n%s", banned, inv)
		}
	}
	if strings.Contains(inv, "Pet quantity and eggs") {
		t.Fatalf("diagnostic leaked into the default summary:\n%s", inv)
	}
	if strings.Contains(inv, "Three") {
		t.Fatalf("sixth species was not truncated:\n%s", inv)
	}
	verbose := Render(View{InventoryDiff: diff, Verbose: true})
	if !strings.Contains(verbose, "Three") || !strings.Contains(verbose, "Pet quantity and eggs") || !strings.Contains(verbose, "Difference") {
		t.Fatalf("verbose omitted detail:\n%s", verbose)
	}

	missing := *diff
	missing.Status = snapshot.DiffStatusUnavailable
	missing.Reason = "no previous snapshot"
	missing.PetSpeciesChanges = nil
	missing.ItemChanges = nil
	missing.Aggregates = snapshot.InventoryAggregateSet{
		PetQuantity: snapshot.CountPair{Current: "15", Status: snapshot.DiffStatusUnavailable, Reason: "no previous snapshot"},
		Huge:        snapshot.CountPair{Status: snapshot.DiffStatusUnavailable, Reason: "huge classification is absent"},
	}
	gone := Render(View{InventoryDiff: &missing})
	body := section(t, gone, "\nInventory\n", "\nData\n")
	if strings.Contains(body, "none") || strings.Contains(body, "+0") || strings.Contains(body, " 0") {
		t.Fatalf("missing previous looked like a zero change:\n%s", body)
	}
	if !strings.Contains(body, "no previous snapshot") || !strings.Contains(body, "15") {
		t.Fatalf("current total or reason missing:\n%s", body)
	}
}

func section(t *testing.T, text, start, end string) string {
	t.Helper()
	i := strings.Index(text, start)
	j := strings.Index(text, end)
	if i < 0 || j < 0 || j < i {
		t.Fatalf("section %q..%q missing\n%s", start, end, text)
	}
	return text[i:j]
}
