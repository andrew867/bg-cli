package forge

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ObjectiveID names a forge optimization goal.
type ObjectiveID string

const (
	ObjMinButlers       ObjectiveID = "min-butlers"
	ObjMinShards        ObjectiveID = "min-shards"
	ObjMaxSuccess       ObjectiveID = "max-success"
	ObjMaxSuccessBudget ObjectiveID = "max-success-budget"
	ObjWeightedCost     ObjectiveID = "weighted-cost"
	ObjMaxExpectedValue ObjectiveID = "max-expected-value"
)

// ResourceWeights assigns relative costs per pet id (and optional variant key "id|variant").
type ResourceWeights struct {
	PerPetID      map[string]float64 `json:"perPetId"`
	PerVariantKey map[string]float64 `json:"perVariantKey,omitempty"` // "Butler Penguin|Shiny"
	SpiritShard   float64            `json:"spiritShard"`
	AllowShiny    bool               `json:"allowShiny"`
	AllowRainbow  bool               `json:"allowRainbow"`
	AllowGolden   bool               `json:"allowGolden"`
}

// InventoryAvailability maps pet id (+ optional variant) to usable quantity.
type InventoryAvailability map[string]int

// OptimizeOptions configures recipe search.
type OptimizeOptions struct {
	Objective        ObjectiveID
	Weights          ResourceWeights
	Inventory        InventoryAvailability // key: "id" or "id|variant"
	ButlerBudget     int
	ShardsPerAttempt int
	Model            CappedLinearModel
	Step             int // butler step size
	MaxButlers       int
	MinButlers       int
	ProtectVariants  bool // default true: exclude shiny/rainbow unless allowed
}

// RecipeCandidate is one evaluated forge recipe.
type RecipeCandidate struct {
	Butlers                   int      `json:"butlers"`
	SpiritShards              int      `json:"spiritShards"`
	DisplayedWinProbability   float64  `json:"displayedWinProbabilityFraction"`
	DisplayedWinPercent       string   `json:"displayedWinPercent"`
	ProbabilitySource         string   `json:"probabilitySource"` // modeled|observed|unknown
	ExpectedButlersPerSuccess *float64 `json:"expectedButlersPerSuccess,omitempty"`
	ExpectedShardsPerSuccess  *float64 `json:"expectedShardsPerSuccess,omitempty"`
	WeightedCostPerAttempt    *float64 `json:"weightedCostPerAttempt,omitempty"`
	WeightedCostPerSuccess    *float64 `json:"weightedCostPerSuccess,omitempty"`
	Dominated                 bool     `json:"dominated"`
	Feasible                  bool     `json:"feasible"`
	Notes                     []string `json:"notes,omitempty"`
	ObjectiveScore            float64  `json:"objectiveScore"`
}

// OptimizeReport is the optimizer output.
type OptimizeReport struct {
	OK          bool              `json:"ok"`
	Objective   ObjectiveID       `json:"objective"`
	Candidates  []RecipeCandidate `json:"candidates"`
	ParetoFront []RecipeCandidate `json:"paretoFront"`
	Warnings    []string          `json:"warnings"`
	Assumptions []string          `json:"assumptions"`
	MethodID    string            `json:"methodId"`
}

// DefaultButlerWeights returns unit butler cost and shard cost 1.
func DefaultButlerWeights() ResourceWeights {
	return ResourceWeights{
		PerPetID:     map[string]float64{"Butler Penguin": 1},
		SpiritShard:  1,
		AllowGolden:  true,
		AllowShiny:   false,
		AllowRainbow: false,
	}
}

// OptimizeButlerOnly searches discrete butler counts using a fitted capped-linear model.
func OptimizeButlerOnly(opt OptimizeOptions) (OptimizeReport, error) {
	rep := OptimizeReport{
		OK:        true,
		Objective: opt.Objective,
		MethodID:  "forge-optimize-butler-only.v1",
		Assumptions: []string{
			"independent attempts with stable displayed probabilities",
			"materials fully consumed each attempt",
			"fallback shard recovery not credited unless known",
			"expectedButlersPerSuccess = n / p using displayed/modeled p",
		},
		Warnings: []string{},
	}
	if opt.Model.Slope <= 0 && opt.Model.Cap == nil {
		return rep, fmt.Errorf("model slope/cap required for butler-only optimize")
	}
	if opt.Step <= 0 {
		opt.Step = 1
	}
	if opt.MinButlers <= 0 {
		opt.MinButlers = 1
	}
	if opt.MaxButlers <= 0 {
		opt.MaxButlers = 40
	}
	if opt.ShardsPerAttempt <= 0 {
		opt.ShardsPerAttempt = 100
	}
	if opt.ProtectVariants {
		rep.Assumptions = append(rep.Assumptions, "Shiny/Rainbow Butlers excluded unless explicitly allowed in weights")
	}

	avail := opt.Inventory["Butler Penguin"]
	if avail == 0 {
		avail = opt.Inventory["Butler Penguin|Regular"]
	}
	if avail > 0 && avail < opt.MaxButlers {
		opt.MaxButlers = avail
	}
	if avail == 0 && len(opt.Inventory) > 0 {
		rep.Warnings = append(rep.Warnings, "no Butler Penguin availability in inventory map; searching hypothetical quantities")
	}

	var cands []RecipeCandidate
	for n := opt.MinButlers; n <= opt.MaxButlers; n += opt.Step {
		if opt.ButlerBudget > 0 && n > opt.ButlerBudget && opt.Objective == ObjMaxSuccessBudget {
			continue
		}
		p := opt.Model.Predict(n)
		c := RecipeCandidate{
			Butlers: n, SpiritShards: opt.ShardsPerAttempt,
			DisplayedWinProbability: p,
			DisplayedWinPercent:     FormatPercent(p, 2),
			ProbabilitySource:       "modeled",
			Feasible:                true,
			Notes:                   []string{},
		}
		if eb, err := ExpectedButlersPerSuccess(n, p); err == nil {
			c.ExpectedButlersPerSuccess = &eb
		}
		if es, err := ExpectedShardsPerSuccess(opt.ShardsPerAttempt, p); err == nil {
			c.ExpectedShardsPerSuccess = &es
		}
		costAttempt := float64(n)*weightFor(opt.Weights, "Butler Penguin", "Regular") + float64(opt.ShardsPerAttempt)*opt.Weights.SpiritShard
		c.WeightedCostPerAttempt = &costAttempt
		if p > 0 {
			cs := costAttempt / p
			c.WeightedCostPerSuccess = &cs
		}
		c.ObjectiveScore = scoreCandidate(opt.Objective, c, opt.ButlerBudget)
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		rep.OK = false
		rep.Warnings = append(rep.Warnings, "no feasible recipes")
		return rep, nil
	}

	// Mark dominated on (expected butlers/success, expected shards/success) — lower is better.
	for i := range cands {
		for j := range cands {
			if i == j || !cands[j].Feasible {
				continue
			}
			if dominates(cands[j], cands[i]) {
				cands[i].Dominated = true
				break
			}
		}
	}
	sort.SliceStable(cands, func(i, j int) bool {
		return cands[i].ObjectiveScore < cands[j].ObjectiveScore
	})
	rep.Candidates = cands
	for _, c := range cands {
		if !c.Dominated && c.Feasible {
			rep.ParetoFront = append(rep.ParetoFront, c)
		}
	}
	// Highlight 29 vs 30 efficiency note when both present.
	var c29, c30 *RecipeCandidate
	for i := range cands {
		if cands[i].Butlers == 29 {
			c29 = &cands[i]
		}
		if cands[i].Butlers == 30 {
			c30 = &cands[i]
		}
	}
	if c29 != nil && c30 != nil && c29.ExpectedButlersPerSuccess != nil && c30.ExpectedButlersPerSuccess != nil {
		if *c30.ExpectedButlersPerSuccess > *c29.ExpectedButlersPerSuccess {
			rep.Warnings = append(rep.Warnings, "30 Butlers hits the displayed cap but is less Butler-efficient than 29 under this model; choose objective deliberately")
		}
	}
	return rep, nil
}

func weightFor(w ResourceWeights, id, variant string) float64 {
	if variant == "" {
		variant = "Regular"
	}
	if w.PerVariantKey != nil {
		if v, ok := w.PerVariantKey[id+"|"+variant]; ok {
			return v
		}
	}
	if w.PerPetID != nil {
		if v, ok := w.PerPetID[id]; ok {
			return v
		}
	}
	return 1
}

func scoreCandidate(obj ObjectiveID, c RecipeCandidate, budget int) float64 {
	switch obj {
	case ObjMinButlers:
		if c.ExpectedButlersPerSuccess == nil {
			return math.Inf(1)
		}
		return *c.ExpectedButlersPerSuccess
	case ObjMinShards:
		if c.ExpectedShardsPerSuccess == nil {
			return math.Inf(1)
		}
		return *c.ExpectedShardsPerSuccess
	case ObjMaxSuccess:
		return -c.DisplayedWinProbability
	case ObjMaxSuccessBudget:
		if budget > 0 && c.Butlers > budget {
			return math.Inf(1)
		}
		return -c.DisplayedWinProbability
	case ObjWeightedCost:
		if c.WeightedCostPerSuccess == nil {
			return math.Inf(1)
		}
		return *c.WeightedCostPerSuccess
	default:
		if c.ExpectedButlersPerSuccess == nil {
			return math.Inf(1)
		}
		return *c.ExpectedButlersPerSuccess
	}
}

// dominates reports whether a is better-or-equal on both expected butlers and shards per success, and strictly better on one.
func dominates(a, b RecipeCandidate) bool {
	if a.ExpectedButlersPerSuccess == nil || b.ExpectedButlersPerSuccess == nil {
		return false
	}
	if a.ExpectedShardsPerSuccess == nil || b.ExpectedShardsPerSuccess == nil {
		return false
	}
	be := *a.ExpectedButlersPerSuccess <= *b.ExpectedButlersPerSuccess
	se := *a.ExpectedShardsPerSuccess <= *b.ExpectedShardsPerSuccess
	bs := *a.ExpectedButlersPerSuccess < *b.ExpectedButlersPerSuccess
	ss := *a.ExpectedShardsPerSuccess < *b.ExpectedShardsPerSuccess
	return be && se && (bs || ss)
}

// MixedRecipe is a historical/incomplete mixed observation placeholder for uncertainty-aware listing.
type MixedRecipe struct {
	Butlers            int                   `json:"butlers"`
	PorcelainDollCats  int                   `json:"porcelainDollCats,omitempty"`
	NeonSkeletonCats   int                   `json:"neonSkeletonCats,omitempty"`
	SpiritShards       *int                  `json:"spiritShards,omitempty"`
	ObservedRewardID   string                `json:"observedRewardId,omitempty"`
	DisplayedWin       *DisplayedProbability `json:"displayedWinProbability,omitempty"`
	Notes              string                `json:"notes,omitempty"`
	SuccessChanceKnown bool                  `json:"successChanceKnown"`
}

// ListMixedUncertainty returns uncertainty-aware candidates for incomplete mixed recipes (no fabricated p).
func ListMixedUncertainty(recipes []MixedRecipe) []RecipeCandidate {
	out := make([]RecipeCandidate, 0, len(recipes))
	for _, r := range recipes {
		c := RecipeCandidate{
			Butlers:           r.Butlers,
			ProbabilitySource: "unknown",
			Feasible:          true,
			Notes: []string{
				"mixed recipe incomplete coefficient data — success chance not fabricated",
				r.Notes,
			},
			ObjectiveScore: math.Inf(1),
		}
		if r.SpiritShards != nil {
			c.SpiritShards = *r.SpiritShards
		}
		if r.DisplayedWin != nil && r.DisplayedWin.Fraction != nil {
			c.DisplayedWinProbability = *r.DisplayedWin.Fraction
			c.DisplayedWinPercent = r.DisplayedWin.Raw
			c.ProbabilitySource = "observed"
			c.SuccessKnownFill()
		}
		if r.PorcelainDollCats > 0 || r.NeonSkeletonCats > 0 {
			c.Notes = append(c.Notes, fmt.Sprintf("porcelain=%d neon=%d", r.PorcelainDollCats, r.NeonSkeletonCats))
		}
		out = append(out, c)
	}
	return out
}

func (c *RecipeCandidate) SuccessKnownFill() {
	if c.DisplayedWinProbability > 0 {
		if eb, err := ExpectedButlersPerSuccess(c.Butlers, c.DisplayedWinProbability); err == nil {
			c.ExpectedButlersPerSuccess = &eb
			c.ObjectiveScore = eb
		}
		if c.SpiritShards > 0 {
			if es, err := ExpectedShardsPerSuccess(c.SpiritShards, c.DisplayedWinProbability); err == nil {
				c.ExpectedShardsPerSuccess = &es
			}
		}
	}
}

// VariantAllowed checks protection defaults.
func VariantAllowed(w ResourceWeights, variant string) bool {
	v := strings.ToLower(variant)
	switch {
	case strings.Contains(v, "shiny") && !w.AllowShiny:
		return false
	case strings.Contains(v, "rainbow") && !w.AllowRainbow:
		return false
	case strings.Contains(v, "golden") && !w.AllowGolden:
		return false
	default:
		return true
	}
}
