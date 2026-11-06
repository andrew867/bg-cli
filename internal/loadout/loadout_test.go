package loadout

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixtureInventory(t *testing.T) json.RawMessage {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-loadout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(b, &top); err != nil {
		t.Fatal(err)
	}
	return top["data"]
}

func TestParseLoadoutTierAndDisplayedPower(t *testing.T) {
	pl, err := ParseInventoryLoadout(fixtureInventory(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Pets.List) != 1 || pl.Pets.List[0].DisplayedPower == nil || *pl.Pets.List[0].DisplayedPower != "50000" {
		t.Fatalf("pets=%+v", pl.Pets.List)
	}
	if !pl.Pets.List[0].IsHugeCandidate {
		t.Fatal("expected huge candidate")
	}
	if len(pl.Enchants.List) != 1 {
		t.Fatal(pl.Enchants.List)
	}
	enc := pl.Enchants.List[0]
	if enc.TierStatus != "unresolved" {
		t.Fatalf("level 0 must not resolve to tier I: %+v", enc)
	}
	if pl.Pets.MaxEquipped == nil || *pl.Pets.MaxEquipped != 8 {
		t.Fatalf("max pets=%v", pl.Pets.MaxEquipped)
	}
	if pl.Enchants.MaxEquipped == nil || *pl.Enchants.MaxEquipped != 5 {
		t.Fatalf("max enchants=%v", pl.Enchants.MaxEquipped)
	}
	if _, ok := pl.OwnedCount("Enchant", "Strong Pets"); !ok {
		t.Fatal("owned Strong Pets missing")
	}
}

func TestNeverInventPetDamageScore(t *testing.T) {
	pl, err := ParseInventoryLoadout(fixtureInventory(t))
	if err != nil {
		t.Fatal(err)
	}
	obj, _ := ParseObjective("max-pet-damage")
	est, err := AnalyzeLoadout(DefaultRegistry(), pl, AnalyzeOptions{WorldID: "halloween-games", Objective: obj})
	if err != nil {
		t.Fatal(err)
	}
	if est.ScoreFullyResolved {
		t.Fatal("must not claim fully resolved damage with unknown multipliers")
	}
	if est.NumericScore != nil {
		t.Fatal("must not invent numeric DPS/score")
	}
	if len(est.UnresolvedIDs) == 0 {
		t.Fatal("expected unresolved factors")
	}
}

func TestCurrencyChannelsIndependent(t *testing.T) {
	reg := DefaultRegistry()
	coins := reg.EffectsForCurrency("Coins")
	hallow := reg.EffectsForCurrency("HalloweenGamesCoins")
	if len(coins) == 0 || len(hallow) == 0 {
		t.Fatal("expected per-currency registry rows")
	}
	for _, e := range coins {
		for _, c := range e.ApplicableCurrencies {
			if c == "HalloweenGamesCoins" {
				t.Fatal("coins effect must not list HalloweenGamesCoins")
			}
		}
	}
	pl, _ := ParseInventoryLoadout(fixtureInventory(t))
	obj, _ := ParseObjective("halloween-coins-per-min")
	est, err := AnalyzeLoadout(reg, pl, AnalyzeOptions{WorldID: "halloween-games", Objective: obj})
	if err != nil {
		t.Fatal(err)
	}
	if est.ChannelID != "HalloweenGamesCoins" {
		t.Fatalf("channel=%s", est.ChannelID)
	}
	if est.NumericScore != nil {
		t.Fatal("unknown halloween coin multipliers must not invent a score")
	}
}

func TestHalloweenWorldDoesNotAssumeGeneralPetSlots(t *testing.T) {
	pl, _ := ParseInventoryLoadout(fixtureInventory(t))
	obj, _ := ParseObjective("halloween-coins-per-min")
	res, err := Optimize(DefaultRegistry(), pl, OptimizeOptions{
		WorldID: "halloween-games", Objective: obj, MaxCandidates: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.PetSlotLimit != nil {
		t.Fatalf("halloween pet slot limit must stay unknown without verified registry value; got %v", *res.PetSlotLimit)
	}
	found := false
	for _, w := range res.Warnings {
		if contains(w, "unknown") || contains(w, "not applied") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected warning about unknown halloween slots: %v", res.Warnings)
	}
}

func TestOptimizeOnlyOwnedAndRespectsEnchantSlots(t *testing.T) {
	pl, _ := ParseInventoryLoadout(fixtureInventory(t))
	obj, _ := ParseObjective("max-pet-damage")
	res, err := Optimize(DefaultRegistry(), pl, OptimizeOptions{
		WorldID: "general", Objective: obj, MaxCandidates: 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.PetSlotLimit == nil || *res.PetSlotLimit != 8 {
		t.Fatalf("general world should use inventory maxEquipped: %v", res.PetSlotLimit)
	}
	for _, c := range res.Candidates[1:] {
		for _, s := range c.Substitutions {
			if s.AddID == "" {
				t.Fatal("empty add")
			}
			if _, ok := pl.OwnedCount("Enchant", s.AddID); !ok {
				t.Fatalf("recommended unavailable enchant %q", s.AddID)
			}
		}
		if len(c.Enchants) > 5 {
			t.Fatalf("exceeded enchant slots: %v", c.Enchants)
		}
		if c.MarginalGainJustified || c.ExpectedMarginalGain != nil {
			t.Fatal("must not invent justified marginal gains without verified chain")
		}
		if c.SuggestedTest == "" {
			t.Fatal("expected controlled test suggestion")
		}
	}
}

func TestAssumedMagnitudeStillLabeled(t *testing.T) {
	reg := DefaultRegistry()
	mag := 1.1
	reg.Effects = append(reg.Effects, EffectEntry{
		ID: "Strong Pets", Category: CategoryPetDamage, Magnitude: &mag,
		Stacking: StackMultiplicative, ApplicableWorlds: []string{"general"},
		Confidence: ConfidenceAssumed, Source: "test fixture assumed",
		Notes: "provisional",
	})
	pl, _ := ParseInventoryLoadout(fixtureInventory(t))
	// Equip Strong Pets instead
	pl.Enchants.List = []EquippedSlot{{ID: "Strong Pets", TierStatus: "absent"}}
	obj, _ := ParseObjective("max-pet-damage")
	est, err := AnalyzeLoadout(reg, pl, AnalyzeOptions{WorldID: "general", Objective: obj})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, f := range est.Factors {
		if f.ID == "Strong Pets" && f.Status == FactorResolved && f.Confidence == ConfidenceAssumed {
			found = true
		}
	}
	if !found {
		t.Fatalf("assumed factor missing: %+v", est.Factors)
	}
	// Still unresolved due to huge / other unknowns
	if est.ScoreFullyResolved {
		t.Fatal("overall score should remain unresolved while other factors unknown")
	}
}

func contains(s, sub string) bool { return strings.Contains(s, sub) }
