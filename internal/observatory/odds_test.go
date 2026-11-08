package observatory

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func loadSecretRoomEgg(t *testing.T) EggDocument {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "eggs-secret-room.json"))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := FindEggByDisplayName(b, "Secret Room Egg")
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func TestDirectPercentNotSilentForTinyWeights(t *testing.T) {
	doc := loadSecretRoomEgg(t)
	rep, err := ComputeEggOdds(doc, ModelDirectPercent)
	if err != nil {
		t.Fatal(err)
	}
	var agony PetOddsLine
	for _, ln := range rep.Lines {
		if ln.PetID == "Masquerade Agony" {
			agony = ln
		}
	}
	if agony.EffectiveProb != 0.0034/100 {
		t.Fatalf("direct-percent Agony: got %g want %g", agony.EffectiveProb, 0.0034/100)
	}
	if agony.Interpretation != LabelHypothetical {
		t.Fatalf("label=%s", agony.Interpretation)
	}
	for _, ln := range rep.Lines {
		if !ln.NotVerifiedLiveOdds || !EnsureNeverVerifiedLive(string(ln.Interpretation)) {
			t.Fatalf("must not claim verified live: %+v", ln)
		}
	}
}

func TestNormalizedWeightSecretRoom(t *testing.T) {
	doc := loadSecretRoomEgg(t)
	rep, err := ComputeEggOdds(doc, ModelNormalizedWeight)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]float64{
		"Phantom Tiger":         90 / 100.003415,
		"Thorn Dragon":          10 / 100.003415,
		"Masquerade Agony":      0.0034 / 100.003415,
		"Huge Bat Hoodie Panda": 0.000015 / 100.003415,
	}
	for pet, w := range want {
		p, err := EffectiveProbability(doc, ModelNormalizedWeight, pet)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(p-w) > 1e-12 {
			t.Fatalf("%s: %g vs %g", pet, p, w)
		}
	}
	_ = rep
}

func TestIndependentRareGateDoesNotFoldRares(t *testing.T) {
	doc := loadSecretRoomEgg(t)
	rep, err := ComputeEggOdds(doc, ModelIndependentRareGate)
	if err != nil {
		t.Fatal(err)
	}
	var thorn, agony float64
	for _, ln := range rep.Lines {
		switch ln.PetID {
		case "Thorn Dragon":
			thorn = ln.EffectiveProb
		case "Masquerade Agony":
			agony = ln.EffectiveProb
		}
	}
	if math.Abs(thorn-0.1) > 1e-9 {
		t.Fatalf("thorn common pool=%g want 0.1", thorn)
	}
	if math.Abs(agony-0.0034) > 1e-9 {
		t.Fatalf("agony independent=%g want 0.0034", agony)
	}
}

func TestThornBoost235Percent(t *testing.T) {
	m := ThornBoostMultiplier(90, 10, 0.235)
	if math.Abs(m-2.764705882) > 0.002 {
		t.Fatalf("m=%g want ~2.765", m)
	}
	doc := loadSecretRoomEgg(t)
	rep, err := OddsWithWeightMultiplier(doc, "Thorn Dragon", m)
	if err != nil {
		t.Fatal(err)
	}
	var thorn float64
	for _, ln := range rep.Lines {
		if ln.PetID == "Thorn Dragon" {
			thorn = ln.EffectiveProb
		}
	}
	if math.Abs(thorn-0.235) > 0.001 {
		t.Fatalf("thorn effective=%g want 0.235", thorn)
	}
}

func TestPAtLeastOneStableHuge(t *testing.T) {
	p := 0.000015 // literal fraction scenario
	n := 51724.0
	got := PAtLeastOne(p, n)
	if got <= 0 || math.IsNaN(got) || got >= 1 {
		t.Fatalf("P=%v", got)
	}
	// Compare with naive for sanity
	naive := 1 - math.Pow(1-p, n)
	if math.Abs(got-naive) > 1e-9 {
		t.Fatalf("stable=%g naive=%g", got, naive)
	}
}

func TestEggsForConfidence(t *testing.T) {
	p := 0.0034
	n, err := EggsForConfidence(p, 0.5)
	if err != nil || n <= 0 {
		t.Fatalf("n=%v err=%v", n, err)
	}
	if PAtLeastOne(p, n) < 0.49 {
		t.Fatalf("confidence not met at n=%v", n)
	}
}

func TestSensitivityAgonyHuge(t *testing.T) {
	doc := loadSecretRoomEgg(t)
	sens := SensitivityReport(doc, map[string][]float64{
		"Masquerade Agony":      {1, 2, 5, 10},
		"Huge Bat Hoodie Panda": {1, 2, 4.7},
	})
	if len(sens["Masquerade Agony"]) != 4 || len(sens["Huge Bat Hoodie Panda"]) != 3 {
		t.Fatalf("sens rows: agony=%d huge=%d", len(sens["Masquerade Agony"]), len(sens["Huge Bat Hoodie Panda"]))
	}
	for _, rows := range sens {
		for _, r := range rows {
			if r.Label != LabelHypothetical {
				t.Fatalf("sensitivity must be hypothetical: %+v", r)
			}
		}
	}
}

func TestOneSidedZeroSuccess(t *testing.T) {
	ub := OneSidedUpperBound(0, 51724, 0.95)
	if ub <= 0 || ub >= 1 {
		t.Fatalf("upper=%v", ub)
	}
	// observed rate must not be claimed as zero probability
	if ObservedRate(0, 51724) != 0 {
		t.Fatal()
	}
}
