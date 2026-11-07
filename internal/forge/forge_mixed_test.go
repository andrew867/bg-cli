package forge

import (
	"path/filepath"
	"testing"
)

func TestMixedShardDominationWhenComparable(t *testing.T) {
	// Same luck multiplier → comparable contexts at 75.0% displayed success.
	file := ObservationFile{
		SchemaVersion: "1.0",
		Observations: []Observation{
			{
				ObservationID: "t-100", EventID: "HalloweenGames", RecipeKind: "mixed",
				SpiritShards: intPtr(100), DisplayedLuckMultiplier: floatPtr(130),
				DisplayedWinProbability: ParseDisplayedProbability("75.0%"),
				SelectedPets: []PetInput{
					{ID: "Butler Penguin", Quantity: 22},
					{ID: "Porcelain Doll Cat", Quantity: 30400},
				},
			},
			{
				ObservationID: "t-60", EventID: "HalloweenGames", RecipeKind: "mixed",
				SpiritShards: intPtr(60), DisplayedLuckMultiplier: floatPtr(130),
				DisplayedWinProbability: ParseDisplayedProbability("75.0%"),
				SelectedPets: []PetInput{
					{ID: "Butler Penguin", Quantity: 22},
					{ID: "Porcelain Doll Cat", Quantity: 29300},
				},
			},
			{
				ObservationID: "t-63", EventID: "HalloweenGames", RecipeKind: "mixed",
				SpiritShards: intPtr(63), DisplayedLuckMultiplier: floatPtr(130),
				DisplayedWinProbability: ParseDisplayedProbability("75.0%"),
				SelectedPets: []PetInput{
					{ID: "Butler Penguin", Quantity: 22},
					{ID: "Porcelain Doll Cat", Quantity: 28575},
				},
			},
		},
	}
	rep, err := OptimizeObservedMixed(file, ObservedMixedOptions{Objective: ObjMinShards})
	if err != nil {
		t.Fatal(err)
	}
	byShards := map[int]RecipeCandidate{}
	for _, c := range rep.Candidates {
		byShards[c.SpiritShards] = c
	}
	if c := byShards[100]; !c.Dominated {
		t.Fatalf("100-shard recipe should be dominated when contexts comparable: %+v", c)
	}
	if c60, c63 := byShards[60], byShards[63]; c60.Dominated || c63.Dominated {
		t.Fatalf("60 vs 63 trade shards vs porcelain; neither should dominate: 60=%v 63=%v", c60.Dominated, c63.Dominated)
	}
}

func TestMixedFixturePreservesLuckAndContextMismatch(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-mixed-shards.json"))
	if err != nil {
		t.Fatal(err)
	}
	var o100, o60, o63 Observation
	for _, o := range f.Observations {
		switch o.ObservationID {
		case "mixed-22-30400-100":
			o100 = o
		case "mixed-22-29300-60":
			o60 = o
		case "mixed-22-28575-63":
			o63 = o
		}
	}
	if o100.DisplayedLuckMultiplier == nil || *o100.DisplayedLuckMultiplier != 130 {
		t.Fatal("expected luck 130 on 100-shard row")
	}
	if o60.DisplayedLuckMultiplier == nil || *o60.DisplayedLuckMultiplier != 125 {
		t.Fatal("expected luck 125 on 60-shard row")
	}
	if o63.DisplayedLuckMultiplier == nil || *o63.DisplayedLuckMultiplier != 128 {
		t.Fatal("expected luck 128 on 63-shard row")
	}
	if !ListIncomparableMixed(o100, o60) {
		t.Fatal("different luck → incomparable despite same displayed percent")
	}
	rep, err := OptimizeObservedMixed(f, ObservedMixedOptions{Objective: ObjMinShards})
	if err != nil {
		t.Fatal(err)
	}
	// Different comparable keys: no cross-domination marking.
	if rep.Candidates[0].Dominated || rep.Candidates[1].Dominated || rep.Candidates[2].Dominated {
		t.Fatalf("fixture luck mismatch prevents dominance: %+v", rep.Candidates)
	}
}

func TestShardBudgetFilter(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-mixed-shards.json"))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := OptimizeObservedMixed(f, ObservedMixedOptions{Objective: ObjMinShards, ShardBudget: 65})
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range rep.Candidates {
		if c.SpiritShards > 65 {
			t.Fatalf("shard budget filter failed: %+v", c)
		}
	}
}

func intPtr(n int) *int           { return &n }
func floatPtr(f float64) *float64 { return &f }
