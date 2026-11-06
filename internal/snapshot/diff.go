package snapshot

import (
	"fmt"
	"sort"
	"strings"
)

// DiffOptions configures offline snapshot comparison.
type DiffOptions struct {
	EggID      string
	PetID      string
	EggPrice   string
	CurrencyID string
	FutureEggs int64
	BatchSize  int64
}

// CounterDelta is a named before/after/delta triple.
type CounterDelta struct {
	ID       string `json:"id"`
	Previous string `json:"previous"`
	Current  string `json:"current"`
	Delta    string `json:"delta"`
	Negative bool   `json:"negative"`
	Approx   bool   `json:"approximate,omitempty"`
}

// DiffResult is the full offline comparison report.
type DiffResult struct {
	OK                    bool                 `json:"ok"`
	FromManifest          string               `json:"fromManifest"`
	ToManifest            string               `json:"toManifest"`
	Warnings              []string             `json:"warnings"`
	Assumptions           []string             `json:"assumptions"`
	FromExportedAt        string               `json:"fromExportedAt,omitempty"`
	ToExportedAt          string               `json:"toExportedAt,omitempty"`
	InventorySnapshotFrom *string              `json:"inventorySnapshotAtFrom,omitempty"`
	InventorySnapshotTo   *string              `json:"inventorySnapshotAtTo,omitempty"`
	Statistics            []CounterDelta       `json:"statistics"`
	PurchasedEggs         []CounterDelta       `json:"purchasedEggs"`
	PetHatchCounts        []CounterDelta       `json:"petHatchCounts"`
	InventoryByPet        []CounterDelta       `json:"inventoryByPet"`
	InventoryByVariant    []map[string]any     `json:"inventoryByVariant"`
	Currencies            []CounterDelta       `json:"currencies"`
	Reconciliation        ReconciliationReport `json:"reconciliation"`
	HatchRate             *HatchRateResult     `json:"hatchRate,omitempty"`
	Cost                  *CostResult          `json:"cost,omitempty"`
	MethodID              string               `json:"methodId"`
}

// DiffBundles compares two loaded offline snapshots.
func DiffBundles(from, to *Bundle, opt DiffOptions) (DiffResult, error) {
	res := DiffResult{
		OK:           true,
		FromManifest: from.Manifest.ManifestFile,
		ToManifest:   to.Manifest.ManifestFile,
		Warnings:     append([]string{}, from.Warnings...),
		Assumptions: []string{
			"comparison is offline from saved snapshot files; no API calls were made",
			"negative deltas are flagged but not automatic errors (upstream reset/correction possible)",
			"owned inventory changes are not equivalent to lifetime hatch deltas",
		},
		MethodID: "snapshot-diff.v1",
	}
	res.Warnings = append(res.Warnings, to.Warnings...)
	res.FromExportedAt = from.Manifest.ExportedAt
	res.ToExportedAt = to.Manifest.ExportedAt

	for _, f := range from.Manifest.Files {
		if f.Kind == "inventory-json" && f.SnapshotAt != nil {
			res.InventorySnapshotFrom = f.SnapshotAt
		}
	}
	for _, f := range to.Manifest.Files {
		if f.Kind == "inventory-json" && f.SnapshotAt != nil {
			res.InventorySnapshotTo = f.SnapshotAt
		}
	}

	if from.ProfileData == nil || to.ProfileData == nil {
		res.OK = false
		res.Warnings = append(res.Warnings, "profile data missing on one or both snapshots")
	}

	var fromPV, toPV ProfileView
	var err error
	if from.ProfileData != nil {
		fromPV, err = ExtractProfileView(from.ProfileData)
		if err != nil {
			return res, fmt.Errorf("from profile: %w", err)
		}
	}
	if to.ProfileData != nil {
		toPV, err = ExtractProfileView(to.ProfileData)
		if err != nil {
			return res, fmt.Errorf("to profile: %w", err)
		}
	}

	res.Statistics = diffNamedMaps(fromPV.Statistics, toPV.Statistics, "")
	res.PurchasedEggs = diffNamedMaps(fromPV.PurchasedEggs, toPV.PurchasedEggs, "")
	res.PetHatchCounts = diffNamedMaps(fromPV.PetHatchCount, toPV.PetHatchCount, opt.PetID)
	res.Currencies = diffNamedMaps(fromPV.Currencies, toPV.Currencies, "")

	var fromPets, toPets []PetStack
	if from.Inventory != nil {
		fromPets, err = ExtractPetStacks(from.Inventory)
		if err != nil {
			res.Warnings = append(res.Warnings, "from inventory: "+err.Error())
		}
	}
	if to.Inventory != nil {
		toPets, err = ExtractPetStacks(to.Inventory)
		if err != nil {
			res.Warnings = append(res.Warnings, "to inventory: "+err.Error())
		}
	}
	fromByID, fromByVar := AggregatePets(fromPets, opt.PetID)
	toByID, toByVar := AggregatePets(toPets, opt.PetID)
	res.InventoryByPet = diffNamedMaps(fromByID, toByID, "")
	res.InventoryByVariant = variantDiffRows(fromByVar, toByVar)

	statDeltas := DiffExactMaps(toPV.Statistics, fromPV.Statistics)
	eggOpen := LookupCI(statDeltas, "Eggs Opened")
	purchDeltas := DiffExactMaps(toPV.PurchasedEggs, fromPV.PurchasedEggs)
	hatchDeltas := DiffExactMaps(toPV.PetHatchCount, fromPV.PetHatchCount)
	purchPos := SumPositiveDeltas(purchDeltas)
	hatchPos := SumPositiveDeltas(hatchDeltas)
	var selectedEgg, selectedPet ExactInt
	if opt.EggID != "" {
		selectedEgg = LookupCI(purchDeltas, opt.EggID)
	}
	if opt.PetID != "" {
		selectedPet = LookupCI(hatchDeltas, opt.PetID)
	}
	invNet := LookupCI(DiffExactMaps(toByID, fromByID), opt.PetID)
	eggsHatchedDelta, _ := SubExact(zeroIfMissing(toPV.EggsHatched), zeroIfMissing(fromPV.EggsHatched))

	res.Reconciliation = ReconcileHatchAccounting(
		eggOpen, eggsHatchedDelta, purchPos, selectedEgg, hatchPos, selectedPet, invNet,
		fromPV.HasStatistics && toPV.HasStatistics,
		fromPV.HasPurchased && toPV.HasPurchased,
		fromPV.HasPetHatch && toPV.HasPetHatch,
	)

	if opt.EggID != "" || opt.PetID != "" {
		otherEggs := []string{}
		for _, k := range ChangedKeys(purchDeltas) {
			if opt.EggID == "" || !strings.EqualFold(k, opt.EggID) {
				otherEggs = append(otherEggs, k)
			}
		}
		sort.Strings(otherEggs)
		hrIn := HatchRateInput{
			EggID:              opt.EggID,
			PetID:              opt.PetID,
			EggsPurchasedDelta: selectedEgg,
			PetHatchDelta:      selectedPet,
			FutureEggs:         opt.FutureEggs,
		}
		if opt.EggID == "" {
			hrIn.EggsPurchasedDelta = eggOpen
			res.Assumptions = append(res.Assumptions, "no --egg specified; using Eggs Opened delta as trial count for hatch-rate analysis")
		}
		hr := ComputeHatchRate(hrIn, otherEggs)
		res.HatchRate = &hr
	}

	if opt.EggPrice != "" || opt.CurrencyID != "" || opt.FutureEggs > 0 || opt.BatchSize > 0 {
		cin := CostInput{
			CurrencyID:   opt.CurrencyID,
			FutureEggs:   opt.FutureEggs,
			BatchSize:    opt.BatchSize,
			HasCurrency:  opt.CurrencyID != "",
			HasFuture:    opt.FutureEggs > 0,
			HasBatchSize: opt.BatchSize > 0,
		}
		if opt.EggPrice != "" {
			cin.EggPrice = ParseExactIntString(opt.EggPrice)
			cin.HasEggPrice = cin.EggPrice.ParseOK
		}
		eggsForCost := selectedEgg
		if !eggsForCost.ParseOK || eggsForCost.Int == nil || (eggsForCost.Int.Sign() == 0 && eggOpen.ParseOK) {
			if opt.EggID == "" {
				eggsForCost = eggOpen
			}
		}
		var rate *float64
		if res.HatchRate != nil {
			rate = res.HatchRate.ObservedRate
		}
		startBal := LookupCI(fromPV.Currencies, opt.CurrencyID)
		endBal := LookupCI(toPV.Currencies, opt.CurrencyID)
		cost := ComputeCostTiming(cin, eggsForCost, startBal, endBal, rate, res.InventorySnapshotFrom, res.InventorySnapshotTo)
		res.Cost = &cost
	}

	if len(FlagNegative(statDeltas)) > 0 {
		res.Warnings = append(res.Warnings, "one or more Statistics counters decreased")
	}
	return res, nil
}

func diffNamedMaps(oldM, newM map[string]ExactInt, filter string) []CounterDelta {
	deltas := DiffExactMaps(newM, oldM)
	keys := make([]string, 0, len(deltas))
	for k := range deltas {
		if filter != "" && !strings.EqualFold(k, filter) {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	out := make([]CounterDelta, 0, len(keys))
	for _, k := range keys {
		o := oldM[k]
		n := newM[k]
		if !o.ParseOK {
			o = ExactInt{Text: "0", ParseOK: true, SourceText: "0"}
		}
		if !n.ParseOK {
			n = ExactInt{Text: "0", ParseOK: true, SourceText: "0"}
		}
		d := deltas[k]
		out = append(out, CounterDelta{
			ID:       k,
			Previous: o.String(),
			Current:  n.String(),
			Delta:    d.String(),
			Negative: d.ParseOK && d.Int != nil && d.Int.Sign() < 0,
			Approx:   o.Approximate || n.Approximate || d.Approximate,
		})
	}
	return out
}

func variantDiffRows(fromByVar, toByVar map[string]map[string]ExactInt) []map[string]any {
	pets := map[string]struct{}{}
	for k := range fromByVar {
		pets[k] = struct{}{}
	}
	for k := range toByVar {
		pets[k] = struct{}{}
	}
	ids := make([]string, 0, len(pets))
	for k := range pets {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	var rows []map[string]any
	for _, id := range ids {
		vars := map[string]struct{}{}
		for v := range fromByVar[id] {
			vars[v] = struct{}{}
		}
		for v := range toByVar[id] {
			vars[v] = struct{}{}
		}
		vnames := make([]string, 0, len(vars))
		for v := range vars {
			vnames = append(vnames, v)
		}
		sort.Strings(vnames)
		for _, v := range vnames {
			o := fromByVar[id][v]
			n := toByVar[id][v]
			if !o.ParseOK {
				o = ExactInt{Text: "0", ParseOK: true, SourceText: "0"}
			}
			if !n.ParseOK {
				n = ExactInt{Text: "0", ParseOK: true, SourceText: "0"}
			}
			d, _ := SubExact(n, o)
			rows = append(rows, map[string]any{
				"id":       id,
				"variant":  v,
				"previous": o.String(),
				"current":  n.String(),
				"delta":    d.String(),
				"negative": d.ParseOK && d.Int != nil && d.Int.Sign() < 0,
			})
		}
	}
	return rows
}
