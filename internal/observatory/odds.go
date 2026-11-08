package observatory

import (
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

// ResultLabel distinguishes hypothetical models from public-config arithmetic.
type ResultLabel string

const (
	LabelHypothetical     ResultLabel = "Hypothetical"
	LabelPublicConfigMath ResultLabel = "PublicConfigMath"
)

// OddsModel identifies a probability interpretation.
type OddsModel string

const (
	ModelDirectPercent       OddsModel = "direct-percent"
	ModelNormalizedWeight    OddsModel = "normalized-weight"
	ModelIndependentRareGate OddsModel = "independent-rare-gate"
)

// PetOddsLine is effective probability for one pet under a model.
type PetOddsLine struct {
	PetID               string      `json:"petId"`
	RawWeight           string      `json:"rawWeight"`
	Label               string      `json:"label,omitempty"`
	EffectiveProb       float64     `json:"effectiveProbability"`
	OneInN              float64     `json:"oneInN"`
	Model               OddsModel   `json:"model"`
	Interpretation      ResultLabel `json:"interpretation"`
	Assumption          string      `json:"assumption"`
	NotVerifiedLiveOdds bool        `json:"notVerifiedLiveOdds"`
}

// EggOddsReport summarizes one egg under a model.
type EggOddsReport struct {
	EggDisplayName string        `json:"eggDisplayName"`
	Model          OddsModel     `json:"model"`
	Lines          []PetOddsLine `json:"lines"`
	Notes          []string      `json:"notes"`
}

// ComputeEggOdds applies a model to parsed egg pets.
func ComputeEggOdds(doc EggDocument, model OddsModel) (EggOddsReport, error) {
	if len(doc.Pets) == 0 {
		return EggOddsReport{}, fmt.Errorf("no pets table on egg %q", doc.DisplayName)
	}
	report := EggOddsReport{
		EggDisplayName: doc.DisplayName,
		Model:          model,
		Notes: []string{
			"results are not verified live player odds",
			"raw weights are never silently treated as percents without an explicit model",
		},
	}
	switch model {
	case ModelDirectPercent:
		report = applyDirectPercent(doc, report)
	case ModelNormalizedWeight:
		report = applyNormalizedWeight(doc, report)
	case ModelIndependentRareGate:
		report = applyIndependentRareGate(doc, report)
	default:
		return EggOddsReport{}, fmt.Errorf("unknown model %q", model)
	}
	return report, nil
}

func applyDirectPercent(doc EggDocument, rep EggOddsReport) EggOddsReport {
	for _, p := range doc.Pets {
		w, ok := parseWeightFloat(p.RawWeight)
		line := PetOddsLine{
			PetID: p.ID, RawWeight: p.RawWeight, Label: p.Label,
			Model: ModelDirectPercent, Interpretation: LabelHypothetical,
			Assumption:          "each raw weight is a percent point (weight/100)",
			NotVerifiedLiveOdds: true,
		}
		if !ok {
			line.Assumption += "; unparsable weight"
			rep.Lines = append(rep.Lines, line)
			continue
		}
		line.EffectiveProb = w / 100
		line.OneInN = OneInN(line.EffectiveProb)
		rep.Lines = append(rep.Lines, line)
	}
	return rep
}

func applyNormalizedWeight(doc EggDocument, rep EggOddsReport) EggOddsReport {
	var sum float64
	weights := make([]float64, len(doc.Pets))
	ok := make([]bool, len(doc.Pets))
	for i, p := range doc.Pets {
		w, parsed := parseWeightFloat(p.RawWeight)
		weights[i] = w
		ok[i] = parsed
		if parsed {
			sum += w
		}
	}
	for i, p := range doc.Pets {
		line := PetOddsLine{
			PetID: p.ID, RawWeight: p.RawWeight, Label: p.Label,
			Model: ModelNormalizedWeight, Interpretation: LabelHypothetical,
			Assumption:          "p_i = w_i / sum(w)",
			NotVerifiedLiveOdds: true,
		}
		if !ok[i] || sum <= 0 {
			rep.Lines = append(rep.Lines, line)
			continue
		}
		line.EffectiveProb = weights[i] / sum
		line.OneInN = OneInN(line.EffectiveProb)
		rep.Lines = append(rep.Lines, line)
	}
	return rep
}

func applyIndependentRareGate(doc EggDocument, rep EggOddsReport) EggOddsReport {
	commons, rares := partitionRare(doc.Pets)
	var commonSum float64
	for _, p := range commons {
		if w, ok := parseWeightFloat(p.RawWeight); ok {
			commonSum += w
		}
	}
	for _, p := range commons {
		line := PetOddsLine{
			PetID: p.ID, RawWeight: p.RawWeight, Label: p.Label,
			Model: ModelIndependentRareGate, Interpretation: LabelHypothetical,
			Assumption:          "commons compete only among themselves; rare labeled/<<1 pets are not folded into common pool",
			NotVerifiedLiveOdds: true,
		}
		w, parsed := parseWeightFloat(p.RawWeight)
		if parsed && commonSum > 0 {
			line.EffectiveProb = w / commonSum
		}
		line.OneInN = OneInN(line.EffectiveProb)
		rep.Lines = append(rep.Lines, line)
	}
	for _, p := range rares {
		line := PetOddsLine{
			PetID: p.ID, RawWeight: p.RawWeight, Label: p.Label,
			Model: ModelIndependentRareGate, Interpretation: LabelHypothetical,
			Assumption:          "independent rare gate: raw weight used as literal probability fraction (not normalized into commons)",
			NotVerifiedLiveOdds: true,
		}
		if w, ok := parseWeightFloat(p.RawWeight); ok {
			line.EffectiveProb = w
		}
		line.OneInN = OneInN(line.EffectiveProb)
		rep.Lines = append(rep.Lines, line)
	}
	return rep
}

func partitionRare(pets []EggPetEntry) (commons, rares []EggPetEntry) {
	hasLarge := false
	for _, p := range pets {
		w, ok := parseWeightFloat(p.RawWeight)
		if ok && w >= 1 {
			hasLarge = true
			break
		}
	}
	for _, p := range pets {
		if isRarePet(p, hasLarge) {
			rares = append(rares, p)
		} else {
			commons = append(commons, p)
		}
	}
	return commons, rares
}

func isRarePet(p EggPetEntry, hasLarge bool) bool {
	lbl := strings.TrimSpace(p.Label)
	if strings.EqualFold(lbl, "Great") || strings.EqualFold(lbl, "Insane") {
		return true
	}
	w, ok := parseWeightFloat(p.RawWeight)
	if ok && hasLarge && w < 1 {
		return true
	}
	return false
}

// EffectiveProbability returns p under normalized-weight for one pet id.
func EffectiveProbability(doc EggDocument, model OddsModel, petID string) (float64, error) {
	rep, err := ComputeEggOdds(doc, model)
	if err != nil {
		return 0, err
	}
	for _, ln := range rep.Lines {
		if ln.PetID == petID {
			return ln.EffectiveProb, nil
		}
	}
	return 0, fmt.Errorf("pet %q not found", petID)
}

// OneInN returns 1/p with guards.
func OneInN(p float64) float64 {
	if p <= 0 || math.IsNaN(p) || math.IsInf(p, 0) {
		return math.Inf(1)
	}
	if p >= 1 {
		return 1
	}
	return 1 / p
}

// PAtLeastOne computes 1-(1-p)^n with stable log1p/expm1.
func PAtLeastOne(p, n float64) float64 {
	if n <= 0 {
		return 0
	}
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	return -math.Expm1(n * math.Log1p(-p))
}

// EggsForConfidence returns trials needed for probability conf of at least one success.
func EggsForConfidence(p, conf float64) (float64, error) {
	if p <= 0 || p > 1 {
		return 0, fmt.Errorf("invalid p")
	}
	if conf <= 0 || conf >= 1 {
		return 0, fmt.Errorf("invalid confidence")
	}
	// 1 - (1-p)^n >= conf → n >= log(1-conf) / log(1-p)
	num := math.Log(1 - conf)
	den := math.Log1p(-p)
	if den == 0 {
		return math.Inf(1), nil
	}
	n := num / den
	if n < 0 {
		n = -n
	}
	return n, nil
}

// ThornBoostMultiplier solves  (w*m) / (sumOthers + w*m) = target for one boosted common.
func ThornBoostMultiplier(commonOthersSum, boostedWeight, target float64) float64 {
	if boostedWeight <= 0 || target <= 0 || target >= 1 {
		return math.NaN()
	}
	// boostedWeight*m / (commonOthersSum + boostedWeight*m) = target
	// m = target*commonOthersSum / (boostedWeight*(1-target))
	return target * commonOthersSum / (boostedWeight * (1 - target))
}

// OddsWithWeightMultiplier recomputes normalized-weight odds with a multiplier on one pet.
func OddsWithWeightMultiplier(doc EggDocument, petID string, mult float64) (EggOddsReport, error) {
	if mult <= 0 {
		return EggOddsReport{}, fmt.Errorf("multiplier must be positive")
	}
	var sum float64
	type wRow struct {
		w   float64
		ok  bool
		pet EggPetEntry
	}
	rows := make([]wRow, len(doc.Pets))
	for i, p := range doc.Pets {
		w, ok := parseWeightFloat(p.RawWeight)
		if p.ID == petID {
			w *= mult
		}
		rows[i] = wRow{w: w, ok: ok, pet: p}
		if ok {
			sum += w
		}
	}
	rep := EggOddsReport{
		EggDisplayName: doc.DisplayName,
		Model:          ModelNormalizedWeight,
		Notes: []string{
			fmt.Sprintf("weight multiplier %.4g applied to %q only", mult, petID),
			"Hypothetical fitted scenario — not verified live odds",
		},
	}
	for _, r := range rows {
		line := PetOddsLine{
			PetID: r.pet.ID, RawWeight: r.pet.RawWeight, Label: r.pet.Label,
			Model: ModelNormalizedWeight, Interpretation: LabelHypothetical,
			Assumption:          "normalized after per-pet multiplier",
			NotVerifiedLiveOdds: true,
		}
		if r.ok && sum > 0 {
			line.EffectiveProb = r.w / sum
			line.OneInN = OneInN(line.EffectiveProb)
		}
		rep.Lines = append(rep.Lines, line)
	}
	return rep, nil
}

// SensitivityRow is one multiplier scenario.
type SensitivityRow struct {
	PetID         string      `json:"petId"`
	Multiplier    float64     `json:"multiplier"`
	EffectiveProb float64     `json:"effectiveProbability"`
	OneInN        float64     `json:"oneInN"`
	Label         ResultLabel `json:"label"`
}

// SensitivityReport varies multipliers on selected pets.
func SensitivityReport(doc EggDocument, petMultipliers map[string][]float64) map[string][]SensitivityRow {
	out := map[string][]SensitivityRow{}
	for petID, mults := range petMultipliers {
		for _, m := range mults {
			rep, err := OddsWithWeightMultiplier(doc, petID, m)
			if err != nil {
				continue
			}
			var prob float64
			for _, ln := range rep.Lines {
				if ln.PetID == petID {
					prob = ln.EffectiveProb
					break
				}
			}
			out[petID] = append(out[petID], SensitivityRow{
				PetID: petID, Multiplier: m,
				EffectiveProb: prob, OneInN: OneInN(prob),
				Label: LabelHypothetical,
			})
		}
	}
	return out
}

// OneSidedUpperBound returns a conservative upper bound when successes=0.
func OneSidedUpperBound(successes, trials float64, confidence float64) float64 {
	if trials <= 0 {
		return 1
	}
	if successes > 0 {
		return successes / trials // observed rate; caller should use Wilson for two-sided
	}
	// Rule of three: upper ≈ -ln(1-conf)/n for one-sided
	if confidence <= 0 || confidence >= 1 {
		confidence = 0.95
	}
	return -math.Log(1-confidence) / trials
}

// ObservedRate computes successes/trials as exact rational when possible.
func ObservedRate(successes, trials int64) float64 {
	if trials <= 0 {
		return math.NaN()
	}
	tf := new(big.Float).SetInt64(trials)
	sf := new(big.Float).SetInt64(successes)
	q, _ := new(big.Float).Quo(sf, tf).Float64()
	return q
}

func parseWeightFloat(s string) (float64, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, false
	}
	v, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false
	}
	return v, true
}

// EnsureNeverVerifiedLive panics in tests if a label claims live verification.
func EnsureNeverVerifiedLive(label string) bool {
	lower := strings.ToLower(label)
	return !strings.Contains(lower, "verified live") && !strings.Contains(lower, "current live odds")
}
