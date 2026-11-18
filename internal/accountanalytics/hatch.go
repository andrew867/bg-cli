package accountanalytics

import (
	"fmt"
	"math/big"
	"sort"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

const hatchMethodID = "accountanalytics.hatch-distribution.v1"

// HatchSpeciesRow is hatch delta and observed rate for one pet species in an egg interval.
type HatchSpeciesRow struct {
	PetID           string   `json:"petId"`
	HatchDelta      string   `json:"hatchDelta"`
	ObservedRate    *float64 `json:"observedRate,omitempty"`
	ObservedPercent *float64 `json:"observedPercent,omitempty"`
	WilsonLow95     *float64 `json:"wilsonLow95,omitempty"`
	WilsonHigh95    *float64 `json:"wilsonHigh95,omitempty"`
	MethodID        string   `json:"methodId"`
}

// HatchReportResult is the full hatch distribution between two profile snapshots for one egg.
type HatchReportResult struct {
	MethodID         string            `json:"methodId"`
	EggID            string            `json:"eggId"`
	EggPurchaseDelta string            `json:"eggPurchaseDelta"`
	SpeciesDeltaSum  string            `json:"speciesDeltaSum"`
	FullyReconciled  bool              `json:"fullyReconciled"`
	Residual         string            `json:"residual,omitempty"`
	Species          []HatchSpeciesRow `json:"species"`
	Warnings         []string          `json:"warnings,omitempty"`
}

// HatchReport compares two bundles' profile counters for egg purchases and pet hatch distribution.
func HatchReport(from, to *snapshot.Bundle, eggID string) (HatchReportResult, error) {
	if from == nil || to == nil {
		return HatchReportResult{}, fmt.Errorf("nil bundle")
	}
	if from.ProfileData == nil || to.ProfileData == nil {
		return HatchReportResult{}, fmt.Errorf("profile data missing on one or both bundles")
	}
	fromPV, err := snapshot.ExtractProfileView(from.ProfileData)
	if err != nil {
		return HatchReportResult{}, fmt.Errorf("from profile: %w", err)
	}
	toPV, err := snapshot.ExtractProfileView(to.ProfileData)
	if err != nil {
		return HatchReportResult{}, fmt.Errorf("to profile: %w", err)
	}
	if eggID == "" {
		return HatchReportResult{}, fmt.Errorf("eggID required")
	}
	purchDeltas := snapshot.DiffExactMaps(toPV.PurchasedEggs, fromPV.PurchasedEggs)
	hatchDeltas := snapshot.DiffExactMaps(toPV.PetHatchCount, fromPV.PetHatchCount)
	eggDelta := snapshot.LookupCI(purchDeltas, eggID)
	if !eggDelta.ParseOK {
		return HatchReportResult{}, fmt.Errorf("egg %q has no purchasable delta", eggID)
	}

	speciesSum := snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}
	var speciesRows []HatchSpeciesRow
	var petIDs []string
	for id, d := range hatchDeltas {
		if !d.ParseOK || d.Int == nil || d.Int.Sign() == 0 {
			continue
		}
		petIDs = append(petIDs, id)
	}
	sort.Strings(petIDs)
	for _, petID := range petIDs {
		d := hatchDeltas[petID]
		if d.Int.Sign() > 0 {
			speciesSum = snapshot.AddExact(speciesSum, d)
		}
		hr := snapshot.ComputeHatchRate(snapshot.HatchRateInput{
			EggID:              eggID,
			PetID:              petID,
			EggsPurchasedDelta: eggDelta,
			PetHatchDelta:      d,
		}, nil)
		row := HatchSpeciesRow{
			PetID:      petID,
			HatchDelta: d.Text,
			MethodID:   hr.MethodID,
		}
		if hr.ObservedRate != nil {
			row.ObservedRate = hr.ObservedRate
			row.ObservedPercent = hr.ObservedPercent
			row.WilsonLow95 = hr.WilsonLow95
			row.WilsonHigh95 = hr.WilsonHigh95
		}
		speciesRows = append(speciesRows, row)
	}

	reconciled := false
	residual := ""
	if eggDelta.Int != nil && speciesSum.Int != nil {
		res := new(big.Int).Sub(speciesSum.Int, eggDelta.Int)
		residual = res.String()
		reconciled = res.Sign() == 0
	}

	report := HatchReportResult{
		MethodID:         hatchMethodID,
		EggID:            eggID,
		EggPurchaseDelta: eggDelta.Text,
		SpeciesDeltaSum:  speciesSum.Text,
		FullyReconciled:  reconciled,
		Residual:         residual,
		Species:          speciesRows,
	}
	if !reconciled {
		report.Warnings = append(report.Warnings,
			fmt.Sprintf("species hatch delta sum %s != egg purchase delta %s", speciesSum.Text, eggDelta.Text))
	}
	return report, nil
}
