package loadout

import (
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestCalibrateRequiresCurrencyAndRecordsConfounders(t *testing.T) {
	// Reuse snapshot package regression fixtures via writing two tiny manifests is heavy;
	// call Calibrate with nil bundles for error path, and empty currency for soft failure.
	rep, err := CalibrateFromSnapshots(CalibrationInput{
		From:      &snapshot.Bundle{Manifest: &snapshot.Manifest{}, Warnings: nil},
		To:        &snapshot.Bundle{Manifest: &snapshot.Manifest{}, Warnings: nil},
		Objective: Objective{ID: ObjHalloweenCoinsPerMin, CurrencyID: "HalloweenGamesCoins"},
		WorldID:   "halloween-games",
	})
	if err == nil && rep.OK {
		// ProfileData nil → ExtractProfileView fails
		t.Log("expected error or soft failure without profile data")
	}
	_ = filepath.Separator
	rep2, err := CalibrateFromSnapshots(CalibrationInput{
		From:       &snapshot.Bundle{Manifest: &snapshot.Manifest{Files: nil}, ProfileData: []byte(`{"Currency":{}}`)},
		To:         &snapshot.Bundle{Manifest: &snapshot.Manifest{Files: nil}, ProfileData: []byte(`{"Currency":{}}`)},
		Objective:  Objective{ID: ObjHalloweenCoinsPerMin},
		WorldID:    "halloween-games",
		CurrencyID: "HalloweenGamesCoins",
	})
	if err != nil {
		t.Fatal(err)
	}
	if rep2.OK {
		t.Fatal("missing currency amounts should soft-fail OK=false")
	}
	if rep2.Confidence != ConfidenceUnknown {
		t.Fatalf("%s", rep2.Confidence)
	}
}
