package loadout

import (
	"fmt"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

// Substitution explains one recommended change.
type Substitution struct {
	SlotKind   string     `json:"slotKind"` // pet | enchant
	RemoveID   string     `json:"removeId,omitempty"`
	AddID      string     `json:"addId"`
	Reason     string     `json:"reason"`
	OwnedCount string     `json:"ownedCount,omitempty"`
	Confidence Confidence `json:"confidence"`
}

// CandidateLoadout is a proposed equip set with uncertainty.
type CandidateLoadout struct {
	Rank                  int            `json:"rank"`
	Pets                  []string       `json:"pets"`
	Enchants              []string       `json:"enchants"`
	Substitutions         []Substitution `json:"substitutions"`
	ExpectedMarginalGain  *float64       `json:"expectedMarginalGain,omitempty"` // only when justified
	MarginalGainUnit      string         `json:"marginalGainUnit,omitempty"`
	MarginalGainJustified bool           `json:"marginalGainJustified"`
	Uncertainty           []string       `json:"uncertainty"`
	SuggestedTest         string         `json:"suggestedControlledTest,omitempty"`
}

// OptimizeResult is the full optimizer report.
type OptimizeResult struct {
	OK                 bool               `json:"ok"`
	Objective          Objective          `json:"objective"`
	WorldID            string             `json:"worldId"`
	RegistryVersion    string             `json:"registryVersion"`
	Current            PlayerLoadout      `json:"currentLoadout"`
	CurrentEstimate    ChannelEstimate    `json:"currentEstimate"`
	PetSlotLimit       *int               `json:"petSlotLimit,omitempty"`
	PetSlotLimitSource string             `json:"petSlotLimitSource,omitempty"`
	EnchantSlotLimit   *int               `json:"enchantSlotLimit,omitempty"`
	Candidates         []CandidateLoadout `json:"candidates"`
	Assumptions        []string           `json:"assumptions"`
	Warnings           []string           `json:"warnings"`
	MethodID           string             `json:"methodId"`
}

// OptimizeOptions configures candidate search.
type OptimizeOptions struct {
	WorldID       string
	Objective     Objective
	CurrencyID    string
	MaxCandidates int
}

// Optimize produces candidate loadouts constrained by owned quantities and slot limits.
// It never invents effective DPS or final currency multipliers.
func Optimize(reg Registry, pl PlayerLoadout, opt OptimizeOptions) (OptimizeResult, error) {
	if opt.MaxCandidates <= 0 {
		opt.MaxCandidates = 5
	}
	est, err := AnalyzeLoadout(reg, pl, AnalyzeOptions{
		WorldID: opt.WorldID, Objective: opt.Objective, CurrencyID: opt.CurrencyID,
	})
	if err != nil {
		return OptimizeResult{}, err
	}
	res := OptimizeResult{
		OK:              true,
		Objective:       opt.Objective,
		WorldID:         est.WorldID,
		RegistryVersion: reg.Version,
		Current:         pl,
		CurrentEstimate: est,
		Assumptions:     append([]string{}, est.Assumptions...),
		Warnings:        append([]string{}, est.Warnings...),
		MethodID:        "loadout-optimize.v1-conservative",
	}
	res.Assumptions = append(res.Assumptions,
		"Only owned enchant/pet stacks may be recommended.",
		"Enchant recommendations never exceed equipped.enchants.maxEnchants (official field; legacy maxEquipped accepted as fallback).",
		"Pet slot limits prefer world registry; inventory general maxEquipped is used only for world=general.",
		"Equipped pet uid is treated as stack-identity; quantities are preserved per list/slot entry and never deduplicated by uid.",
	)

	res.PetSlotLimit, res.PetSlotLimitSource = resolvePetSlots(reg, pl, est.WorldID, &res.Warnings)
	if pl.Enchants.MaxEquipped != nil {
		res.EnchantSlotLimit = pl.Enchants.MaxEquipped
	}

	// Baseline candidate: current loadout.
	base := CandidateLoadout{
		Rank:                  1,
		Pets:                  idsOf(pl.Pets.List),
		Enchants:              idsOf(pl.Enchants.List),
		Uncertainty:           append([]string{}, est.UnresolvedIDs...),
		MarginalGainJustified: false,
		SuggestedTest:         suggestedTest(opt.Objective, est.WorldID),
	}
	if len(est.UnresolvedIDs) > 0 {
		base.Uncertainty = append(base.Uncertainty, "current estimate is not fully resolved; no numeric marginal gain claimed")
	}
	res.Candidates = append(res.Candidates, base)

	// Propose enchant substitutions from owned books that appear relevant for the objective.
	ownedEnchantIDs := uniqueOwned(pl.OwnedEnchants)
	equippedSet := map[string]struct{}{}
	for _, e := range pl.Enchants.List {
		equippedSet[strings.ToLower(e.ID)] = struct{}{}
	}
	slotLimit := 5
	if res.EnchantSlotLimit != nil {
		slotLimit = *res.EnchantSlotLimit
	}

	type scored struct {
		id     string
		reason string
		conf   Confidence
	}
	var proposals []scored
	for _, id := range ownedEnchantIDs {
		if _, on := equippedSet[strings.ToLower(id)]; on {
			continue
		}
		cnt, _ := pl.OwnedCount("Enchant", id)
		if !cnt.ParseOK || cnt.Int == nil || cnt.Int.Sign() <= 0 {
			continue
		}
		e, ok := reg.LookupEffect(id)
		reason := "owned enchant available; effect magnitude unknown — candidate only"
		conf := ConfidenceUnknown
		relevant := false
		if ok {
			conf = e.Confidence
			switch opt.Objective.Category {
			case CategoryPetDamage, CategoryHugeScaling:
				relevant = e.Category == CategoryPetDamage || e.Category == CategoryHugeScaling
			case CategoryCurrencyGain, CategoryEventActivity:
				cur := opt.CurrencyID
				if cur == "" {
					cur = opt.Objective.CurrencyID
				}
				for _, c := range e.ApplicableCurrencies {
					if strings.EqualFold(c, cur) {
						relevant = true
						break
					}
				}
			default:
				relevant = e.Category == opt.Objective.Category
			}
			if e.Magnitude == nil {
				reason = fmt.Sprintf("owned %q is catalogued but magnitude is %s; included as uncertain candidate", id, e.Confidence)
			} else {
				reason = fmt.Sprintf("owned %q has confidence=%s magnitude; still verify in target world before treating as gain", id, e.Confidence)
			}
		} else {
			reason = fmt.Sprintf("owned enchant %q not in effect registry; cannot assert applicability", id)
		}
		if !relevant && ok {
			continue
		}
		if !ok {
			// Still surface as uncertain owned option when slots free.
			reason = fmt.Sprintf("owned enchant %q not in registry; candidate requires verification", id)
		}
		proposals = append(proposals, scored{id: id, reason: reason, conf: conf})
	}
	sort.Slice(proposals, func(i, j int) bool { return proposals[i].id < proposals[j].id })

	rank := 2
	for _, p := range proposals {
		if len(res.Candidates) >= opt.MaxCandidates {
			break
		}
		candPets := idsOf(pl.Pets.List)
		candEnc := idsOf(pl.Enchants.List)
		var subs []Substitution
		cnt, _ := pl.OwnedCount("Enchant", p.id)
		if len(candEnc) >= slotLimit {
			// Must replace something — pick first equipped as placeholder removal with clear reason.
			if len(candEnc) == 0 {
				continue
			}
			remove := candEnc[0]
			candEnc = append([]string{p.id}, candEnc[1:]...)
			subs = append(subs, Substitution{
				SlotKind: "enchant", RemoveID: remove, AddID: p.id,
				Reason:     p.reason + fmt.Sprintf("; replace %q to respect maxEquipped=%d", remove, slotLimit),
				OwnedCount: cnt.String(), Confidence: p.conf,
			})
		} else {
			candEnc = append(candEnc, p.id)
			subs = append(subs, Substitution{
				SlotKind: "enchant", AddID: p.id,
				Reason:     p.reason + "; free enchant slot available",
				OwnedCount: cnt.String(), Confidence: p.conf,
			})
		}
		c := CandidateLoadout{
			Rank:                  rank,
			Pets:                  candPets,
			Enchants:              candEnc,
			Substitutions:         subs,
			MarginalGainJustified: false,
			ExpectedMarginalGain:  nil,
			Uncertainty: []string{
				"expected marginal improvement not computed: missing verified multipliers for this objective/world",
			},
			SuggestedTest: suggestedTest(opt.Objective, est.WorldID),
		}
		if p.conf == ConfidenceVerified || p.conf == ConfidenceMeasured {
			// Still no numeric gain without a complete chain.
			c.Uncertainty = append(c.Uncertainty, "registry confidence is high for the substitution item, but the full channel score remains unresolved")
		}
		res.Candidates = append(res.Candidates, c)
		rank++
	}

	if len(res.Candidates) == 1 {
		res.Warnings = append(res.Warnings, "no alternative owned enchants matched the objective filters; current loadout is the only candidate")
	}

	// Pet slot over-equip check
	if res.PetSlotLimit != nil && len(pl.Pets.List) > *res.PetSlotLimit {
		res.Warnings = append(res.Warnings, fmt.Sprintf("current equipped pets (%d) exceed resolved world slot limit (%d)", len(pl.Pets.List), *res.PetSlotLimit))
	}
	_ = snapshot.ExactInt{}
	return res, nil
}

func resolvePetSlots(reg Registry, pl PlayerLoadout, worldID string, warnings *[]string) (*int, string) {
	w, ok := reg.LookupWorld(worldID)
	if ok && w.MaxEquippedPets != nil && (w.MaxEquippedConf == ConfidenceVerified || w.MaxEquippedConf == ConfidenceMeasured) {
		return w.MaxEquippedPets, w.MaxEquippedSource
	}
	if strings.EqualFold(worldID, "general") && pl.Pets.MaxEquipped != nil {
		return pl.Pets.MaxEquipped, pl.Pets.MaxSource
	}
	if ok && w.MaxEquippedPets != nil && w.MaxEquippedConf == ConfidenceAssumed {
		*warnings = append(*warnings, "using assumed world pet slot limit; treat as provisional")
		return w.MaxEquippedPets, w.MaxEquippedSource + " (assumed)"
	}
	*warnings = append(*warnings, fmt.Sprintf("pet slot limit for world %q unknown; inventory general maxEquipped was not applied", worldID))
	return nil, "unknown"
}

// idsOf expands equipped entries by quantity (no UID dedupe). Five Butler Penguins
// sharing one uid still yield five id entries (or a single id repeated by quantity).
func idsOf(slots []EquippedSlot) []string {
	out := make([]string, 0, len(slots))
	for _, s := range slots {
		q := s.Quantity
		if q < 1 {
			q = 1
		}
		for i := 0; i < q; i++ {
			out = append(out, s.ID)
		}
	}
	return out
}

func uniqueOwned(stacks []OwnedStack) []string {
	seen := map[string]string{}
	for _, s := range stacks {
		k := strings.ToLower(s.ID)
		if _, ok := seen[k]; !ok {
			seen[k] = s.ID
		}
	}
	out := make([]string, 0, len(seen))
	for _, v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func suggestedTest(obj Objective, worldID string) string {
	cur := obj.CurrencyID
	if cur == "" {
		cur = "(non-currency objective)"
	}
	return fmt.Sprintf(
		"Controlled test: take account snapshot A, run a fixed-duration session in world %q with only the listed loadout change, take snapshot B, then run `account snapshot diff --from A --to B` focusing on %s; keep enchant/pet slots otherwise constant and record confounders (server events, potions, co-op).",
		worldID, cur,
	)
}
