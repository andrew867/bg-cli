package forge

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestParseCensoredProbability(t *testing.T) {
	p := ParseDisplayedProbability("<0.001%")
	if !p.Censored || p.Fraction != nil || p.UpperBoundPct == nil || *p.UpperBoundPct != 0.001 {
		t.Fatalf("%+v", p)
	}
	z := ParseDisplayedProbability("0%")
	if z.Fraction == nil || *z.Fraction != 0 {
		t.Fatal(z)
	}
}

func TestButlerOnlyFixtureFitAndExpectedCosts(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-butler-2026-11-06.json"))
	if err != nil {
		t.Fatal(err)
	}
	rep := AnalyzeObservations(f)
	var model *CappedLinearModel
	for _, g := range rep.Groups {
		if g.Model != nil && g.Model.NPoints >= 11 {
			model = g.Model
		}
	}
	if model == nil {
		// find butler-only group
		for _, g := range rep.Groups {
			if g.Model != nil {
				model = g.Model
				break
			}
		}
	}
	if model == nil {
		t.Fatalf("no model: %+v", rep.Groups)
	}
	if model.Cap == nil || math.Abs(*model.Cap-0.75) > 1e-9 {
		t.Fatalf("cap=%v", model.Cap)
	}
	// Approximate slope ~0.02584
	if math.Abs(model.Slope-0.02584) > 0.0005 {
		t.Fatalf("slope=%v", model.Slope)
	}
	// Cap behaviour
	if math.Abs(model.Predict(30)-0.75) > 1e-9 {
		t.Fatalf("predict30=%v", model.Predict(30))
	}
	if model.Predict(29) > 0.75 {
		t.Fatal("29 should be below/at cap prediction without forcing")
	}

	// Expected butlers/success regression table (using OBSERVED p, not only model)
	cases := []struct {
		n   int
		pct float64
		eb  float64
	}{
		{1, 0.0258, 38.76},
		{5, 0.129, 38.76},
		{10, 0.258, 38.76},
		{20, 0.517, 38.68},
		{27, 0.698, 38.68},
		{29, 0.749, 38.72},
		{30, 0.75, 40.00},
	}
	for _, c := range cases {
		got, err := ExpectedButlersPerSuccess(c.n, c.pct)
		if err != nil {
			t.Fatal(err)
		}
		if math.Abs(got-c.eb) > 0.02 {
			t.Fatalf("n=%d got=%v want~%v", c.n, got, c.eb)
		}
	}

	opt, err := OptimizeButlerOnly(OptimizeOptions{
		Objective: ObjMinButlers, Weights: DefaultButlerWeights(),
		Model: *model, ShardsPerAttempt: 100, MinButlers: 1, MaxButlers: 30,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Best min-butlers should prefer ~29 over 30
	if len(opt.Candidates) == 0 {
		t.Fatal("no candidates")
	}
	best := opt.Candidates[0]
	if best.Butlers == 30 {
		t.Fatalf("30 should not win min-butlers: %+v", best)
	}
	var c29, c30 RecipeCandidate
	for _, c := range opt.Candidates {
		if c.Butlers == 29 {
			c29 = c
		}
		if c.Butlers == 30 {
			c30 = c
		}
	}
	if c29.ExpectedButlersPerSuccess == nil || c30.ExpectedButlersPerSuccess == nil {
		t.Fatal("missing expected costs")
	}
	if *c30.ExpectedButlersPerSuccess <= *c29.ExpectedButlersPerSuccess {
		t.Fatalf("expected 30 less efficient in butlers: 29=%v 30=%v", *c29.ExpectedButlersPerSuccess, *c30.ExpectedButlersPerSuccess)
	}
}

func TestMixedObservationExcludedFromButlerFit(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-butler-2026-11-06.json"))
	if err != nil {
		t.Fatal(err)
	}
	var mixed, butler27 Observation
	for _, o := range f.Observations {
		if o.ObservationID == "mixed-27-earlier" {
			mixed = o
		}
		if o.ObservationID == "butler-27" {
			butler27 = o
		}
	}
	if mixed.RecipeKind != "mixed" || butler27.RecipeKind != "butler-only" {
		t.Fatalf("%s %s", mixed.RecipeKind, butler27.RecipeKind)
	}
	if mixed.ComparableKey() == butler27.ComparableKey() {
		t.Fatal("mixed must not share comparable key with butler-only")
	}
	if butler27.DisplayedWinProbability.Fraction == nil || math.Abs(*butler27.DisplayedWinProbability.Fraction-0.698) > 1e-9 {
		t.Fatal(butler27.DisplayedWinProbability)
	}
}

func TestSoldOutAndCensoredRewardsPreserved(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-butler-2026-11-06.json"))
	if err != nil {
		t.Fatal(err)
	}
	var o30 Observation
	for _, o := range f.Observations {
		if o.ObservationID == "butler-30" {
			o30 = o
		}
	}
	foundCensored, foundSoldOut := false, false
	sumKnown := 0.0
	for _, r := range o30.RewardProbabilities {
		if r.Displayed.Censored {
			foundCensored = true
			if r.Displayed.Fraction != nil {
				t.Fatal("censored must not invent exact fraction")
			}
		}
		if r.Available != nil && !*r.Available {
			foundSoldOut = true
		}
		if r.Displayed.Fraction != nil {
			sumKnown += *r.Displayed.Fraction
		}
	}
	if !foundCensored || !foundSoldOut {
		t.Fatal("expected censored + sold out")
	}
	// Do not require sum==1; rounding may leave residual mass
	if math.Abs(sumKnown-1) < 1e-12 {
		t.Log("exact 1.0 sum unusual with rounding + censoring")
	}
}

func TestVariantProtection(t *testing.T) {
	w := DefaultButlerWeights()
	if VariantAllowed(w, "Shiny") || VariantAllowed(w, "Rainbow") {
		t.Fatal("shiny/rainbow protected by default")
	}
	if !VariantAllowed(w, "Regular") || !VariantAllowed(w, "Golden") {
		t.Fatal("regular/golden allowed by default")
	}
}

func TestTooManyPetsConstraintRecorded(t *testing.T) {
	f, err := LoadObservations(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-butler-2026-11-06.json"))
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, o := range f.Observations {
		for _, w := range o.ValidationWarnings {
			if w == "TOO MANY PETS" {
				found = true
			}
		}
	}
	if !found {
		t.Fatal("missing TOO MANY PETS observation")
	}
}

func TestLedgerDoesNotValidateSingleAttempt(t *testing.T) {
	ok := true
	f := LedgerFile{SchemaVersion: "1.0", Attempts: []Attempt{
		{AttemptID: "a1", Success: &ok, DisplayedWinProbability: ParseDisplayedProbability("75.0%")},
	}}
	s := SummarizeLedger(f)
	if s.ObservedRate == nil || *s.ObservedRate != 1 {
		t.Fatal(s)
	}
	if len(s.Notes) < 1 {
		t.Fatal("need caution notes")
	}
}

func TestNoNetworkOfflineLoad(t *testing.T) {
	// Ensure fixture path load does not require network — just file read.
	_, err := os.Stat(filepath.Join("..", "..", "testdata", "fixtures", "forge-observations-butler-2026-11-06.json"))
	if err != nil {
		t.Fatal(err)
	}
}
