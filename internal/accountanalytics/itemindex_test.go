package accountanalytics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestParseItemIndexVariantKeys(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-item-index-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	parsed, err := ParseItemIndex(env["data"])
	if err != nil {
		t.Fatal(err)
	}
	pet := parsed.Classes["Pet"]
	if pet == nil || !pet.Countable || len(pet.Variants) != 2 {
		t.Fatalf("pet bucket: %+v", pet)
	}
	foundRegular, foundGolden := false, false
	for _, vr := range pet.Variants {
		if vr.BaseID == "Test Cat" && vr.Variant == "Regular" {
			foundRegular = true
		}
		if vr.Variant == "Golden" && vr.PT == "1" {
			foundGolden = true
		}
	}
	if !foundRegular || !foundGolden {
		t.Fatalf("variants: %+v", pet.Variants)
	}
	egg := parsed.Classes["Egg"]
	if egg == nil || len(egg.Flags) != 1 || !egg.Flags[0].Owned {
		t.Fatalf("egg bool bucket: %+v", egg)
	}
}

func TestDiffIndexesAndCompletion(t *testing.T) {
	a, _ := ParseItemIndex(json.RawMessage(`{"Pet":{"{\"id\":\"A\"}":1},"Egg":{"1":true}}`))
	b, _ := ParseItemIndex(json.RawMessage(`{"Pet":{"{\"id\":\"A\"}":2,"{\"id\":\"B\"}":1},"Egg":{"1":true}}`))
	diff := DiffIndexes(a, b)
	if len(diff.Rows) < 2 {
		t.Fatalf("diff rows: %+v", diff.Rows)
	}
	cr := CompletionReport(b, []CatalogPet{{ID: "A"}, {ID: "B"}, {ID: "C"}})
	if cr.Denominator != 3 || cr.Discovered != 2 || cr.Percent < 66.6 || cr.Percent > 66.7 {
		t.Fatalf("%+v", cr)
	}
	if len(cr.Missing) != 1 || cr.Missing[0] != "C" {
		t.Fatalf("missing: %v", cr.Missing)
	}
}

func TestAnalyzeIndexFromBundle(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-item-index-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]json.RawMessage
	_ = json.Unmarshal(raw, &env)
	b := &snapshot.Bundle{
		ViewRaw: map[string]json.RawMessage{
			"itemIndex": env["data"],
		},
	}
	rep, err := AnalyzeIndex(b)
	if err != nil {
		t.Fatal(err)
	}
	if rep.VariantCount != 2 || rep.BoolCount != 2 {
		t.Fatalf("%+v", rep)
	}
}
