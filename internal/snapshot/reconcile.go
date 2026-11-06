package snapshot

import (
	"math/big"
	"strings"
)

// ReconciliationStatus describes hatch accounting agreement.
type ReconciliationStatus string

const (
	Reconciled       ReconciliationStatus = "RECONCILED"
	Partial          ReconciliationStatus = "PARTIAL"
	Mismatch         ReconciliationStatus = "MISMATCH"
	InsufficientData ReconciliationStatus = "INSUFFICIENT_DATA"
)

// ReconciliationReport cross-checks egg and hatch counters.
type ReconciliationReport struct {
	Status                   ReconciliationStatus `json:"status"`
	EggsOpenedDelta          *string              `json:"eggsOpenedDelta,omitempty"`
	EggsHatchedDelta         *string              `json:"eggsHatchedDelta,omitempty"`
	PurchasedEggsDeltaSum    *string              `json:"purchasedEggsPositiveDeltaSum,omitempty"`
	SelectedEggPurchaseDelta *string              `json:"selectedEggPurchaseDelta,omitempty"`
	PetHatchPositiveDeltaSum *string              `json:"petHatchPositiveDeltaSum,omitempty"`
	SelectedPetHatchDelta    *string              `json:"selectedPetHatchDelta,omitempty"`
	InventoryNetSelectedPet  *string              `json:"inventoryNetSelectedPet,omitempty"`
	ResidualEggsVsHatches    *string              `json:"residualEggsOpenedMinusPetHatches,omitempty"`
	ResidualSelectedEggVsPet *string              `json:"residualSelectedEggPurchasesMinusSelectedPetHatches,omitempty"`
	Notes                    []string             `json:"notes"`
	Meaning                  string               `json:"meaning"`
}

// ReconcileHatchAccounting compares interval counters.
func ReconcileHatchAccounting(
	eggsOpenedDelta ExactInt,
	eggsHatchedDelta ExactInt,
	purchasedPositiveSum ExactInt,
	selectedEggDelta ExactInt,
	petHatchPositiveSum ExactInt,
	selectedPetHatch ExactInt,
	inventoryNetSelected ExactInt,
	hasEggsOpened, hasPurchased, hasPetHatch bool,
) ReconciliationReport {
	rep := ReconciliationReport{
		Notes:   []string{},
		Meaning: "RECONCILED means selected counters numerically agree under stated rules; it does not prove absence of other game activity",
	}
	ptr := func(e ExactInt) *string {
		if !e.ParseOK {
			return nil
		}
		s := e.Text
		return &s
	}
	if hasEggsOpened && eggsOpenedDelta.ParseOK {
		rep.EggsOpenedDelta = ptr(eggsOpenedDelta)
	}
	if eggsHatchedDelta.ParseOK {
		rep.EggsHatchedDelta = ptr(eggsHatchedDelta)
		rep.Notes = append(rep.Notes, "EggsHatched and Statistics[\"Eggs Opened\"] may differ by a persistent offset; both are preserved without automatic normalization")
	}
	if hasPurchased && purchasedPositiveSum.ParseOK {
		rep.PurchasedEggsDeltaSum = ptr(purchasedPositiveSum)
	}
	if selectedEggDelta.ParseOK {
		rep.SelectedEggPurchaseDelta = ptr(selectedEggDelta)
	}
	if hasPetHatch && petHatchPositiveSum.ParseOK {
		rep.PetHatchPositiveDeltaSum = ptr(petHatchPositiveSum)
	}
	if selectedPetHatch.ParseOK {
		rep.SelectedPetHatchDelta = ptr(selectedPetHatch)
	}
	if inventoryNetSelected.ParseOK {
		rep.InventoryNetSelectedPet = ptr(inventoryNetSelected)
		rep.Notes = append(rep.Notes, "inventory net change is not identical to newly hatched pets (trading/forging/consuming can differ)")
	}

	if !hasEggsOpened || !hasPetHatch || !eggsOpenedDelta.ParseOK || !petHatchPositiveSum.ParseOK {
		rep.Status = InsufficientData
		rep.Notes = append(rep.Notes, "missing Eggs Opened and/or PetHatchCount data")
		return rep
	}

	resid := new(big.Int).Sub(eggsOpenedDelta.Int, petHatchPositiveSum.Int)
	rs := resid.String()
	rep.ResidualEggsVsHatches = &rs

	if selectedEggDelta.ParseOK && selectedPetHatch.ParseOK {
		r2 := new(big.Int).Sub(selectedEggDelta.Int, selectedPetHatch.Int)
		s2 := r2.String()
		rep.ResidualSelectedEggVsPet = &s2
	}

	// Primary reconcile: Eggs Opened delta == sum of positive PetHatchCount deltas
	if eggsOpenedDelta.Int.Cmp(petHatchPositiveSum.Int) == 0 {
		rep.Status = Reconciled
		if selectedEggDelta.ParseOK && selectedEggDelta.Int.Cmp(eggsOpenedDelta.Int) != 0 {
			rep.Status = Partial
			rep.Notes = append(rep.Notes, "Eggs Opened delta matches total pet hatch deltas, but selected egg purchase delta differs; other egg types also changed")
		}
		return rep
	}

	if hasPurchased && purchasedPositiveSum.ParseOK && purchasedPositiveSum.Int.Cmp(petHatchPositiveSum.Int) == 0 {
		rep.Status = Partial
		rep.Notes = append(rep.Notes, "sum of positive PurchasedEggs deltas matches pet hatch deltas, but Eggs Opened differs")
		return rep
	}

	rep.Status = Mismatch
	rep.Notes = append(rep.Notes, "Eggs Opened delta does not equal sum of positive PetHatchCount deltas")
	return rep
}

// SumPositiveDeltas sums positive values from a map of deltas.
func SumPositiveDeltas(deltas map[string]ExactInt) ExactInt {
	sum := ExactInt{Int: new(big.Int), Text: "0", ParseOK: true, SourceText: "0"}
	for _, d := range deltas {
		if d.ParseOK && d.Int != nil && d.Int.Sign() > 0 {
			sum = AddExact(sum, d)
		}
	}
	return sum
}

// DiffExactMaps computes new-old for each key union.
func DiffExactMaps(neu, old map[string]ExactInt) map[string]ExactInt {
	keys := map[string]struct{}{}
	for k := range neu {
		keys[k] = struct{}{}
	}
	for k := range old {
		keys[k] = struct{}{}
	}
	out := map[string]ExactInt{}
	for k := range keys {
		n := neu[k]
		o := old[k]
		if !n.ParseOK {
			n = ExactInt{Int: new(big.Int), Text: "0", ParseOK: true, SourceText: "0"}
		}
		if !o.ParseOK {
			o = ExactInt{Int: new(big.Int), Text: "0", ParseOK: true, SourceText: "0"}
		}
		d, err := SubExact(n, o)
		if err != nil {
			continue
		}
		out[k] = d
	}
	return out
}

func FlagNegative(deltas map[string]ExactInt) []string {
	var keys []string
	for k, d := range deltas {
		if d.ParseOK && d.Int != nil && d.Int.Sign() < 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

func ChangedKeys(deltas map[string]ExactInt) []string {
	var keys []string
	for k, d := range deltas {
		if d.ParseOK && d.Int != nil && d.Int.Sign() != 0 {
			keys = append(keys, k)
		}
	}
	return keys
}

func LookupCI(m map[string]ExactInt, id string) ExactInt {
	if v, ok := m[id]; ok {
		return v
	}
	for k, v := range m {
		if strings.EqualFold(k, id) {
			return v
		}
	}
	return ExactInt{Int: new(big.Int), Text: "0", ParseOK: true, SourceText: "0"}
}
