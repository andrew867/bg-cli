package craft

import (
	"fmt"
	"strings"
)

const MethodIDOptimize = "craft.combine-o-matic.optimize.v1"

// InventoryStock maps itemId|variant → quantity.
type InventoryStock map[string]int

// OptimizeObjective selects a planning goal.
type OptimizeObjective string

const (
	ObjEarliestFirstHuge OptimizeObjective = "earliest-first-huge"
	ObjMinExtraAgonies   OptimizeObjective = "min-extra-agonies"
	ObjMinExtraShards    OptimizeObjective = "min-extra-shards"
	ObjMaxProgression    OptimizeObjective = "max-progression"
)

// OptimizeOptions configures a read-only craft plan.
type OptimizeOptions struct {
	Objective      OptimizeObjective
	TargetRecipe   string
	Stock          InventoryStock
	AllowProtected bool // if false, skip golden/rainbow/shiny unless recipe requires that variant
	Recipes        []Recipe
}

// PlanStep is one suggested craft (never auto-executed).
type PlanStep struct {
	RecipeID     string   `json:"recipeId"`
	Output       string   `json:"output"`
	Count        int      `json:"count"`
	ShardsNeeded int      `json:"spiritShardsNeeded"`
	DurationH    float64  `json:"durationHours"`
	Notes        []string `json:"notes,omitempty"`
}

// OptimizeReport is a read-only craft plan.
type OptimizeReport struct {
	OK          bool              `json:"ok"`
	MethodID    string            `json:"methodId"`
	Objective   OptimizeObjective `json:"objective"`
	BOM         *BOMResult        `json:"bom,omitempty"`
	Steps       []PlanStep        `json:"steps"`
	Shortfalls  []BOMLine         `json:"shortfalls,omitempty"`
	Warnings    []string          `json:"warnings"`
	Assumptions []string          `json:"assumptions"`
}

func stockKey(id string, v VariantClass) string {
	return id + "|" + string(v)
}

// OptimizePlan builds a read-only plan from inventory vs BOM. Never crafts.
func OptimizePlan(opt OptimizeOptions) (OptimizeReport, error) {
	recipes := opt.Recipes
	if len(recipes) == 0 {
		recipes = DefaultMasqueradeRecipes()
	}
	target := opt.TargetRecipe
	if target == "" {
		target = "huge-bat-hoodie-bunny"
		if opt.Objective == ObjMaxProgression {
			target = "gargantuan-butler-penguin"
		}
	}
	bom, err := ExpandBOM(recipes, target, 1)
	if err != nil {
		return OptimizeReport{}, err
	}
	rep := OptimizeReport{
		OK: true, MethodID: MethodIDOptimize, Objective: opt.Objective, BOM: &bom,
		Assumptions: []string{
			"plan is advisory only — CLI never submits crafts",
			"valuable variants protected by default unless recipe AllowedVariants requires them",
			"possession is not equipment; stock map should reflect craftable inventory",
		},
		Warnings: append([]string{}, bom.Warnings...),
	}
	if !opt.AllowProtected {
		rep.Assumptions = append(rep.Assumptions, "Golden/Rainbow/Shiny/Huge/Titanic/Gargantuan not consumed unless recipe requires that class")
	}

	// Shortfalls vs Normal-variant stock by default.
	for _, line := range bom.Lines {
		if line.Kind == "spirit-shards" {
			have := opt.Stock[stockKey("Spirit Shards", VariantNormal)]
			if have == 0 {
				have = opt.Stock["Spirit Shards"]
			}
			if have < line.Quantity {
				rep.Shortfalls = append(rep.Shortfalls, BOMLine{ItemID: line.ItemID, Quantity: line.Quantity - have, Kind: line.Kind})
			}
			continue
		}
		if line.Kind == "crafted-intermediate" {
			continue // produced by plan steps
		}
		have := opt.Stock[stockKey(line.ItemID, VariantNormal)]
		if have == 0 {
			have = opt.Stock[line.ItemID]
		}
		if have < line.Quantity {
			rep.Shortfalls = append(rep.Shortfalls, BOMLine{ItemID: line.ItemID, Quantity: line.Quantity - have, Kind: line.Kind})
		}
	}

	rec, _ := LookupRecipe(recipes, target)
	rep.Steps = append(rep.Steps, PlanStep{
		RecipeID: rec.ID, Output: rec.OutputItemID, Count: 1,
		ShardsNeeded: rec.SpiritShards, DurationH: rec.DurationHours,
		Notes: []string{"single-target plan; expand intermediates separately via recipes list"},
	})

	switch opt.Objective {
	case ObjEarliestFirstHuge:
		bunny, _ := LookupRecipe(recipes, "huge-bat-hoodie-bunny")
		rep.Steps = []PlanStep{{
			RecipeID: bunny.ID, Output: bunny.OutputItemID, Count: 1,
			ShardsNeeded: bunny.SpiritShards, DurationH: bunny.DurationHours,
			Notes: []string{"earliest first-stage huge under observed recipes (1h)"},
		}}
		bom2, _ := ExpandBOM(recipes, bunny.ID, 1)
		rep.BOM = &bom2
	case ObjMinExtraAgonies, ObjMinExtraShards, ObjMaxProgression:
		// BOM shortfalls already computed for target
	default:
		if opt.Objective != "" {
			return rep, fmt.Errorf("unknown objective %q", opt.Objective)
		}
	}

	for _, line := range bom.Lines {
		if strings.EqualFold(line.ItemID, "Masquerade Agony") && line.Quantity == 480 && target == "gargantuan-butler-penguin" {
			rep.Assumptions = append(rep.Assumptions, "Gargantuan BOM regression: 480 Normal Agonies expected for 1× target with no discounts")
		}
	}
	return rep, nil
}
