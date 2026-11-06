package loadout

import (
	"fmt"
	"strings"
)

// Provenance tracks how a derived quantity was obtained.
type Provenance struct {
	ModelID    string     `json:"modelId"`
	Source     string     `json:"source"`
	Confidence Confidence `json:"confidence"`
	Notes      string     `json:"notes,omitempty"`
}

// Quantity is a named numeric or unknown layer.
type Quantity struct {
	Name       string     `json:"name"`
	Value      *float64   `json:"value,omitempty"`
	Text       string     `json:"text,omitempty"`
	Unit       string     `json:"unit,omitempty"`
	Unknown    bool       `json:"unknown"`
	Provenance Provenance `json:"provenance"`
}

// PetPowerBreakdown is per-pet layered analysis for one world context.
type PetPowerBreakdown struct {
	PetID            string                     `json:"petId"`
	UID              string                     `json:"uid,omitempty"`
	WorldID          string                     `json:"worldId"`
	PublishedBase    Quantity                   `json:"publishedBasePower"`
	VariantAdjusted  Quantity                   `json:"variantAdjustedPower"`
	HugeScaling      Quantity                   `json:"hugeScaling"`
	LevelBonus       Quantity                   `json:"levelXpBonus"`
	CharmBonus       Quantity                   `json:"charmBonus"`
	EnchantBonus     Quantity                   `json:"enchantBonus"`
	PotionBonus      Quantity                   `json:"potionBonus"`
	EventBonus       Quantity                   `json:"eventSpecificBonus"`
	ModeledHitDamage Quantity                   `json:"modeledEffectiveHitDamage"`
	Empirical        Quantity                   `json:"empiricallyMeasured"`
	HalloweenModels  []HalloweenModelPrediction `json:"halloweenModels,omitempty"`
	Missing          []string                   `json:"missingData"`
	Warnings         []string                   `json:"warnings"`
}

// PowerInspectReport is the account power inspect payload.
type PowerInspectReport struct {
	OK               bool                           `json:"ok"`
	WorldID          string                         `json:"worldId"`
	ModelSuiteID     string                         `json:"modelSuiteId"`
	PublicConfigMeta map[string]CollectionCacheMeta `json:"publicConfigMeta,omitempty"`
	Pets             []PetPowerBreakdown            `json:"pets"`
	CurrenciesNote   string                         `json:"currenciesNote"`
	Assumptions      []string                       `json:"assumptions"`
	Warnings         []string                       `json:"warnings"`
	EquipBest        *EquipBestComparison           `json:"equipBestComparison,omitempty"`
}

// InspectOptions configures power inspection.
type InspectOptions struct {
	WorldID        string
	Annotations    AnnotationFile
	Public         PublicConfigIndex
	BestEventRel   float64 // relative scale for best event pet; 1.0 default when unknown units
	EventTeamLimit *int
}

// InspectPower builds layered per-pet reports without inventing missing multipliers.
func InspectPower(pl PlayerLoadout, opt InspectOptions) PowerInspectReport {
	world := opt.WorldID
	if world == "" {
		world = "general"
	}
	rep := PowerInspectReport{
		OK:               true,
		WorldID:          world,
		ModelSuiteID:     "pet-power-layers.v1",
		PublicConfigMeta: opt.Public.Meta,
		CurrenciesNote:   "Currency economies are never combined into this damage report; use loadout objectives per currency ID.",
		Assumptions: []string{
			"cachedPower is published catalog power, not proven post-boost DPS",
			"indexDesc Halloween extras are presentation claims unless independently verified",
			"unknown multipliers are marked unknown rather than defaulted to 1 for rankings",
			"Huge XP curve " + HugeXPCurveCommunityV1 + " is community/fixture-based, not official",
		},
		Warnings: append([]string{}, pl.Warnings...),
	}
	if strings.EqualFold(world, "halloween-games") || strings.EqualFold(world, "HalloweenGames") {
		rep.Warnings = append(rep.Warnings, "Halloween Games is not present in /api/collection/Worlds; world rules are independently versioned here")
	}

	bestRel := opt.BestEventRel
	if bestRel <= 0 {
		bestRel = 1
	}

	for _, slot := range pl.Pets.List {
		rep.Pets = append(rep.Pets, breakdownPet(slot, pl, opt, world, bestRel))
	}
	if len(pl.Pets.List) == 0 {
		rep.Warnings = append(rep.Warnings, "no equipped pets in inventory payload")
	}

	if len(opt.Annotations.EquipBestOrder) > 0 {
		cmp := CompareEquipBest(opt.Annotations.EquipBestOrder, pl, rep.Pets, world)
		rep.EquipBest = &cmp
	}
	return rep
}

func breakdownPet(slot EquippedSlot, pl PlayerLoadout, opt InspectOptions, world string, bestRel float64) PetPowerBreakdown {
	b := PetPowerBreakdown{
		PetID: slot.ID, UID: slot.UID, WorldID: world,
		Missing: []string{}, Warnings: []string{},
	}
	if slot.Slot != "" {
		b.Warnings = append(b.Warnings, "equippedSlot="+slot.Slot)
	}
	if slot.Quantity > 1 {
		b.Warnings = append(b.Warnings, fmt.Sprintf("equippedQuantity=%d (list entry not collapsed by uid)", slot.Quantity))
	}
	if slot.UID != "" {
		b.Warnings = append(b.Warnings, "uidRole=stack-identity (not unique instance)")
	}
	ann, annOK := opt.Annotations.MatchAnnotation(slot)
	if !annOK && ann.Notes != "" {
		b.Warnings = append(b.Warnings, ann.Notes)
		b.Missing = append(b.Missing, "ambiguous pet annotation; prefer slot/instanceKey over shared uid")
	}

	// 1. Published base
	if pc, ok := opt.Public.LookupPet(slot.ID); ok && len(pc.CachedPower) > 0 {
		if f, err := pc.CachedPower[0].Float64(); err == nil {
			b.PublishedBase = qty("publishedBasePower", &f, "catalog_power", Provenance{
				ModelID: "pets.cachedPower[0]", Source: "public /api/collection/Pets", Confidence: ConfidenceVerified,
				Notes: "hashShort=" + pc.HashShort + " dateModified=" + pc.DateModified,
			})
		}
	} else if slot.DisplayedPower != nil {
		// inventory display only
		b.PublishedBase = Quantity{Name: "publishedBasePower", Text: *slot.DisplayedPower, Unit: "displayed_power", Unknown: false,
			Provenance: Provenance{ModelID: "inventory.displayedPower", Source: "account inventory field", Confidence: ConfidenceUnknown,
				Notes: "not proven equal to catalog cachedPower or post-boost damage"}}
	} else {
		b.PublishedBase = unknownQty("publishedBasePower", "catalog/inventory base power unavailable")
		b.Missing = append(b.Missing, "publishedBasePower")
	}

	// 2. Variant — unknown factor unless documented
	b.VariantAdjusted = unknownQty("variantAdjustedPower", "variant multipliers not verified from public config; not defaulted to 1")
	if slot.Variant != "" && slot.Variant != "Regular" {
		b.Warnings = append(b.Warnings, "variant="+slot.Variant+" present but multiplier unknown")
		b.Missing = append(b.Missing, "variantMultiplier")
	}

	// 3. Huge scaling
	if slot.IsHugeCandidate {
		b.HugeScaling = unknownQty("hugeScaling", "normal-world Huge scaling not published as a numeric formula")
	} else {
		b.HugeScaling = qty("hugeScaling", nil, "", Provenance{ModelID: "n/a", Source: "not a Huge candidate", Confidence: ConfidenceVerified, Notes: "layer inapplicable"})
		b.HugeScaling.Unknown = false
		b.HugeScaling.Text = "inapplicable"
	}

	// 4. Level/XP
	if annOK && ann.Level != nil {
		var bonus float64
		var conf Confidence
		var note, curve string
		if ann.DisplayedLevelDamageBonusPercent != nil {
			bonus = *ann.DisplayedLevelDamageBonusPercent
			conf = ConfidenceMeasured
			curve = "annotation.displayedLevelDamageBonusPercent"
			note = "from manual tooltip annotation"
		} else {
			bonus, curve, conf, note = DisplayedHugeXPBonusPercent(*ann.Level)
		}
		b.LevelBonus = qty("levelXpBonus", &bonus, "percent", Provenance{ModelID: curve, Source: ann.Source, Confidence: conf, Notes: note})
	} else {
		b.LevelBonus = unknownQty("levelXpBonus", "Huge level not present in account inventory API; supply annotations")
		b.Missing = append(b.Missing, "level")
	}

	// 5. Charms
	b.CharmBonus, b.Missing = charmLayer(ann, annOK, opt.Public, b.Missing)

	// 6–7. Enchants / potions on the pet — account API lacks per-pet enchants; account-wide enchants are separate
	b.EnchantBonus = unknownQty("enchantBonus", "per-pet enchant contribution not separated from account-wide enchant loadout in inventory API")
	b.PotionBonus = unknownQty("potionBonus", "active potions not exposed as structured inventory fields")

	// 8. Event-specific
	if strings.Contains(strings.ToLower(world), "halloween") {
		if pc, ok := opt.Public.LookupPet(slot.ID); ok && pc.HalloweenExtraDamagePresentation != nil {
			v := float64(*pc.HalloweenExtraDamagePresentation)
			b.EventBonus = qty("eventSpecificBonus", &v, "presentation_extra_damage", Provenance{
				ModelID: "pets.indexDesc.halloweenExtra", Source: "public indexDesc (presentation)", Confidence: ConfidenceAssumed,
				Notes: "NOT verified as server damage formula; do not treat as parsed engine input",
			})
		} else if slot.IsHugeCandidate {
			b.EventBonus = unknownQty("eventSpecificBonus", "Halloween Huge 31% rule is tooltip-observed; see halloweenModels")
		} else {
			b.EventBonus = unknownQty("eventSpecificBonus", "no structured event bonus field for this pet")
		}
		// Competing models for Huges
		if slot.IsHugeCandidate {
			levelBonus := 0.0
			if b.LevelBonus.Value != nil {
				levelBonus = *b.LevelBonus.Value
			}
			charmFactor, charmKnown := charmDamageProduct(ann, annOK, opt.Public)
			in := HalloweenModelInput{
				BestEventStatRelative: bestRel,
				LevelBonusPercent:     levelBonus,
				VariantFactor:         1,
				VariantFactorKnown:    slot.Variant == "" || slot.Variant == "Regular",
				CharmDamageFactor:     charmFactor,
				CharmFactorKnown:      charmKnown,
				ApplyLevel:            true,
				ApplyVariant:          false,
				ApplyCharms:           true,
			}
			if !annOK || ann.Level == nil {
				in.CharmFactorKnown = charmKnown
			}
			b.HalloweenModels = PredictHalloweenModels(in)
			// Also attach documented Model B no-charm hypotheticals for the three screenshot pets
			if ex, ok := ExampleModelBNoCharm()[slot.ID]; ok && (ann.DisplayedLevelDamageBonusPercent != nil || (ann.Level != nil)) {
				b.Warnings = append(b.Warnings, fmt.Sprintf("Model B no-charm hypothetical relative index for annotated level curve product=%.5f (prediction only)", ex))
			}
		}
	} else {
		b.EventBonus = qty("eventSpecificBonus", nil, "", Provenance{ModelID: "n/a", Source: "non-event world", Confidence: ConfidenceVerified, Notes: "layer inapplicable"})
		b.EventBonus.Text = "inapplicable"
		b.EventBonus.Unknown = false
	}

	// 9. Modeled effective hit damage — only if all critical layers resolved (they are not)
	b.ModeledHitDamage = unknownQty("modeledEffectiveHitDamage", "withheld: one or more required layers unknown; refusing silent ×1 defaults")
	// 10. Empirical
	b.Empirical = unknownQty("empiricallyMeasured", "no empirical measurement attached; use account experiment compare")

	_ = pl
	return b
}

func charmLayer(ann PetAnnotation, annOK bool, pub PublicConfigIndex, missing []string) (Quantity, []string) {
	if !annOK || len(ann.Charms) == 0 {
		return unknownQty("charmBonus", "charm list absent from account inventory; supply annotations"), append(missing, "charms")
	}
	var parts []string
	unresolved := false
	for _, name := range ann.Charms {
		cc, ok := pub.LookupCharm(name)
		if !ok {
			parts = append(parts, name+":unknown_not_in_public_charms")
			unresolved = true
			continue
		}
		switch cc.Family {
		case "damage_percent", "mixed":
			if cc.DamagePercentClaim != nil {
				parts = append(parts, fmt.Sprintf("%s:damage_claim=%.1f%%(family=%s)", name, *cc.DamagePercentClaim, cc.Family))
			} else {
				parts = append(parts, name+":damage_claim_unresolved")
				unresolved = true
			}
		case "triggered":
			parts = append(parts, name+":triggered(not_constant_multiplier)")
			unresolved = true
		case "reward_proc", "movement", "slots", "currency_coins", "currency_diamonds":
			parts = append(parts, name+":family="+cc.Family+"(not_raw_damage)")
		default:
			parts = append(parts, name+":family=unknown")
			unresolved = true
		}
	}
	q := Quantity{Name: "charmBonus", Text: strings.Join(parts, "; "), Unknown: unresolved,
		Provenance: Provenance{ModelID: "charms.desc-classification.v1", Source: "annotations+public Charms", Confidence: ConfidenceAssumed,
			Notes: "Bonus/Lightning/etc. are not auto-converted into constant damage multipliers"}}
	return q, missing
}

func charmDamageProduct(ann PetAnnotation, annOK bool, pub PublicConfigIndex) (float64, bool) {
	if !annOK {
		return 1, false
	}
	f := 1.0
	known := true
	saw := false
	for _, name := range ann.Charms {
		cc, ok := pub.LookupCharm(name)
		if !ok {
			return 1, false
		}
		switch cc.Family {
		case "damage_percent", "mixed":
			if cc.DamagePercentClaim == nil {
				return 1, false
			}
			f *= 1 + (*cc.DamagePercentClaim)/100
			saw = true
		case "triggered":
			return 1, false // cannot fold into constant product
		default:
			// non-damage charms ignored for damage product
		}
	}
	if !saw {
		return 1, true // explicitly no damage charms
	}
	return f, known
}

func qty(name string, v *float64, unit string, p Provenance) Quantity {
	return Quantity{Name: name, Value: v, Unit: unit, Unknown: false, Provenance: p}
}

func unknownQty(name, note string) Quantity {
	return Quantity{Name: name, Unknown: true, Provenance: Provenance{ModelID: "unresolved", Source: "insufficient data", Confidence: ConfidenceUnknown, Notes: note}}
}
