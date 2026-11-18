package craft

import (
	"fmt"
	"sort"
)

const MethodIDBOM = "craft.combine-o-matic.bom.v1"

// BOMLine is one aggregated material requirement.
type BOMLine struct {
	ItemID   string `json:"itemId"`
	Quantity int    `json:"quantity"`
	Kind     string `json:"kind"` // base-pet|crafted-intermediate|spirit-shards
}

// BOMResult is a recursive bill of materials for a craft target.
type BOMResult struct {
	MethodID             string    `json:"methodId"`
	TargetRecipeID       string    `json:"targetRecipeId"`
	TargetOutput         string    `json:"targetOutput"`
	TargetCount          int       `json:"targetCount"`
	Lines                []BOMLine `json:"lines"`
	TotalSpiritShards    int       `json:"totalSpiritShards"`
	SumDurationHours     float64   `json:"sumDurationHours"`
	TheoreticalParallelH *float64  `json:"theoreticalParallelHoursIfSlotsVerified,omitempty"`
	ParallelNote         string    `json:"parallelNote"`
	Warnings             []string  `json:"warnings"`
	Provenance           string    `json:"provenance"`
}

// ExpandBOM recursively expands recipes into base materials for count copies of target.
// Intermediate crafted outputs are counted and expanded; Spirit Shards are summed.
func ExpandBOM(recipes []Recipe, targetIDOrOutput string, count int) (BOMResult, error) {
	if count <= 0 {
		return BOMResult{}, fmt.Errorf("count must be positive")
	}
	r, ok := LookupRecipe(recipes, targetIDOrOutput)
	if !ok {
		return BOMResult{}, fmt.Errorf("unknown recipe %q", targetIDOrOutput)
	}
	byOut := map[string]Recipe{}
	for _, x := range recipes {
		byOut[x.OutputItemID] = x
	}
	base := map[string]int{}
	inter := map[string]int{}
	shards := 0
	sumDur := 0.0
	warn := []string{
		"Combine-o-Matic is distinct from Halloween Forge; do not reuse Forge Saver or Forge odds",
		"theoretical parallel duration assumes verified concurrent craft slots — not claimed as supported",
	}

	var expand func(Recipe, int)
	expand = func(rec Recipe, n int) {
		shards += rec.SpiritShards * n
		sumDur += rec.DurationHours * float64(n)
		for _, ing := range rec.Ingredients {
			need := ing.Quantity * n
			if child, ok := byOut[ing.ItemID]; ok {
				inter[ing.ItemID] += need
				expand(child, need)
			} else {
				base[ing.ItemID] += need
			}
		}
	}
	expand(r, count)

	out := BOMResult{
		MethodID: MethodIDBOM, TargetRecipeID: r.ID, TargetOutput: r.OutputItemID,
		TargetCount: count, TotalSpiritShards: shards, SumDurationHours: sumDur,
		ParallelNote: "43h-style parallel paths are theoretical only until craft slot concurrency is verified from official sources or controlled observation",
		Warnings:     warn,
		Provenance:   "recursive expansion of observed Combine-o-Matic recipes",
	}
	// Critical-path parallel estimate: max chain depth durations for Gargantuan tree is documented as theoretical.
	if r.ID == "gargantuan-butler-penguin" && count == 1 {
		// Single serial chain durations: 24 + 12 + 4 + 2 + 1 = 43 if each stage waits on previous unique path
		// with max parallelism across independent branches — labeled theoretical.
		p := 43.0
		out.TheoreticalParallelH = &p
	}
	for id, q := range base {
		out.Lines = append(out.Lines, BOMLine{ItemID: id, Quantity: q, Kind: "base-pet"})
	}
	for id, q := range inter {
		out.Lines = append(out.Lines, BOMLine{ItemID: id, Quantity: q, Kind: "crafted-intermediate"})
	}
	out.Lines = append(out.Lines, BOMLine{ItemID: "Spirit Shards", Quantity: shards, Kind: "spirit-shards"})
	sort.Slice(out.Lines, func(i, j int) bool {
		if out.Lines[i].Kind != out.Lines[j].Kind {
			return out.Lines[i].Kind < out.Lines[j].Kind
		}
		return out.Lines[i].ItemID < out.Lines[j].ItemID
	})
	return out, nil
}
