package loadout

import (
	"fmt"
	"strings"
)

// FactorStatus is whether a multiplicative/additive factor can be applied numerically.
type FactorStatus string

const (
	FactorResolved     FactorStatus = "resolved"
	FactorUnresolved   FactorStatus = "unresolved"
	FactorInapplicable FactorStatus = "inapplicable"
)

// ModelFactor is one contributor in a damage or currency model.
type ModelFactor struct {
	ID         string       `json:"id"`
	Status     FactorStatus `json:"status"`
	Magnitude  *float64     `json:"magnitude,omitempty"`
	Confidence Confidence   `json:"confidence"`
	Source     string       `json:"source"`
	Reason     string       `json:"reason"`
}

// ChannelEstimate is a per-currency or damage-channel estimate that never mixes IDs.
type ChannelEstimate struct {
	ChannelID          string        `json:"channelId"` // currency id or "pet-damage"
	WorldID            string        `json:"worldId"`
	Objective          ObjectiveID   `json:"objective"`
	ScoreFullyResolved bool          `json:"scoreFullyResolved"`
	NumericScore       *float64      `json:"numericScore,omitempty"` // only when fully resolved
	ScoreUnit          string        `json:"scoreUnit,omitempty"`
	DisplayedPowerSum  *string       `json:"displayedPowerSum,omitempty"` // NOT post-boost damage
	Factors            []ModelFactor `json:"factors"`
	UnresolvedIDs      []string      `json:"unresolvedFactorIds"`
	Assumptions        []string      `json:"assumptions"`
	Warnings           []string      `json:"warnings"`
}

// AnalyzeOptions configures modeling.
type AnalyzeOptions struct {
	WorldID    string
	Objective  Objective
	CurrencyID string // optional override; defaults from objective
}

// AnalyzeLoadout builds channel estimates without inventing multipliers.
func AnalyzeLoadout(reg Registry, pl PlayerLoadout, opt AnalyzeOptions) (ChannelEstimate, error) {
	obj := opt.Objective
	worldID := opt.WorldID
	if worldID == "" {
		worldID = "general"
		if obj.RequiresWorld != "" {
			worldID = obj.RequiresWorld
		}
	}
	currency := opt.CurrencyID
	if currency == "" {
		currency = obj.CurrencyID
	}

	est := ChannelEstimate{
		WorldID:   worldID,
		Objective: obj.ID,
		Assumptions: []string{
			"Displayed pet power is never treated as actual post-boost damage.",
			"Currency channels are independent and never combined into one power score.",
			"Unresolved registry magnitudes remain unresolved; numeric scores are omitted when incomplete.",
		},
		Warnings: append([]string{}, pl.Warnings...),
	}

	world, ok := reg.LookupWorld(worldID)
	if !ok {
		est.Warnings = append(est.Warnings, fmt.Sprintf("world %q not in registry; limits remain unknown", worldID))
	} else if world.MaxEquippedPets == nil && world.MaxEquippedConf != ConfidenceVerified {
		est.Warnings = append(est.Warnings, fmt.Sprintf("world %q pet slot limit unknown; general inventory maxEquipped must not be assumed for this world", worldID))
	}
	if obj.RequiresWorld != "" && !strings.EqualFold(worldID, obj.RequiresWorld) {
		est.Warnings = append(est.Warnings, fmt.Sprintf("objective %s expects world %s", obj.ID, obj.RequiresWorld))
	}

	switch obj.ID {
	case ObjMaxPetDamage:
		est.ChannelID = "pet-damage"
		est.ScoreUnit = "relative_model_units"
		return analyzePetDamage(reg, pl, est, worldID)
	case ObjCoinsPerMin, ObjDiamondsPerMin, ObjHalloweenCoinsPerMin, ObjTreatsPerEventActivity:
		if currency == "" {
			return est, fmt.Errorf("objective %s requires a currency id", obj.ID)
		}
		est.ChannelID = currency
		est.ScoreUnit = "currency_units_per_minute_or_activity"
		return analyzeCurrencyChannel(reg, pl, est, worldID, currency)
	case ObjBreakablesPerMin:
		est.ChannelID = "breakables"
		est.ScoreUnit = "breakables_per_minute"
		return analyzeGenericUnresolved(reg, pl, est, worldID, CategoryBreakables, "breakables-per-min requires verified breakable-rate factors")
	case ObjItemDropsPerHour:
		est.ChannelID = "item-drops"
		est.ScoreUnit = "drops_per_hour"
		return analyzeGenericUnresolved(reg, pl, est, worldID, CategoryItemDrops, "item-drops-per-hour requires verified drop-rate factors")
	case ObjHatchEfficiency:
		est.ChannelID = "hatch"
		est.ScoreUnit = "expected_cost_per_success"
		return analyzeGenericUnresolved(reg, pl, est, worldID, CategoryHatch, "hatch efficiency requires verified rates or calibrated snapshot intervals; use account snapshot diff for measured rates")
	default:
		return est, fmt.Errorf("unsupported objective %s", obj.ID)
	}
}

func analyzePetDamage(reg Registry, pl PlayerLoadout, est ChannelEstimate, worldID string) (ChannelEstimate, error) {
	// Sum displayed powers when present — labeled clearly.
	var sum int64
	have := false
	for _, p := range pl.Pets.List {
		if p.DisplayedPower == nil {
			est.Warnings = append(est.Warnings, fmt.Sprintf("equipped pet %q has no displayed power field", p.ID))
			continue
		}
		// keep as string factor only; do not claim DPS
		est.Factors = append(est.Factors, ModelFactor{
			ID: "displayed-power:" + p.ID, Status: FactorResolved, Confidence: ConfidenceUnknown,
			Source: "inventory equipped pet display field",
			Reason: "recorded displayed power " + *p.DisplayedPower + " (not post-boost damage)",
		})
		have = true
		_ = sum
	}
	if have {
		// Do not invent a numeric combined DPS; expose presence only.
		est.Assumptions = append(est.Assumptions, "displayedPowerSum omitted as a damage score; individual displayed powers listed as non-DPS factors")
	}

	// World / huge / halloween bonuses from registry — only apply resolved magnitudes.
	for _, id := range []string{"huge-pet-scaling", "halloween-games-pet-bonus"} {
		e, ok := reg.LookupEffect(id)
		if !ok {
			continue
		}
		f := factorFromEffect(e, worldID, "")
		est.Factors = append(est.Factors, f)
		if f.Status == FactorUnresolved {
			est.UnresolvedIDs = append(est.UnresolvedIDs, e.ID)
		}
	}
	for _, enc := range pl.Enchants.List {
		e, ok := reg.LookupEffect(enc.ID)
		if !ok {
			est.Factors = append(est.Factors, ModelFactor{
				ID: enc.ID, Status: FactorUnresolved, Confidence: ConfidenceUnknown,
				Source: "equipped enchant not in registry",
				Reason: "no verified magnitude; cannot invent pet-damage contribution",
			})
			est.UnresolvedIDs = append(est.UnresolvedIDs, enc.ID)
			continue
		}
		if e.Category != CategoryPetDamage && e.Category != CategoryHugeScaling {
			est.Factors = append(est.Factors, ModelFactor{
				ID: e.ID, Status: FactorInapplicable, Confidence: e.Confidence,
				Source: e.Source, Reason: "equipped but category is not pet_damage/huge_scaling for this objective",
			})
			continue
		}
		f := factorFromEffect(e, worldID, "")
		if enc.TierStatus == "unresolved" {
			f.Status = FactorUnresolved
			f.Magnitude = nil
			f.Reason = enc.TierReason + "; " + f.Reason
			est.UnresolvedIDs = append(est.UnresolvedIDs, e.ID)
		}
		est.Factors = append(est.Factors, f)
		if f.Status == FactorUnresolved {
			est.UnresolvedIDs = append(est.UnresolvedIDs, e.ID)
		}
	}

	est.ScoreFullyResolved = len(est.UnresolvedIDs) == 0 && allFactorsResolved(est.Factors)
	if !est.ScoreFullyResolved {
		est.NumericScore = nil
		est.Warnings = append(est.Warnings, "pet damage score incomplete; candidate loadouts will include uncertainty and a suggested controlled test")
	}
	return est, nil
}

func analyzeCurrencyChannel(reg Registry, pl PlayerLoadout, est ChannelEstimate, worldID, currencyID string) (ChannelEstimate, error) {
	effects := reg.EffectsForCurrency(currencyID)
	if len(effects) == 0 {
		est.Factors = append(est.Factors, ModelFactor{
			ID: "currency:" + currencyID, Status: FactorUnresolved, Confidence: ConfidenceUnknown,
			Source: "registry", Reason: "no registry effects list this currency id",
		})
		est.UnresolvedIDs = append(est.UnresolvedIDs, "currency:"+currencyID)
	}
	for _, e := range effects {
		f := factorFromEffect(e, worldID, currencyID)
		est.Factors = append(est.Factors, f)
		if f.Status == FactorUnresolved {
			est.UnresolvedIDs = append(est.UnresolvedIDs, e.ID)
		}
	}
	for _, enc := range pl.Enchants.List {
		e, ok := reg.LookupEffect(enc.ID)
		if !ok {
			est.Factors = append(est.Factors, ModelFactor{
				ID: enc.ID, Status: FactorUnresolved, Confidence: ConfidenceUnknown,
				Source: "equipped enchant", Reason: "not in registry; cannot invent currency multiplier for " + currencyID,
			})
			est.UnresolvedIDs = append(est.UnresolvedIDs, enc.ID)
			continue
		}
		applies := false
		for _, c := range e.ApplicableCurrencies {
			if strings.EqualFold(c, currencyID) {
				applies = true
				break
			}
		}
		if !applies {
			est.Factors = append(est.Factors, ModelFactor{
				ID: e.ID, Status: FactorInapplicable, Confidence: e.Confidence,
				Source: e.Source, Reason: "enchant not listed as applicable to currency " + currencyID,
			})
			continue
		}
		f := factorFromEffect(e, worldID, currencyID)
		est.Factors = append(est.Factors, f)
		if f.Status == FactorUnresolved {
			est.UnresolvedIDs = append(est.UnresolvedIDs, e.ID)
		}
	}
	_ = pl
	est.ScoreFullyResolved = len(est.UnresolvedIDs) == 0
	if !est.ScoreFullyResolved {
		est.NumericScore = nil
		est.Warnings = append(est.Warnings, fmt.Sprintf("no fully resolved multiplier chain for currency %s", currencyID))
	}
	return est, nil
}

func analyzeGenericUnresolved(reg Registry, pl PlayerLoadout, est ChannelEstimate, worldID string, cat EffectCategory, reason string) (ChannelEstimate, error) {
	est.Factors = append(est.Factors, ModelFactor{
		ID: string(cat), Status: FactorUnresolved, Confidence: ConfidenceUnknown,
		Source: "registry", Reason: reason,
	})
	est.UnresolvedIDs = append(est.UnresolvedIDs, string(cat))
	est.ScoreFullyResolved = false
	est.NumericScore = nil
	for _, e := range reg.Effects {
		if e.Category != cat {
			continue
		}
		est.Factors = append(est.Factors, factorFromEffect(e, worldID, ""))
	}
	_ = pl
	return est, nil
}

func factorFromEffect(e EffectEntry, worldID, currencyID string) ModelFactor {
	f := ModelFactor{
		ID: e.ID, Confidence: e.Confidence, Source: e.Source,
	}
	if len(e.ApplicableWorlds) > 0 {
		ok := false
		for _, w := range e.ApplicableWorlds {
			if strings.EqualFold(w, worldID) {
				ok = true
				break
			}
		}
		if !ok {
			f.Status = FactorInapplicable
			f.Reason = fmt.Sprintf("effect limited to worlds %v; current world %s", e.ApplicableWorlds, worldID)
			return f
		}
	}
	if currencyID != "" && len(e.ApplicableCurrencies) > 0 {
		ok := false
		for _, c := range e.ApplicableCurrencies {
			if strings.EqualFold(c, currencyID) {
				ok = true
				break
			}
		}
		if !ok {
			f.Status = FactorInapplicable
			f.Reason = "currency not in applicableCurrencies"
			return f
		}
	}
	if e.Magnitude == nil || e.Confidence == ConfidenceUnknown {
		f.Status = FactorUnresolved
		f.Reason = "magnitude unknown or confidence=unknown; not invented"
		return f
	}
	if e.Confidence == ConfidenceAssumed {
		f.Status = FactorResolved
		f.Magnitude = e.Magnitude
		f.Reason = "assumed magnitude — treat as provisional"
		return f
	}
	f.Status = FactorResolved
	f.Magnitude = e.Magnitude
	f.Reason = "registry magnitude with confidence=" + string(e.Confidence)
	return f
}

func allFactorsResolved(fs []ModelFactor) bool {
	for _, f := range fs {
		if f.Status == FactorUnresolved {
			return false
		}
	}
	return true
}
