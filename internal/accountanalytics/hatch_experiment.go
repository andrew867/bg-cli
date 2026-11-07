package accountanalytics

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
)

const ExperimentSchemaVersion = "1.0"
const MethodIDHatchExperiment = "accountanalytics.hatch-experiment.v1"

// HatchExperimentObservation is a versioned offline hatch experiment record.
// Possession of an enchant/potion is not evidence it was equipped/active.
type HatchExperimentObservation struct {
	SchemaVersion         string   `json:"schemaVersion"`
	ID                    string   `json:"id"`
	EventID               string   `json:"eventId,omitempty"`
	EggID                 string   `json:"eggId"`
	CheckpointLabel       string   `json:"checkpointLabel,omitempty"`
	ObservedAt            string   `json:"observedAt,omitempty"`
	Timezone              string   `json:"timezone,omitempty"`
	EggsCumulative        *int64   `json:"eggsCumulative,omitempty"` // PurchasedEggs or observed cumulative hatches
	RarePetID             string   `json:"rarePetId,omitempty"`
	RareCumulative        *int64   `json:"rareCumulative,omitempty"`
	EggsPerPurchase       *int     `json:"eggsPerPurchase,omitempty"`       // only when evidenced
	EggsPerPurchaseSource string   `json:"eggsPerPurchaseSource,omitempty"` // screenshot|config|api-EggHatchCount|unknown
	APIEggHatchCount      *int     `json:"apiEggHatchCount,omitempty"`
	DisplayedPriceRaw     string   `json:"displayedPriceRaw,omitempty"`
	CurrencyID            string   `json:"currencyId,omitempty"`
	CocktailActive        *bool    `json:"cocktailActive,omitempty"`
	CocktailTier          string   `json:"cocktailTier,omitempty"`
	LuckyEggs             []string `json:"luckyEggs,omitempty"` // e.g. "IX","IX","VIII","VIII","VIII"
	EnchantsDeclared      []string `json:"enchantsDeclared,omitempty"`
	PotionsDeclared       []string `json:"potionsDeclared,omitempty"`
	EventLuckNotes        string   `json:"eventLuckNotes,omitempty"`
	Interruptions         []string `json:"interruptions,omitempty"`
	Notes                 string   `json:"notes,omitempty"`
	Provenance            string   `json:"provenance,omitempty"`
	Confidence            string   `json:"confidence,omitempty"`
}

// HatchExperimentFile is a collection of observations.
type HatchExperimentFile struct {
	SchemaVersion string                       `json:"schemaVersion"`
	Observations  []HatchExperimentObservation `json:"observations"`
	Notes         []string                     `json:"notes,omitempty"`
}

// LoadHatchExperiments reads a versioned experiment JSON file.
func LoadHatchExperiments(path string) (HatchExperimentFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return HatchExperimentFile{}, err
	}
	var f HatchExperimentFile
	if err := json.Unmarshal(b, &f); err != nil {
		return HatchExperimentFile{}, err
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = ExperimentSchemaVersion
	}
	return f, nil
}

// IntervalStats is derived between two ordered checkpoints for the same egg/rare.
type IntervalStats struct {
	FromID           string   `json:"fromId"`
	ToID             string   `json:"toId"`
	EggDelta         int64    `json:"eggDelta"`
	RareDelta        int64    `json:"rareDelta"`
	ObservedRate     float64  `json:"observedRate"`
	EggsPerSuccess   *float64 `json:"eggsPerSuccess,omitempty"`
	PurchasesIfKnown *int64   `json:"purchasesIfKnown,omitempty"`
	WilsonLow95      float64  `json:"wilsonLow95"`
	WilsonHigh95     float64  `json:"wilsonHigh95"`
	CocktailActive   *bool    `json:"cocktailActive,omitempty"`
	Warnings         []string `json:"warnings,omitempty"`
	MethodID         string   `json:"methodId"`
}

// CompareHatchExperiments computes interval stats and pairwise cocktail warnings.
func CompareHatchExperiments(file HatchExperimentFile) ([]IntervalStats, []string, error) {
	var intervals []IntervalStats
	var notes []string
	notes = append(notes,
		"observed rates are historical frequencies, not authoritative server probabilities",
		"rate ratio between Cocktail and no-Cocktail is not a causal claim",
		"possession of Lucky Eggs / Cocktail is not proof they were active unless recorded",
	)
	obs := file.Observations
	for i := 0; i+1 < len(obs); i++ {
		a, b := obs[i], obs[i+1]
		if a.EggID == "" || a.EggID != b.EggID {
			continue
		}
		if a.EggsCumulative == nil || b.EggsCumulative == nil || a.RareCumulative == nil || b.RareCumulative == nil {
			continue
		}
		eggDelta := *b.EggsCumulative - *a.EggsCumulative
		rareDelta := *b.RareCumulative - *a.RareCumulative
		if eggDelta <= 0 {
			continue
		}
		rate := float64(rareDelta) / float64(eggDelta)
		lo, hi := wilson(float64(rareDelta), float64(eggDelta), 1.959963984540054)
		st := IntervalStats{
			FromID: a.ID, ToID: b.ID, EggDelta: eggDelta, RareDelta: rareDelta,
			ObservedRate: rate, WilsonLow95: lo, WilsonHigh95: hi,
			CocktailActive: b.CocktailActive, MethodID: MethodIDHatchExperiment,
		}
		if rareDelta > 0 {
			eps := float64(eggDelta) / float64(rareDelta)
			st.EggsPerSuccess = &eps
		}
		if a.EggsPerPurchase != nil && *a.EggsPerPurchase > 0 {
			p := eggDelta / int64(*a.EggsPerPurchase)
			st.PurchasesIfKnown = &p
		}
		if a.APIEggHatchCount != nil && a.EggsPerPurchase != nil && *a.APIEggHatchCount != *a.EggsPerPurchase {
			st.Warnings = append(st.Warnings, fmt.Sprintf(
				"API EggHatchCount=%d differs from observed/configured eggsPerPurchase=%d — field semantics unresolved; do not assume equivalence",
				*a.APIEggHatchCount, *a.EggsPerPurchase,
			))
		}
		intervals = append(intervals, st)
	}

	// Cocktail vs no-Cocktail aggregate if both present in notes-friendly form
	var cockEggs, cockRare, plainEggs, plainRare int64
	for _, st := range intervals {
		if st.CocktailActive != nil && *st.CocktailActive {
			cockEggs += st.EggDelta
			cockRare += st.RareDelta
		} else if st.CocktailActive != nil && !*st.CocktailActive {
			plainEggs += st.EggDelta
			plainRare += st.RareDelta
		}
	}
	if cockEggs > 0 && plainEggs > 0 {
		cr := float64(cockRare) / float64(cockEggs)
		pr := float64(plainRare) / float64(plainEggs)
		notes = append(notes, fmt.Sprintf("Cocktail aggregate observed rate≈%.6f vs no-Cocktail≈%.6f (confounded; not causation)", cr, pr))
	}
	return intervals, notes, nil
}

func wilson(successes, trials, z float64) (low, high float64) {
	if trials <= 0 {
		return 0, 1
	}
	p := successes / trials
	z2 := z * z
	den := 1 + z2/trials
	center := p + z2/(2*trials)
	margin := z * math.Sqrt(p*(1-p)/trials+z2/(4*trials*trials))
	low = (center - margin) / den
	high = (center + margin) / den
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return low, high
}
