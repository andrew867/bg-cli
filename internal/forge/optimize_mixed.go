package forge

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

// ObservedMixedOptions configures Pareto search over measured mixed recipes only.
type ObservedMixedOptions struct {
	Objective        ObjectiveID
	Weights          ResourceWeights
	Inventory        InventoryAvailability
	ShardBudget      int // 0 = no filter
	MinButlers       int
	ProtectVariants  bool
	PorcelainPerUnit float64 // weighted cost per porcelain doll in recipe
}

// observationToCandidate converts one mixed observation when displayed probability is known.
func observationToMixedCandidate(o Observation, opt ObservedMixedOptions) (RecipeCandidate, bool) {
	if o.RecipeKind != "mixed" && o.RecipeKind != "butler-only" {
		// allow butler-only rows in same file but mixed mode prefers mixed
	}
	if o.DisplayedWinProbability.Fraction == nil || *o.DisplayedWinProbability.Fraction <= 0 {
		return RecipeCandidate{}, false
	}
	p := *o.DisplayedWinProbability.Fraction
	butlers := 0
	porcelain := 0
	neon := 0
	for _, pet := range o.SelectedPets {
		if pet.Quantity <= 0 {
			continue
		}
		switch {
		case strings.EqualFold(pet.ID, "Butler Penguin"):
			butlers += pet.Quantity
		case strings.EqualFold(pet.ID, "Porcelain Doll Cat"):
			porcelain += pet.Quantity
		case strings.EqualFold(pet.ID, "Neon Skeleton Cat"):
			neon += pet.Quantity
		}
	}
	shards := 0
	if o.SpiritShards != nil {
		shards = *o.SpiritShards
	}
	if opt.ShardBudget > 0 && shards > opt.ShardBudget {
		return RecipeCandidate{}, false
	}
	if opt.MinButlers > 0 && butlers < opt.MinButlers {
		return RecipeCandidate{}, false
	}
	c := RecipeCandidate{
		Butlers:                 butlers,
		SpiritShards:            shards,
		DisplayedWinProbability: p,
		DisplayedWinPercent:     o.DisplayedWinProbability.Raw,
		ProbabilitySource:       "observed",
		Feasible:                true,
		Notes: []string{
			"observed mixed recipe",
			fmt.Sprintf("comparableKey=%s", o.MixedDominanceContextKey()),
			fmt.Sprintf("porcelain=%d neon=%d", porcelain, neon),
		},
	}
	if eb, err := ExpectedButlersPerSuccess(butlers, p); err == nil {
		c.ExpectedButlersPerSuccess = &eb
	}
	if shards > 0 {
		if es, err := ExpectedShardsPerSuccess(shards, p); err == nil {
			c.ExpectedShardsPerSuccess = &es
		}
	}
	w := opt.Weights
	if w.PerPetID == nil {
		w = DefaultButlerWeights()
	}
	cost := float64(butlers)*weightFor(w, "Butler Penguin", "Regular") +
		float64(shards)*w.SpiritShard
	if opt.PorcelainPerUnit > 0 {
		cost += float64(porcelain) * opt.PorcelainPerUnit
	}
	c.WeightedCostPerAttempt = &cost
	if p > 0 {
		cs := cost / p
		c.WeightedCostPerSuccess = &cs
	}
	c.ObjectiveScore = scoreCandidate(opt.Objective, c, 0)
	// stash porcelain in notes for dominance (DominatedMixed uses observation id via notes key)
	c.Notes = append(c.Notes, fmt.Sprintf("porcelainDollCats=%d", porcelain))
	return c, true
}

// mixedResourceMeta is parsed from candidate notes for 3-axis dominance.
type mixedResourceMeta struct {
	comparableKey string
	porcelain     int
}

func mixedMeta(c RecipeCandidate) mixedResourceMeta {
	out := mixedResourceMeta{}
	for _, n := range c.Notes {
		if strings.HasPrefix(n, "comparableKey=") {
			out.comparableKey = strings.TrimPrefix(n, "comparableKey=")
		}
		if strings.HasPrefix(n, "porcelainDollCats=") {
			fmt.Sscanf(strings.TrimPrefix(n, "porcelainDollCats="), "%d", &out.porcelain)
		}
	}
	return out
}

// dominatesMixed decides Pareto dominance among observed recipes with the same comparable context.
func dominatesMixed(a, b RecipeCandidate) bool {
	ma, mb := mixedMeta(a), mixedMeta(b)
	if ma.comparableKey == "" || mb.comparableKey == "" || ma.comparableKey != mb.comparableKey {
		return false
	}
	if a.ExpectedButlersPerSuccess == nil || b.ExpectedButlersPerSuccess == nil ||
		a.ExpectedShardsPerSuccess == nil || b.ExpectedShardsPerSuccess == nil {
		return false
	}
	be := *a.ExpectedButlersPerSuccess <= *b.ExpectedButlersPerSuccess
	se := *a.ExpectedShardsPerSuccess <= *b.ExpectedShardsPerSuccess
	pe := ma.porcelain <= mb.porcelain
	bs := *a.ExpectedButlersPerSuccess < *b.ExpectedButlersPerSuccess
	ss := *a.ExpectedShardsPerSuccess < *b.ExpectedShardsPerSuccess
	ps := ma.porcelain < mb.porcelain
	return be && se && pe && (bs || ss || ps)
}

// OptimizeObservedMixed builds a Pareto front from mixed observations only (no modeled slope).
func OptimizeObservedMixed(file ObservationFile, opt ObservedMixedOptions) (OptimizeReport, error) {
	rep := OptimizeReport{
		OK:        true,
		Objective: opt.Objective,
		MethodID:  "forge-optimize-observed-mixed.v1",
		Assumptions: []string{
			"success probabilities taken only from observed UI previews",
			"no extrapolation from butler-only capped-linear fit",
			"dominance requires matching comparableKey (luck, shards, pet lines)",
			"porcelain doll counts participate in mixed Pareto dominance",
		},
		Warnings: []string{},
	}
	if opt.Objective == "" {
		opt.Objective = ObjMinShards
	}
	var cands []RecipeCandidate
	seen := map[string]struct{}{}
	for _, o := range file.Observations {
		if o.RecipeKind != "mixed" {
			continue
		}
		c, ok := observationToMixedCandidate(o, opt)
		if !ok {
			continue
		}
		key := o.ComparableKey() + "|" + fmt.Sprintf("b=%d s=%d p=%.6f", c.Butlers, c.SpiritShards, c.DisplayedWinProbability)
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		cands = append(cands, c)
	}
	if len(cands) == 0 {
		rep.OK = false
		rep.Warnings = append(rep.Warnings, "no feasible observed mixed recipes")
		return rep, nil
	}
	for i := range cands {
		for j := range cands {
			if i == j || !cands[j].Feasible {
				continue
			}
			if dominatesMixed(cands[j], cands[i]) {
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
	if len(rep.ParetoFront) == 0 {
		rep.Warnings = append(rep.Warnings, "no non-dominated recipes under current filters")
	}
	return rep, nil
}

// ListIncomparableMixed returns pairs that share success rate but mismatch comparable context.
func ListIncomparableMixed(a, b Observation) bool {
	if a.DisplayedWinProbability.Fraction == nil || b.DisplayedWinProbability.Fraction == nil {
		return true
	}
	if math.Abs(*a.DisplayedWinProbability.Fraction-*b.DisplayedWinProbability.Fraction) > 1e-9 {
		return false
	}
	// Luck / event / pet-line context can differ while UI shows the same rounded percent.
	return a.MixedDominanceContextKey() != b.MixedDominanceContextKey()
}
