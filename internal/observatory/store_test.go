package observatory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestStoreRoundTripAndDiscover(t *testing.T) {
	dir := t.TempDir()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "collections-list.json"))
	if err != nil {
		t.Fatal(err)
	}
	meta := CollectionSnapshotMeta{
		OriginURL:  "https://ps99.biggamesapi.io/api/collections",
		Collection: "collections-list",
	}
	if err := SaveCollection(dir, "collections-list", raw, meta); err != nil {
		t.Fatal(err)
	}
	got, err := LoadCollection(dir, "collections-list")
	if err != nil {
		t.Fatal(err)
	}
	if string(got.Raw) != string(raw) {
		t.Fatal("raw mismatch")
	}
	list, err := ListLocalCollections(dir)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	parsed, err := ParseCollectionsList(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !parsed.HasCollection("NewDiscoveryCollection") {
		t.Fatal("unknown collection should ingest without code change")
	}
}

func TestDiffSemanticReorderVsWeight(t *testing.T) {
	from := []byte(`{"pets":[{"id":"A","weight":1},{"id":"B","weight":2}]}`)
	reorder := []byte(`{"pets":[{"id":"B","weight":2},{"id":"A","weight":1}]}`)
	weight := []byte(`{"pets":[{"id":"A","weight":1},{"id":"B","weight":3}]}`)
	dr, err := DiffCollections(from, reorder)
	if err != nil {
		t.Fatal(err)
	}
	if !dr.OK || len(dr.Changes) != 0 {
		t.Fatalf("reorder should be ok: %+v", dr)
	}
	dr2, err := DiffCollections(from, weight)
	if err != nil {
		t.Fatal(err)
	}
	if dr2.OK || len(dr2.Changes) == 0 {
		t.Fatalf("weight change expected: %+v", dr2)
	}
}

func TestParseSecretRoomEggPets(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "eggs-secret-room.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := FindEggByDisplayName(b, "Secret Room Egg")
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Pets) != 4 {
		t.Fatalf("pets=%d", len(doc.Pets))
	}
	want := map[string]struct{ weight, label string }{
		"Phantom Tiger":         {"90", ""},
		"Thorn Dragon":          {"10", ""},
		"Masquerade Agony":      {"0.0034", "Great"},
		"Huge Bat Hoodie Panda": {"0.000015", "Insane"},
	}
	for _, p := range doc.Pets {
		w, ok := want[p.ID]
		if !ok {
			t.Fatalf("unexpected pet %s", p.ID)
		}
		if p.RawWeight != w.weight || p.Label != w.label {
			t.Fatalf("%s: weight=%q label=%q", p.ID, p.RawWeight, p.Label)
		}
		if p.Semantics != WeightNotPercent && p.Semantics != WeightRawDecimal {
			t.Fatalf("semantics=%s", p.Semantics)
		}
		if p.ID == "Huge Bat Hoodie Panda" {
			if _, ok := p.Extra["huge"]; !ok {
				t.Fatal("trailing huge field should be preserved")
			}
		}
	}
}

func TestGraphHeuristicNotPromoted(t *testing.T) {
	e := Edge{
		FromCollection: "Zones", FromKey: "z1",
		ToCollection: "Eggs", ToKey: "Secret Room Egg",
		Relation: "offers_egg", Evidence: EvidenceNameHeuristic,
	}
	p := PromoteHeuristic(e)
	if p.Evidence != EvidenceUnresolved {
		t.Fatalf("promoted=%s", p.Evidence)
	}
	rep := ReportUnresolved([]Edge{e})
	if len(rep.Items) != 1 {
		t.Fatalf("items=%d", len(rep.Items))
	}
}
