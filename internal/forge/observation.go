package forge

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"
)

const ObservationSchemaVersion = "1.0"

// PetInput is one selected pet type/variant/quantity in a Forge recipe preview or attempt.
type PetInput struct {
	ID       string `json:"id"`
	Variant  string `json:"variant,omitempty"` // Regular|Golden|Rainbow|Shiny|...
	Quantity int    `json:"quantity"`
}

// RewardProbability is one displayed loot row.
type RewardProbability struct {
	RewardID         string               `json:"rewardId"`
	Displayed        DisplayedProbability `json:"displayed"`
	Available        *bool                `json:"available,omitempty"` // false when SOLD OUT
	AvailabilityNote string               `json:"availabilityNote,omitempty"`
	Provenance       string               `json:"provenance,omitempty"`
	Confidence       string               `json:"confidence,omitempty"` // verified|measured|assumed|unknown
}

// Observation is one Forge UI preview and/or executed attempt record.
type Observation struct {
	SchemaVersion           string               `json:"schemaVersion"`
	ObservationID           string               `json:"observationId,omitempty"`
	ObservedAt              string               `json:"observedAt"` // RFC3339 with offset preferred
	Timezone                string               `json:"timezone,omitempty"`
	Game                    string               `json:"game,omitempty"`
	EventID                 string               `json:"eventId"`
	GameUpdateOrBuild       string               `json:"gameUpdateOrBuild,omitempty"`
	ServerID                string               `json:"serverId,omitempty"`
	SelectedPets            []PetInput           `json:"selectedPets"`
	SpiritShards            *int                 `json:"spiritShards,omitempty"`
	DisplayedLuckMultiplier *float64             `json:"displayedLuckMultiplier,omitempty"`
	DisplayedWinProbability DisplayedProbability `json:"displayedWinProbability"`
	RewardProbabilities     []RewardProbability  `json:"rewardProbabilities,omitempty"`
	DisplayedSuccessCapPct  *float64             `json:"displayedSuccessCapPercentPoints,omitempty"`
	ForgeButtonEnabled      *bool                `json:"forgeButtonEnabled,omitempty"`
	ValidationWarnings      []string             `json:"validationWarnings,omitempty"` // e.g. TOO MANY PETS
	Notes                   string               `json:"notes,omitempty"`
	SourceScreenshot        string               `json:"sourceScreenshot,omitempty"`
	AttemptExecuted         bool                 `json:"attemptExecuted"`
	ObservedRewardID        string               `json:"observedRewardId,omitempty"`
	IsProbabilityPreview    bool                 `json:"isProbabilityPreview"`
	RecipeKind              string               `json:"recipeKind,omitempty"` // butler-only|mixed|unknown
	Confidence              string               `json:"confidence,omitempty"`
	Provenance              string               `json:"provenance,omitempty"`
}

// ObservationFile is a versioned collection of forge observations.
type ObservationFile struct {
	SchemaVersion string        `json:"schemaVersion"`
	Observations  []Observation `json:"observations"`
	Notes         []string      `json:"notes,omitempty"`
}

// LoadObservations reads an observation JSON file (array or {observations:[...]}).
func LoadObservations(path string) (ObservationFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ObservationFile{}, err
	}
	return ParseObservationsJSON(b)
}

func ParseObservationsJSON(b []byte) (ObservationFile, error) {
	var f ObservationFile
	if err := json.Unmarshal(b, &f); err == nil && (len(f.Observations) > 0 || f.SchemaVersion != "") {
		if f.SchemaVersion == "" {
			f.SchemaVersion = ObservationSchemaVersion
		}
		for i := range f.Observations {
			normalizeObservation(&f.Observations[i])
		}
		return f, nil
	}
	var arr []Observation
	if err := json.Unmarshal(b, &arr); err != nil {
		return ObservationFile{}, fmt.Errorf("parse observations: %w", err)
	}
	f = ObservationFile{SchemaVersion: ObservationSchemaVersion, Observations: arr}
	for i := range f.Observations {
		normalizeObservation(&f.Observations[i])
	}
	return f, nil
}

func normalizeObservation(o *Observation) {
	if o.SchemaVersion == "" {
		o.SchemaVersion = ObservationSchemaVersion
	}
	if o.EventID == "" {
		o.EventID = "HalloweenGames"
	}
	if o.Game == "" {
		o.Game = "Pet Simulator 99"
	}
	// Re-parse displayed probabilities from Raw if Fraction missing.
	if o.DisplayedWinProbability.Raw != "" && !o.DisplayedWinProbability.ParseOK {
		o.DisplayedWinProbability = ParseDisplayedProbability(o.DisplayedWinProbability.Raw)
	}
	for i := range o.RewardProbabilities {
		rp := &o.RewardProbabilities[i]
		if rp.Displayed.Raw != "" && !rp.Displayed.ParseOK {
			rp.Displayed = ParseDisplayedProbability(rp.Displayed.Raw)
		}
	}
	if o.RecipeKind == "" {
		o.RecipeKind = classifyRecipeKind(o.SelectedPets)
	}
	if !o.AttemptExecuted {
		o.IsProbabilityPreview = true
	}
}

func classifyRecipeKind(pets []PetInput) string {
	if len(pets) == 0 {
		return "unknown"
	}
	nonZero := 0
	onlyButler := true
	for _, p := range pets {
		if p.Quantity <= 0 {
			continue
		}
		nonZero++
		if !strings.EqualFold(p.ID, "Butler Penguin") {
			onlyButler = false
		}
	}
	if nonZero == 0 {
		return "unknown"
	}
	if onlyButler && nonZero == 1 {
		return "butler-only"
	}
	if onlyButler {
		// multiple butler variant lines still butler-only
		allButler := true
		for _, p := range pets {
			if p.Quantity > 0 && !strings.EqualFold(p.ID, "Butler Penguin") {
				allButler = false
			}
		}
		if allButler {
			return "butler-only"
		}
	}
	return "mixed"
}

// ComparableKey groups observations that may be fit together.
func (o Observation) ComparableKey() string {
	shards := "shards=?"
	if o.SpiritShards != nil {
		shards = fmt.Sprintf("shards=%d", *o.SpiritShards)
	}
	luck := "luck=?"
	if o.DisplayedLuckMultiplier != nil {
		luck = fmt.Sprintf("luck=%g", *o.DisplayedLuckMultiplier)
	}
	pets := make([]string, 0, len(o.SelectedPets))
	for _, p := range o.SelectedPets {
		v := p.Variant
		if v == "" {
			v = "Regular"
		}
		pets = append(pets, fmt.Sprintf("%s|%s", p.ID, v))
	}
	sort.Strings(pets)
	return strings.Join([]string{
		o.EventID,
		o.GameUpdateOrBuild,
		o.RecipeKind,
		shards,
		luck,
		"pets=" + strings.Join(pets, "+"),
	}, "|")
}

// ButlerOnlyQuantity returns Butler Penguin quantity when recipe is butler-only with a single variant line.
func (o Observation) ButlerOnlyQuantity() (int, bool) {
	if o.RecipeKind != "butler-only" {
		return 0, false
	}
	n := 0
	for _, p := range o.SelectedPets {
		if strings.EqualFold(p.ID, "Butler Penguin") {
			n += p.Quantity
		}
	}
	if n <= 0 {
		return 0, false
	}
	return n, true
}

// ParseTime attempts to parse ObservedAt.
func (o Observation) ParseTime() (time.Time, error) {
	if o.ObservedAt == "" {
		return time.Time{}, fmt.Errorf("missing observedAt")
	}
	if t, err := time.Parse(time.RFC3339Nano, o.ObservedAt); err == nil {
		return t, nil
	}
	return time.Parse(time.RFC3339, o.ObservedAt)
}
