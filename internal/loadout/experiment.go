package loadout

import (
	"fmt"
	"strings"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

// ExperimentMeta describes a controlled farming / loadout experiment.
type ExperimentMeta struct {
	ExperimentID     string   `json:"experimentId"`
	World            string   `json:"world"`
	CurrencyID       string   `json:"currencyId"`
	PetLoadout       []string `json:"petLoadout,omitempty"`
	EnchantLoadout   []string `json:"enchantLoadout,omitempty"`
	PotionsBuffs     []string `json:"potionsAndBuffsKnownActive,omitempty"`
	Activity         string   `json:"activity,omitempty"`
	KnownConfounders []string `json:"knownConfounders,omitempty"`
	Design           string   `json:"design,omitempty"` // A/B or A/B/A
}

// ExperimentCompareReport extends snapshot calibration with experiment checks.
type ExperimentCompareReport struct {
	OK           bool              `json:"ok"`
	Meta         ExperimentMeta    `json:"meta"`
	Calibration  CalibrationReport `json:"calibration"`
	Checks       []string          `json:"checks"`
	FailedChecks []string          `json:"failedChecks"`
	Warnings     []string          `json:"warnings"`
	MethodID     string            `json:"methodId"`
}

// CompareExperiment runs offline snapshot comparison with experiment hygiene checks.
func CompareExperiment(from, to *snapshot.Bundle, meta ExperimentMeta, fromLoadout, toLoadout *PlayerLoadout) (ExperimentCompareReport, error) {
	rep := ExperimentCompareReport{
		OK:       true,
		Meta:     meta,
		MethodID: "account-experiment-compare.v1",
		Checks:   []string{},
		Warnings: []string{},
	}
	if meta.CurrencyID == "" {
		return rep, fmt.Errorf("experiment currencyId is required")
	}
	cal, err := CalibrateFromSnapshots(CalibrationInput{
		From: from, To: to, CurrencyID: meta.CurrencyID,
		Objective: Objective{ID: ObjectiveID("currency-rate"), CurrencyID: meta.CurrencyID},
		WorldID:   meta.World, Note: meta.ExperimentID,
	})
	if err != nil {
		return rep, err
	}
	rep.Calibration = cal
	rep.Warnings = append(rep.Warnings, cal.Warnings...)

	pass := func(s string) { rep.Checks = append(rep.Checks, "PASS: "+s) }
	fail := func(s string) {
		rep.FailedChecks = append(rep.FailedChecks, s)
		rep.OK = false
	}

	if cal.ElapsedSeconds == nil || *cal.ElapsedSeconds <= 0 {
		fail("duration missing or non-positive")
	} else {
		pass("positive elapsed duration from inventory snapshotAt")
	}
	if cal.ObservedPerMinute != nil && *cal.ObservedPerMinute == 0 {
		rep.Warnings = append(rep.Warnings, "zero observed earnings for "+meta.CurrencyID)
	}
	if strings.HasPrefix(cal.CurrencyDelta, "-") {
		rep.Warnings = append(rep.Warnings, "negative currency delta — may include expenditures; not labeled pure farming income")
	}

	// Concurrent hatch / loadout change detection via profile + inventory when available.
	if from != nil && to != nil && from.ProfileData != nil && to.ProfileData != nil {
		fp, err1 := snapshot.ExtractProfileView(from.ProfileData)
		tp, err2 := snapshot.ExtractProfileView(to.ProfileData)
		if err1 == nil && err2 == nil {
			eggOpen := snapshot.LookupCI(snapshot.DiffExactMaps(tp.Statistics, fp.Statistics), "Eggs Opened")
			if eggOpen.ParseOK && eggOpen.Int != nil && eggOpen.Int.Sign() != 0 {
				fail("Eggs Opened changed during interval — concurrent hatching confounder")
			} else {
				pass("Eggs Opened unchanged")
			}
		}
	}
	if fromLoadout != nil && toLoadout != nil {
		if !sameIDs(fromLoadout.Enchants.List, toLoadout.Enchants.List) {
			fail("enchant loadout changed between snapshots")
		} else {
			pass("enchant loadout IDs unchanged")
		}
		if !sameIDs(fromLoadout.Pets.List, toLoadout.Pets.List) {
			fail("pet loadout changed between snapshots")
		} else {
			pass("pet loadout IDs unchanged")
		}
	} else {
		rep.Warnings = append(rep.Warnings, "loadout payloads not both available; pet/enchant stability not fully checked")
	}

	rep.Warnings = append(rep.Warnings,
		"single interval is one sample — do not present as a reliable future earning rate",
		"net currency change is not pure farming income without accounting for other flows",
		"Coins enchants must not be assumed to affect "+meta.CurrencyID+" unless sourced or measured",
	)
	for _, c := range meta.KnownConfounders {
		rep.Warnings = append(rep.Warnings, "declared confounder: "+c)
	}
	return rep, nil
}
