package loadout

import (
	"fmt"
	"strings"
)

// RecommendOptions wraps optimize with event team limits and objective aliases.
type RecommendOptions struct {
	OptimizeOptions
	EventTeamLimit *int
	Annotations    AnnotationFile
	Public         PublicConfigIndex
}

// Recommend is the uncertainty-aware loadout recommender (deterministic).
func Recommend(reg Registry, pl PlayerLoadout, opt RecommendOptions) (OptimizeResult, error) {
	// Map aliases
	obj := opt.Objective
	switch strings.ToLower(string(obj.ID)) {
	case "damage", "max-damage", "event-damage":
		if strings.Contains(strings.ToLower(opt.WorldID), "halloween") {
			obj, _ = ParseObjective(string(ObjMaxPetDamage))
		} else {
			obj, _ = ParseObjective(string(ObjMaxPetDamage))
		}
		opt.Objective = obj
	case "currency-rate":
		if opt.CurrencyID == "" {
			return OptimizeResult{}, fmt.Errorf("--currency is required for currency-rate")
		}
		// pick closest typed objective or keep generic via coins-per-min shape
		switch opt.CurrencyID {
		case "HalloweenGamesCoins":
			obj, _ = ParseObjective(string(ObjHalloweenCoinsPerMin))
		case "HalloweenGamesTreats":
			obj, _ = ParseObjective(string(ObjTreatsPerEventActivity))
		case "Diamonds":
			obj, _ = ParseObjective(string(ObjDiamondsPerMin))
		case "Coins":
			obj, _ = ParseObjective(string(ObjCoinsPerMin))
		default:
			obj = Objective{ID: ObjCoinsPerMin, DisplayName: "Currency per minute (" + opt.CurrencyID + ")", CurrencyID: opt.CurrencyID, Category: CategoryCurrencyGain}
		}
		opt.Objective = obj
	}

	res, err := Optimize(reg, pl, opt.OptimizeOptions)
	if err != nil {
		return res, err
	}

	limit := opt.EventTeamLimit
	if limit == nil {
		limit = opt.Annotations.EventTeamLimit
	}
	if limit != nil {
		res.PetSlotLimit = limit
		res.PetSlotLimitSource = "annotation/flag eventTeamLimit"
		if len(pl.Pets.List) > *limit {
			res.Warnings = append(res.Warnings, fmt.Sprintf("equipped pets (%d) exceed event team limit (%d)", len(pl.Pets.List), *limit))
		}
	}

	// Evidence notes for Coins enchant vs event currency
	if opt.CurrencyID != "" && !strings.EqualFold(opt.CurrencyID, "Coins") {
		res.Assumptions = append(res.Assumptions,
			"Coins enchant Desc references Coins only; applicability to "+opt.CurrencyID+" is unknown unless measured")
	}
	res.Assumptions = append(res.Assumptions,
		"recommendations remain experimental when multipliers are unresolved",
		"Pareto tradeoffs across currencies are not collapsed into one score",
	)
	_ = opt.Public
	return res, nil
}
