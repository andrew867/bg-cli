package loadout

import (
	"fmt"
	"strings"
)

// ObjectiveID identifies an optimization goal.
type ObjectiveID string

const (
	ObjMaxPetDamage           ObjectiveID = "max-pet-damage"
	ObjCoinsPerMin            ObjectiveID = "coins-per-min"
	ObjDiamondsPerMin         ObjectiveID = "diamonds-per-min"
	ObjHalloweenCoinsPerMin   ObjectiveID = "halloween-coins-per-min"
	ObjTreatsPerEventActivity ObjectiveID = "treats-per-event-activity"
	ObjBreakablesPerMin       ObjectiveID = "breakables-per-min"
	ObjItemDropsPerHour       ObjectiveID = "item-drops-per-hour"
	ObjHatchEfficiency        ObjectiveID = "hatch-efficiency"
)

// Objective describes a named goal and its currency channel (if any).
type Objective struct {
	ID            ObjectiveID    `json:"id"`
	DisplayName   string         `json:"displayName"`
	CurrencyID    string         `json:"currencyId,omitempty"` // empty for non-currency goals
	Category      EffectCategory `json:"effectCategory"`
	RequiresWorld string         `json:"requiresWorld,omitempty"`
}

// KnownObjectives is the supported objective catalogue.
func KnownObjectives() []Objective {
	return []Objective{
		{ID: ObjMaxPetDamage, DisplayName: "Maximum pet damage (modeled factors only)", Category: CategoryPetDamage},
		{ID: ObjCoinsPerMin, DisplayName: "Coins per minute", CurrencyID: "Coins", Category: CategoryCurrencyGain},
		{ID: ObjDiamondsPerMin, DisplayName: "Diamonds per minute", CurrencyID: "Diamonds", Category: CategoryCurrencyGain},
		{ID: ObjHalloweenCoinsPerMin, DisplayName: "Halloween Candy Coins per minute", CurrencyID: "HalloweenGamesCoins", Category: CategoryCurrencyGain, RequiresWorld: "halloween-games"},
		{ID: ObjTreatsPerEventActivity, DisplayName: "Treats per event activity", CurrencyID: "HalloweenGamesTreats", Category: CategoryEventActivity, RequiresWorld: "halloween-games"},
		{ID: ObjBreakablesPerMin, DisplayName: "Breakables per minute", Category: CategoryBreakables},
		{ID: ObjItemDropsPerHour, DisplayName: "Item drops per hour", Category: CategoryItemDrops},
		{ID: ObjHatchEfficiency, DisplayName: "Hatch efficiency and expected cost", Category: CategoryHatch},
	}
}

func ParseObjective(s string) (Objective, error) {
	s = strings.TrimSpace(s)
	for _, o := range KnownObjectives() {
		if strings.EqualFold(string(o.ID), s) {
			return o, nil
		}
	}
	return Objective{}, fmt.Errorf("unknown objective %q; see --list-objectives", s)
}
