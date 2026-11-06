package snapshot

import (
	"math"
	"testing"
)

func TestWilsonAndProbStable(t *testing.T) {
	lo, hi := wilsonScoreInterval(0, 100, 1.959963984540054)
	if lo < 0 || hi <= 0 || hi > 1 || lo > hi {
		t.Fatalf("zero-success wilson: %v %v", lo, hi)
	}
	p := probAtLeastOne(0.0004818, 10000)
	if p <= 0 || p >= 1 || math.IsNaN(p) {
		t.Fatalf("prob=%v", p)
	}
}

func TestHatchRateZeroSuccess(t *testing.T) {
	r := ComputeHatchRate(HatchRateInput{
		EggID: "E", PetID: "P",
		EggsPurchasedDelta: ParseExactIntString("1000"),
		PetHatchDelta:      ParseExactIntString("0"),
		FutureEggs:         10000,
	}, nil)
	if r.ZeroSuccessNote == "" || r.ObservedRate == nil || *r.ObservedRate != 0 {
		t.Fatalf("%+v", r)
	}
	if r.AvgEggsPerSuccess != nil {
		t.Fatal("avg eggs/success should be nil for zero successes")
	}
}
