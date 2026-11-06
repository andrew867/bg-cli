package loadout

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
)

// Confidence describes how well an effect magnitude is known.
type Confidence string

const (
	ConfidenceVerified Confidence = "verified" // from authoritative BIG Games / public config
	ConfidenceMeasured Confidence = "measured" // calibrated from controlled snapshot intervals
	ConfidenceAssumed  Confidence = "assumed"  // provisional; must be labeled in output
	ConfidenceUnknown  Confidence = "unknown"  // present as identifier only; no magnitude
)

// EffectCategory groups mechanical roles.
type EffectCategory string

const (
	CategoryPetDamage     EffectCategory = "pet_damage"
	CategoryHugeScaling   EffectCategory = "huge_scaling"
	CategoryCurrencyGain  EffectCategory = "currency_gain"
	CategoryBreakables    EffectCategory = "breakables"
	CategoryItemDrops     EffectCategory = "item_drops"
	CategoryHatch         EffectCategory = "hatch"
	CategoryEventActivity EffectCategory = "event_activity"
	CategorySlotLimit     EffectCategory = "slot_limit"
	CategoryOther         EffectCategory = "other"
)

// StackingRule documents how multiple instances combine.
type StackingRule string

const (
	StackAdditive       StackingRule = "additive"
	StackMultiplicative StackingRule = "multiplicative"
	StackMax            StackingRule = "max"
	StackUnknown        StackingRule = "unknown"
)

// EffectEntry is one versioned registry row.
type EffectEntry struct {
	ID                   string         `json:"id"`
	Aliases              []string       `json:"aliases,omitempty"`
	Category             EffectCategory `json:"category"`
	Magnitude            *float64       `json:"magnitude,omitempty"` // nil when unknown
	MagnitudeUnit        string         `json:"magnitudeUnit,omitempty"`
	ApplicableWorlds     []string       `json:"applicableWorlds"` // empty = unspecified (not "all")
	ApplicableCurrencies []string       `json:"applicableCurrencies,omitempty"`
	Stacking             StackingRule   `json:"stacking"`
	Cap                  *float64       `json:"cap,omitempty"`
	DiminishingReturns   string         `json:"diminishingReturns,omitempty"`
	Source               string         `json:"source"`
	Confidence           Confidence     `json:"confidence"`
	Notes                string         `json:"notes,omitempty"`
}

// WorldProfile describes pet equip limits and event context.
type WorldProfile struct {
	ID                string     `json:"id"`
	DisplayName       string     `json:"displayName"`
	MaxEquippedPets   *int       `json:"maxEquippedPets,omitempty"` // nil = unknown; never invent
	MaxEquippedSource string     `json:"maxEquippedPetsSource,omitempty"`
	MaxEquippedConf   Confidence `json:"maxEquippedPetsConfidence"`
	Notes             string     `json:"notes,omitempty"`
}

// Registry is a versioned effect/world catalogue.
type Registry struct {
	SchemaVersion string         `json:"schemaVersion"`
	Version       string         `json:"version"`
	Effects       []EffectEntry  `json:"effects"`
	Worlds        []WorldProfile `json:"worlds"`
	Notes         []string       `json:"notes,omitempty"`
}

const RegistrySchemaVersion = "1.0"

// DefaultRegistry returns the built-in conservative registry.
// Magnitudes that are not verified remain nil / unknown.
func DefaultRegistry() Registry {
	return Registry{
		SchemaVersion: RegistrySchemaVersion,
		Version:       "2026-11-06.1",
		Notes: []string{
			"Displayed pet power is never treated as post-boost damage.",
			"Currency channels are independent; never combine into one power score.",
			"Unknown multipliers stay unknown until verified or calibrated.",
			"General maxEquipped from inventory must not be assumed for special events.",
		},
		Worlds: []WorldProfile{
			{
				ID: "general", DisplayName: "General worlds",
				MaxEquippedConf: ConfidenceUnknown,
				Notes:           "Prefer inventory equipped.pets.maxEquipped for the general/default context when present; do not invent a number here.",
			},
			{
				ID: "halloween-games", DisplayName: "Halloween Games event",
				MaxEquippedPets: nil, MaxEquippedConf: ConfidenceUnknown,
				MaxEquippedSource: "unverified",
				Notes:             "Event pet slot limits may differ from general maxEquipped; do not assume the inventory general limit applies.",
			},
		},
		Effects: []EffectEntry{
			{
				ID: "displayed-pet-power", Category: CategoryPetDamage,
				Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{}, Confidence: ConfidenceUnknown,
				Source: "inventory item fields (power/damage display)",
				Notes:  "Displayed power is a UI/base indicator only; not actual post-boost damage.",
			},
			{
				ID: "huge-pet-scaling", Category: CategoryHugeScaling,
				Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{}, Confidence: ConfidenceUnknown,
				Source: "unverified",
				Notes:  "Huge scaling remains unknown until verified against authoritative config or controlled calibration.",
			},
			{
				ID: "halloween-games-pet-bonus", Category: CategoryPetDamage,
				Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{"halloween-games"}, Confidence: ConfidenceUnknown,
				Source: "unverified event bonus",
				Notes:  "Halloween Games pet bonuses are not invented; mark unresolved until verified.",
			},
			{
				ID: "coins-currency-gain", Category: CategoryCurrencyGain,
				ApplicableCurrencies: []string{"Coins"}, Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{}, Confidence: ConfidenceUnknown, Source: "unverified",
			},
			{
				ID: "diamonds-currency-gain", Category: CategoryCurrencyGain,
				ApplicableCurrencies: []string{"Diamonds"}, Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{}, Confidence: ConfidenceUnknown, Source: "unverified",
			},
			{
				ID: "halloween-games-coins-gain", Category: CategoryCurrencyGain,
				ApplicableCurrencies: []string{"HalloweenGamesCoins"}, Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{"halloween-games"}, Confidence: ConfidenceUnknown, Source: "unverified",
			},
			{
				ID: "halloween-games-treats-gain", Category: CategoryEventActivity,
				ApplicableCurrencies: []string{"HalloweenGamesTreats"}, Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{"halloween-games"}, Confidence: ConfidenceUnknown, Source: "unverified",
			},
			{
				ID: "trail-coins-gain", Category: CategoryCurrencyGain,
				ApplicableCurrencies: []string{"TrailCoins"}, Magnitude: nil, Stacking: StackUnknown,
				ApplicableWorlds: []string{}, Confidence: ConfidenceUnknown, Source: "unverified",
			},
		},
	}
}

// LoadRegistryJSON loads a registry file; empty path returns DefaultRegistry.
func LoadRegistryJSON(path string) (Registry, error) {
	if strings.TrimSpace(path) == "" {
		return DefaultRegistry(), nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return Registry{}, err
	}
	var r Registry
	if err := json.Unmarshal(b, &r); err != nil {
		return Registry{}, fmt.Errorf("parse registry: %w", err)
	}
	if r.SchemaVersion == "" {
		return Registry{}, fmt.Errorf("registry missing schemaVersion")
	}
	return r, nil
}

// LookupEffect finds by id or alias (case-insensitive).
func (r Registry) LookupEffect(id string) (EffectEntry, bool) {
	for _, e := range r.Effects {
		if strings.EqualFold(e.ID, id) {
			return e, true
		}
		for _, a := range e.Aliases {
			if strings.EqualFold(a, id) {
				return e, true
			}
		}
	}
	return EffectEntry{}, false
}

func (r Registry) LookupWorld(id string) (WorldProfile, bool) {
	for _, w := range r.Worlds {
		if strings.EqualFold(w.ID, id) {
			return w, true
		}
	}
	return WorldProfile{}, false
}

// EffectsForCurrency returns registry rows that list currencyID.
func (r Registry) EffectsForCurrency(currencyID string) []EffectEntry {
	var out []EffectEntry
	for _, e := range r.Effects {
		for _, c := range e.ApplicableCurrencies {
			if strings.EqualFold(c, currencyID) {
				out = append(out, e)
				break
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}
