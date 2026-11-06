package loadout

import (
	"fmt"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

// CalibrationInput is a measured interval between two offline snapshots.
type CalibrationInput struct {
	From        *snapshot.Bundle
	To          *snapshot.Bundle
	CurrencyID  string
	Objective   Objective
	WorldID     string
	Note        string
	CurrencyCap string // optional exact override of known ceiling for this currency
}

// CalibrationReport records measured rates without inventing multipliers.
type CalibrationReport struct {
	OK                      bool           `json:"ok"`
	CurrencyID              string         `json:"currencyId,omitempty"`
	Objective               ObjectiveID    `json:"objective"`
	WorldID                 string         `json:"worldId"`
	ElapsedSeconds          *float64       `json:"elapsedSeconds,omitempty"`
	ElapsedBasis            string         `json:"elapsedBasis,omitempty"`
	CurrencyDelta           string         `json:"currencyDelta,omitempty"`
	StartingBalance         string         `json:"startingBalance,omitempty"`
	EndingBalance           string         `json:"endingBalance,omitempty"`
	ObservedPerMinute       *float64       `json:"observedPerMinute,omitempty"`
	CurrencyCap             *CapAssessment `json:"currencyCap,omitempty"`
	SampleSizeNote          string         `json:"sampleSizeNote"`
	ChangedVariables        []string       `json:"changedVariables"`
	DataFreshness           []string       `json:"dataFreshness"`
	Confidence              Confidence     `json:"confidence"`
	PossibleConfounders     []string       `json:"possibleConfounders"`
	Warnings                []string       `json:"warnings"`
	SuggestedRegistryUpdate string         `json:"suggestedRegistryUpdate,omitempty"`
	MethodID                string         `json:"methodId"`
}

// CalibrateFromSnapshots measures an observed currency rate from two bundles.
// It does not write new registry magnitudes automatically.
func CalibrateFromSnapshots(in CalibrationInput) (CalibrationReport, error) {
	rep := CalibrationReport{
		OK:             true,
		CurrencyID:     in.CurrencyID,
		Objective:      in.Objective.ID,
		WorldID:        in.WorldID,
		Confidence:     ConfidenceMeasured,
		SampleSizeNote: "single interval; treat as one sample, not a population estimate",
		PossibleConfounders: []string{
			"loadout changes between snapshots",
			"active potions / enchants not captured in inventory equipped list",
			"server events, luck modifiers, co-op bonuses",
			"different breakable maps or AFK vs active play",
			"inventory snapshotAt may be a proxy if profile counters differ in time",
		},
		MethodID: "loadout-calibrate.v1",
		Warnings: []string{},
	}
	if in.Note != "" {
		rep.ChangedVariables = append(rep.ChangedVariables, "userNote: "+in.Note)
	}
	if in.From == nil || in.To == nil {
		return rep, fmt.Errorf("both snapshot bundles are required")
	}
	if in.CurrencyID == "" && in.Objective.CurrencyID != "" {
		in.CurrencyID = in.Objective.CurrencyID
		rep.CurrencyID = in.CurrencyID
	}
	if in.CurrencyID == "" {
		rep.OK = false
		rep.Warnings = append(rep.Warnings, "currency id required for currency-rate calibration")
		rep.Confidence = ConfidenceUnknown
		return rep, nil
	}

	fromPV, err := snapshot.ExtractProfileView(in.From.ProfileData)
	if err != nil {
		return rep, err
	}
	toPV, err := snapshot.ExtractProfileView(in.To.ProfileData)
	if err != nil {
		return rep, err
	}
	start, startOK := currencyPresent(fromPV.Currencies, in.CurrencyID)
	end, endOK := currencyPresent(toPV.Currencies, in.CurrencyID)
	if !startOK || !endOK || !start.ParseOK || !end.ParseOK {
		rep.OK = false
		rep.Confidence = ConfidenceUnknown
		rep.Warnings = append(rep.Warnings, "currency amounts missing or unparsable in one or both snapshots")
		return rep, nil
	}
	delta, err := snapshot.SubExact(end, start)
	if err != nil {
		return rep, err
	}
	rep.CurrencyDelta = delta.Text
	rep.StartingBalance = start.Text
	rep.EndingBalance = end.Text

	spendConfirmed := ConcurrentSpendSignals(in.From, in.To)
	cap := AssessCurrencyCap(in.CurrencyID, start, end, delta, in.CurrencyCap, spendConfirmed)
	if cap.Capped || cap.Finding != "" {
		rep.CurrencyCap = &cap
	}
	if cap.Capped {
		rep.Confidence = ConfidenceUnknown
		rep.Warnings = append(rep.Warnings, cap.Finding)
		rep.PossibleConfounders = append(rep.PossibleConfounders,
			"currency balance may be at a hard ceiling; excess production is discarded and Δbalance/Δt is not gross income",
		)
		if cap.ProductionMayOffsetSpending {
			rep.Warnings = append(rep.Warnings, cap.ProductionOffsetNote)
		}
	}

	var fromAt, toAt *string
	for _, f := range in.From.Manifest.Files {
		if f.Kind == "inventory-json" {
			fromAt = f.SnapshotAt
			if f.RetrievedAt != "" {
				rep.DataFreshness = append(rep.DataFreshness, "from.retrievedAt="+f.RetrievedAt)
			}
		}
	}
	for _, f := range in.To.Manifest.Files {
		if f.Kind == "inventory-json" {
			toAt = f.SnapshotAt
			if f.RetrievedAt != "" {
				rep.DataFreshness = append(rep.DataFreshness, "to.retrievedAt="+f.RetrievedAt)
			}
		}
	}
	if fromAt != nil && toAt != nil {
		t0, e0 := time.Parse(time.RFC3339Nano, *fromAt)
		if e0 != nil {
			t0, e0 = time.Parse(time.RFC3339, *fromAt)
		}
		t1, e1 := time.Parse(time.RFC3339Nano, *toAt)
		if e1 != nil {
			t1, e1 = time.Parse(time.RFC3339, *toAt)
		}
		if e0 == nil && e1 == nil {
			sec := t1.Sub(t0).Seconds()
			if sec > 0 {
				rep.ElapsedSeconds = &sec
				rep.ElapsedBasis = "inventory snapshotAt proxy"
				if !cap.Capped {
					// per-minute rate as float for reporting only — never emit 0 from a capped flat balance
					df, _ := delta.Int.Float64()
					rate := df / (sec / 60.0)
					rep.ObservedPerMinute = &rate
				}
			} else {
				rep.Warnings = append(rep.Warnings, "non-positive elapsed interval; per-minute rate omitted")
				rep.Confidence = ConfidenceUnknown
			}
		}
	} else {
		rep.Warnings = append(rep.Warnings, "missing inventory snapshotAt; elapsed time and per-minute rate omitted")
		rep.Confidence = ConfidenceUnknown
	}

	// Detect loadout-related inventory pet quantity changes as possible confounders.
	if in.From.Inventory != nil && in.To.Inventory != nil {
		fromL, err1 := ParseInventoryLoadout(in.From.Inventory)
		toL, err2 := ParseInventoryLoadout(in.To.Inventory)
		if err1 == nil && err2 == nil {
			if !sameIDs(fromL.Enchants.List, toL.Enchants.List) {
				rep.ChangedVariables = append(rep.ChangedVariables, "equipped enchants differ between snapshots")
			}
			if !sameIDs(fromL.Pets.List, toL.Pets.List) {
				rep.ChangedVariables = append(rep.ChangedVariables, "equipped pets differ between snapshots")
			}
		}
	}
	if len(rep.ChangedVariables) == 0 {
		rep.ChangedVariables = append(rep.ChangedVariables, "no equipped pet/enchant ID set changes detected in inventory payloads")
	}
	rep.SuggestedRegistryUpdate = fmt.Sprintf(
		"If this interval was a controlled test for %s in world %s, manually add a measured effect magnitude to the registry with confidence=measured and cite these manifests; do not auto-promote.",
		in.CurrencyID, in.WorldID,
	)
	return rep, nil
}

func currencyPresent(m map[string]snapshot.ExactInt, id string) (snapshot.ExactInt, bool) {
	if v, ok := m[id]; ok {
		return v, true
	}
	for k, v := range m {
		if strings.EqualFold(k, id) {
			return v, true
		}
	}
	return snapshot.ExactInt{}, false
}

func sameIDs(a, b []EquippedSlot) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].ID != b[i].ID {
			return false
		}
	}
	return true
}
