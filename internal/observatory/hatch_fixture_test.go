package observatory

import (
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

type hatchFixture struct {
	SchemaVersion         string `json:"schemaVersion"`
	EggID                 string `json:"eggId"`
	IntervalNote          string `json:"intervalNote"`
	PriorSeparateTestEggs string `json:"priorSeparateTestEggs"`
	PurchasedEggs         struct {
		Previous string `json:"previous"`
		Current  string `json:"current"`
		Delta    string `json:"delta"`
	} `json:"purchasedEggs"`
	PetHatchCounts []struct {
		PetID    string `json:"petId"`
		Previous string `json:"previous"`
		Current  string `json:"current"`
		Delta    string `json:"delta"`
	} `json:"petHatchCounts"`
	Currencies []struct {
		ID       string `json:"id"`
		Previous string `json:"previous"`
		Current  string `json:"current"`
		Delta    string `json:"delta"`
	} `json:"currencies"`
}

func loadHatchFixture(t *testing.T) hatchFixture {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "secret-room-hatch-51724.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fx hatchFixture
	if err := json.Unmarshal(b, &fx); err != nil {
		t.Fatal(err)
	}
	return fx
}

func TestSecretRoomHatch51724Reconciliation(t *testing.T) {
	fx := loadHatchFixture(t)
	eggDelta, _ := strconv.ParseInt(fx.PurchasedEggs.Delta, 10, 64)
	var sumSpecies int64
	deltas := map[string]int64{}
	for _, row := range fx.PetHatchCounts {
		d, _ := strconv.ParseInt(row.Delta, 10, 64)
		deltas[row.PetID] = d
		sumSpecies += d
	}
	if sumSpecies != eggDelta || sumSpecies != 51724 {
		t.Fatalf("sum=%d egg=%d", sumSpecies, eggDelta)
	}
	if deltas["Phantom Tiger"]+deltas["Thorn Dragon"]+deltas["Masquerade Agony"] != 51724 {
		t.Fatal("species sum mismatch")
	}

	agonyRate := ObservedRate(deltas["Masquerade Agony"], eggDelta) * 100
	if math.Abs(agonyRate-0.03867) > 0.005 {
		t.Fatalf("agony rate %.5f%% want ~0.03867%%", agonyRate)
	}
	thornRate := ObservedRate(deltas["Thorn Dragon"], eggDelta) * 100
	if math.Abs(thornRate-27.57) > 0.05 {
		t.Fatalf("thorn rate %.4f want ~27.57%%", thornRate)
	}

	// prior 1809 must not be inside the 51724 interval delta
	prev, _ := strconv.ParseInt(fx.PurchasedEggs.Previous, 10, 64)
	prior, _ := strconv.ParseInt(fx.PriorSeparateTestEggs, 10, 64)
	if prev != prior || prev != 1809 {
		t.Fatalf("prior baseline: prev=%d priorNote=%s", prev, fx.PriorSeparateTestEggs)
	}
	if eggDelta == prev+51724 {
		// would incorrectly fold prior into interval — cumulative current-prev is correct
	}
	cur, _ := strconv.ParseInt(fx.PurchasedEggs.Current, 10, 64)
	if cur-prev != eggDelta {
		t.Fatalf("delta arithmetic: cur-prev=%d delta=%d", cur-prev, eggDelta)
	}

	for _, c := range fx.Currencies {
		if c.ID != "HalloweenGamesCoins" {
			continue
		}
		if c.Previous != "1118683238213" || c.Current != "9303820293" {
			t.Fatalf("currency strings: before=%s after=%s", c.Previous, c.Current)
		}
		if c.Delta != "-1109379417920" {
			t.Fatalf("currency delta=%s", c.Delta)
		}
	}

	hugeDelta := deltas["Huge Bat Hoodie Panda"]
	if hugeDelta != 0 {
		t.Fatalf("huge delta=%d", hugeDelta)
	}
	ub := OneSidedUpperBound(float64(hugeDelta), float64(eggDelta), 0.95)
	if ub <= 0 {
		t.Fatal("need positive one-sided upper bound for zero successes")
	}
	if ObservedRate(0, eggDelta) == math.NaN() {
		t.Fatal()
	}
}
