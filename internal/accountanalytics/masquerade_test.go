package accountanalytics

import (
	"math"
	"math/big"
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestMasqueradeThornIntervals(t *testing.T) {
	f, err := LoadHatchExperiments(filepath.Join("..", "..", "testdata", "fixtures", "analytics", "masquerade-thorn-hatch-2026-11-07.json"))
	if err != nil {
		t.Fatal(err)
	}
	intervals, notes, err := CompareHatchExperiments(f)
	if err != nil {
		t.Fatal(err)
	}
	if len(notes) < 2 {
		t.Fatal("expected methodology notes")
	}
	// Find 17:09 → 18:24 aggregate via summing no-cocktail intervals or direct check
	var egg, rare int64
	for _, st := range intervals {
		if st.FromID == "thorn-170908" && st.ToID == "thorn-180002" {
			if st.EggDelta != 55610 || st.RareDelta != 18 {
				t.Fatalf("mid interval: %+v", st)
			}
		}
		if st.FromID == "thorn-180002" && st.ToID == "thorn-182415" {
			if st.EggDelta != 29882 || st.RareDelta != 9 {
				t.Fatalf("late interval: %+v", st)
			}
		}
		if st.CocktailActive != nil && !*st.CocktailActive {
			egg += st.EggDelta
			rare += st.RareDelta
		}
	}
	if egg != 85492 || rare != 27 {
		t.Fatalf("no-cocktail aggregate eggs=%d rare=%d", egg, rare)
	}
	rate := float64(rare) / float64(egg)
	// ~0.0316% displayed ⇒ fraction ≈ 0.000316
	if math.Abs(rate*100-0.0316) > 0.0002 {
		t.Fatalf("rate=%v percent=%v", rate, rate*100)
	}
	// purchases 1276 × 67 = 85492
	if 1276*67 != 85492 {
		t.Fatal("purchase arithmetic")
	}
	foundEggHatchWarn := false
	for _, st := range intervals {
		for _, w := range st.Warnings {
			if containsSub(w, "EggHatchCount") {
				foundEggHatchWarn = true
			}
		}
	}
	if !foundEggHatchWarn {
		t.Fatal("expected EggHatchCount vs 67 warning")
	}
	// Cocktail aggregate
	var cEgg, cRare int64
	for _, st := range intervals {
		if st.CocktailActive != nil && *st.CocktailActive {
			cEgg += st.EggDelta
			cRare += st.RareDelta
		}
	}
	if cEgg != 38336 || cRare != 21 {
		t.Fatalf("cocktail aggregate %d/%d", cEgg, cRare)
	}
}

func TestCandyCoinsRunwayNoFabricatedPrice(t *testing.T) {
	start := snapshot.ParseExactIntString("532110000000") // approximate ~532.11B for fixture shape
	end := snapshot.ParseExactIntString("721318533")
	rep := AnalyzeCurrencyRunway("HalloweenGamesCoins", start, end, 1276, 67, 85492, "~720000000", nil)
	if rep.EstimatedGrossSpend != "" {
		t.Fatal("must not invent spend from rounded display alone")
	}
	if len(rep.Warnings) == 0 {
		t.Fatal("need rounded-price warning")
	}
	// With exact price still check purchase×67
	price := big.NewInt(720000000)
	rep2 := AnalyzeCurrencyRunway("HalloweenGamesCoins", start, end, 1276, 67, 85492, "720000000", price)
	if rep2.EstimatedGrossSpend == "" {
		t.Fatal("exact price should estimate spend")
	}
}

func TestItemFlowDoesNotAutoLabelTrade(t *testing.T) {
	rep := BuildAnnotatedItemFlow([]FlowEvent{
		{ItemID: "Huge X", Quantity: "2", Category: FlowTradeOut, Annotation: "two Huges transferred out Nov 7", Verified: false},
	}, nil)
	if len(rep.Events) != 1 {
		t.Fatal(rep.Events)
	}
	if len(rep.Events[0].Warnings) == 0 {
		t.Fatal("unverified trade_out needs warning")
	}
	for _, n := range rep.Notes {
		if containsSub(n, "must not be labeled trade") {
			return
		}
	}
	t.Fatal("missing methodology note")
}

func containsSub(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || indexOfSub(s, sub) >= 0)
}
func indexOfSub(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
