package loadout

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

const fantasyCap = "300000000000000000000"

func profileWithCurrency(currencyID, amount, eggsOpened string, purchased map[string]string) []byte {
	purch := map[string]any{}
	for k, v := range purchased {
		purch[k] = json.RawMessage(v)
	}
	top := map[string]any{
		"Statistics": map[string]any{
			"Eggs Opened": json.RawMessage(eggsOpened),
		},
		"PurchasedEggs": purch,
		"Currency": map[string]any{
			currencyID: map[string]any{"id": currencyID, "_am": json.RawMessage(amount)},
		},
	}
	b, _ := json.Marshal(map[string]any{"Data": top})
	// ExtractProfileView expects top-level Currency / Statistics — match fixture shape used elsewhere.
	b2, _ := json.Marshal(top)
	_ = b
	return b2
}

func bundleAt(t *testing.T, at time.Time, profile []byte) *snapshot.Bundle {
	t.Helper()
	s := at.UTC().Format(time.RFC3339Nano)
	return &snapshot.Bundle{
		Manifest: &snapshot.Manifest{
			Files: []snapshot.FileRecord{
				{Kind: "inventory-json", SnapshotAt: &s, RetrievedAt: s},
			},
		},
		ProfileData: profile,
	}
}

func TestFantasyCoinsCapSuppressesZeroRate(t *testing.T) {
	from := bundleAt(t, time.Date(2026, 11, 6, 16, 18, 0, 0, time.UTC), // 12:48 NST = 16:18 UTC
		profileWithCurrency("FantasyCoins", fantasyCap, "100", nil))
	to := bundleAt(t, time.Date(2026, 11, 6, 17, 29, 0, 0, time.UTC), // 13:59 NST
		profileWithCurrency("FantasyCoins", fantasyCap, "100", nil))

	rep, err := CalibrateFromSnapshots(CalibrationInput{
		From: from, To: to, CurrencyID: "FantasyCoins",
		Objective: Objective{ID: ObjCoinsPerMin, CurrencyID: "FantasyCoins"},
		WorldID:   "world-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.CurrencyCap == nil || !rep.CurrencyCap.Capped {
		t.Fatalf("expected capped: %+v", rep.CurrencyCap)
	}
	if rep.CurrencyCap.Finding != CapFindingCapped {
		t.Fatalf("finding=%q", rep.CurrencyCap.Finding)
	}
	if rep.ObservedPerMinute != nil {
		t.Fatalf("must not report zero rate as income: %v", *rep.ObservedPerMinute)
	}
	if rep.CurrencyDelta != "0" {
		t.Fatalf("delta=%s", rep.CurrencyDelta)
	}
	if rep.Confidence != ConfidenceUnknown {
		t.Fatalf("confidence=%s", rep.Confidence)
	}
}

func TestFantasyCoinsCapWithConcurrentSpendFlagsOffset(t *testing.T) {
	from := bundleAt(t, time.Date(2026, 11, 6, 16, 18, 0, 0, time.UTC),
		profileWithCurrency("FantasyCoins", fantasyCap, "100", map[string]string{"SomeEgg": "10"}))
	to := bundleAt(t, time.Date(2026, 11, 6, 17, 29, 0, 0, time.UTC),
		profileWithCurrency("FantasyCoins", fantasyCap, "150", map[string]string{"SomeEgg": "25"}))

	rep, err := CalibrateFromSnapshots(CalibrationInput{
		From: from, To: to, CurrencyID: "FantasyCoins",
		Objective: Objective{ID: ObjCoinsPerMin, CurrencyID: "FantasyCoins"},
		WorldID:   "world-4",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.CurrencyCap == nil || !rep.CurrencyCap.ProductionMayOffsetSpending {
		t.Fatalf("expected offset note: %+v", rep.CurrencyCap)
	}
	if rep.ObservedPerMinute != nil {
		t.Fatal("still must not invent a rate")
	}

	exp, err := CompareExperiment(from, to, ExperimentMeta{
		CurrencyID: "FantasyCoins", World: "world-4", ExperimentID: "fantasy-cap",
	}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	foundCap, foundZeroIncome := false, false
	for _, w := range exp.Warnings {
		if w == CapFindingCapped {
			foundCap = true
		}
		if w == "zero observed earnings for FantasyCoins (balance not at a known currency ceiling)" ||
			w == "zero observed earnings for FantasyCoins" {
			foundZeroIncome = true
		}
	}
	if !foundCap {
		t.Fatalf("missing CAPPED warning: %v", exp.Warnings)
	}
	if foundZeroIncome {
		t.Fatal("must not claim zero earnings when capped")
	}
}

func TestUncappedZeroDeltaStillReportsRate(t *testing.T) {
	from := bundleAt(t, time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC),
		profileWithCurrency("HalloweenGamesCoins", "1000", "1", nil))
	to := bundleAt(t, time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC),
		profileWithCurrency("HalloweenGamesCoins", "1000", "1", nil))
	rep, err := CalibrateFromSnapshots(CalibrationInput{
		From: from, To: to, CurrencyID: "HalloweenGamesCoins",
		Objective: Objective{ID: ObjHalloweenCoinsPerMin, CurrencyID: "HalloweenGamesCoins"},
		WorldID:   "halloween-games",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.CurrencyCap != nil && rep.CurrencyCap.Capped {
		t.Fatal("HalloweenGamesCoins has no known cap")
	}
	if rep.ObservedPerMinute == nil || *rep.ObservedPerMinute != 0 {
		t.Fatalf("uncapped flat balance may report observed 0: %+v", rep.ObservedPerMinute)
	}
}

func TestUserOverrideCap(t *testing.T) {
	from := bundleAt(t, time.Date(2026, 11, 6, 16, 0, 0, 0, time.UTC),
		profileWithCurrency("HalloweenGamesCoins", "999", "1", nil))
	to := bundleAt(t, time.Date(2026, 11, 6, 17, 0, 0, 0, time.UTC),
		profileWithCurrency("HalloweenGamesCoins", "999", "1", nil))
	rep, err := CalibrateFromSnapshots(CalibrationInput{
		From: from, To: to, CurrencyID: "HalloweenGamesCoins", CurrencyCap: "999",
		Objective: Objective{ID: ObjHalloweenCoinsPerMin, CurrencyID: "HalloweenGamesCoins"},
		WorldID:   "halloween-games",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep.CurrencyCap == nil || !rep.CurrencyCap.Capped || rep.ObservedPerMinute != nil {
		t.Fatalf("%+v", rep.CurrencyCap)
	}
}
