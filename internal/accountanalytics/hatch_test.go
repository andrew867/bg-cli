package accountanalytics

import (
	"math"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func loadHollowEggBundles(t *testing.T) (*snapshot.Bundle, *snapshot.Bundle) {
	t.Helper()
	fromBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "analytics", "hollow-egg-from-profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	toBytes, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "analytics", "hollow-egg-to-profile.json"))
	if err != nil {
		t.Fatal(err)
	}
	return &snapshot.Bundle{ProfileData: fromBytes}, &snapshot.Bundle{ProfileData: toBytes}
}

func TestHollowEggHatchDistribution(t *testing.T) {
	from, to := loadHollowEggBundles(t)
	rep, err := HatchReport(from, to, "Hollow Egg")
	if err != nil {
		t.Fatal(err)
	}
	if !rep.FullyReconciled {
		t.Fatalf("expected reconciled: %+v", rep)
	}
	if rep.EggPurchaseDelta != "415296" || rep.SpeciesDeltaSum != "415296" {
		t.Fatalf("deltas: egg=%s species=%s", rep.EggPurchaseDelta, rep.SpeciesDeltaSum)
	}
	want := map[string]float64{
		"Wisp Griffin":     81.43,
		"Ruinous Angelus":  18.57,
		"Knight Golem":     0.0048,
		"Veil Horse":       0.00024,
	}
	for pet, pct := range want {
		var row HatchSpeciesRow
		for _, r := range rep.Species {
			if r.PetID == pet {
				row = r
				break
			}
		}
		if row.ObservedPercent == nil {
			t.Fatalf("%s: missing percent", pet)
		}
		if math.Abs(*row.ObservedPercent-pct) > 0.02 {
			t.Fatalf("%s: got %.5f want %.5f", pet, *row.ObservedPercent, pct)
		}
		if row.WilsonLow95 == nil || row.WilsonHigh95 == nil {
			t.Fatalf("%s: missing wilson", pet)
		}
	}
}

func TestHatchSpeciesSumArithmetic(t *testing.T) {
	parts := []int64{338163, 77112, 20, 1}
	var sum int64
	for _, p := range parts {
		sum += p
	}
	if sum != 415296 {
		t.Fatalf("sum=%d", sum)
	}
}
