package loadout

// HalloweenEventShare is the in-game tooltip percentage for certain Huges (2026-11-06 observation).
const HalloweenEventShare = 0.31

// HalloweenModelID names a competing event damage hypothesis.
type HalloweenModelID string

const (
	HalloweenModelA HalloweenModelID = "halloween-31-A-flat"
	HalloweenModelB HalloweenModelID = "halloween-31-B-level-variant-charm"
	HalloweenModelC HalloweenModelID = "halloween-31-C-partial-modifiers"
	HalloweenModelD HalloweenModelID = "halloween-31-D-class-coefficients"
)

// HalloweenModelPrediction is one hypothetical relative damage index.
type HalloweenModelPrediction struct {
	ModelID       HalloweenModelID `json:"modelId"`
	Description   string           `json:"description"`
	RelativeIndex *float64         `json:"relativeIndex,omitempty"` // dimensionless prediction; not verified DPS
	Assumptions   []string         `json:"assumptions"`
	Unresolved    []string         `json:"unresolved"`
}

// HalloweenModelInput feeds competing 31% models.
type HalloweenModelInput struct {
	BestEventStatRelative float64 // relative power of best event pet (caller-defined scale)
	LevelBonusPercent     float64 // displayed XP bonus percent; 0 if ignored
	VariantFactor         float64 // 1 if none/unknown-as-neutral only when explicitly established
	VariantFactorKnown    bool
	CharmDamageFactor     float64 // product of verified/assumed damage charms; 1 if none
	CharmFactorKnown      bool
	ClassCoefficient      float64
	ClassCoefficientKnown bool
	ApplyLevel            bool // model C mask
	ApplyVariant          bool
	ApplyCharms           bool
}

// PredictHalloweenModels evaluates Models A–D without selecting a winner.
func PredictHalloweenModels(in HalloweenModelInput) []HalloweenModelPrediction {
	base := HalloweenEventShare * in.BestEventStatRelative
	out := []HalloweenModelPrediction{
		{
			ModelID:     HalloweenModelA,
			Description: "Event damage = 31% of best event stat pet; ignore level and charms",
			Assumptions: []string{"in-game tooltip share 0.31 is applied to best event pet only", "level/charms do not multiply event Huge damage"},
		},
		{
			ModelID:     HalloweenModelB,
			Description: "Event damage = 31% of best event pet, then × level × variant × charm damage factors",
			Assumptions: []string{"tooltip share applies before modifiers", "level/variant/charm factors multiply"},
		},
		{
			ModelID:     HalloweenModelC,
			Description: "31% of best event pet with a partial modifier mask",
			Assumptions: []string{"subset of modifiers apply as configured by Apply* flags"},
		},
		{
			ModelID:     HalloweenModelD,
			Description: "31% model with additional class/variant coefficients",
			Assumptions: []string{"extra class coefficients exist but are unverified unless provided"},
			Unresolved:  []string{"class-specific coefficients absent from public API"},
		},
	}

	a := base
	out[0].RelativeIndex = &a

	if in.VariantFactorKnown && in.CharmFactorKnown {
		b := base * LevelFactor(in.LevelBonusPercent) * in.VariantFactor * in.CharmDamageFactor
		out[1].RelativeIndex = &b
	} else {
		out[1].Unresolved = append(out[1].Unresolved, "missing known variant and/or charm factors; Model B index withheld rather than defaulting unknowns to 1")
	}

	c := base
	un := []string{}
	if in.ApplyLevel {
		c *= LevelFactor(in.LevelBonusPercent)
	}
	if in.ApplyVariant {
		if !in.VariantFactorKnown {
			un = append(un, "variant requested but unknown")
		} else {
			c *= in.VariantFactor
		}
	}
	if in.ApplyCharms {
		if !in.CharmFactorKnown {
			un = append(un, "charms requested but unknown")
		} else {
			c *= in.CharmDamageFactor
		}
	}
	if len(un) == 0 {
		out[2].RelativeIndex = &c
	} else {
		out[2].Unresolved = un
	}

	if in.ClassCoefficientKnown && in.VariantFactorKnown && in.CharmFactorKnown {
		d := base * LevelFactor(in.LevelBonusPercent) * in.VariantFactor * in.CharmDamageFactor * in.ClassCoefficient
		out[3].RelativeIndex = &d
	} else {
		out[3].Unresolved = append(out[3].Unresolved, "class/variant/charm coefficients incomplete; Model D index withheld")
	}
	return out
}

// ExampleModelBNoCharm returns the documented hypothetical products for screenshot pets.
func ExampleModelBNoCharm() map[string]float64 {
	return map[string]float64{
		"Huge Ghost":    HalloweenEventShare * LevelFactor(59.3), // 0.31 * 1.593
		"Huge Hippokin": HalloweenEventShare * LevelFactor(56.9),
		"Huge Froggy":   HalloweenEventShare * LevelFactor(42.1),
	}
}
