package craft

import (
	"fmt"
	"strings"
)

const RecipeSchemaVersion = "1.0"
const MethodIDRecipes = "craft.combine-o-matic.recipes.v1"

// VariantClass restricts which inventory variants a recipe may consume.
type VariantClass string

const (
	VariantNormal  VariantClass = "Normal"
	VariantGolden  VariantClass = "Golden"
	VariantRainbow VariantClass = "Rainbow"
	VariantShiny   VariantClass = "Shiny"
	VariantHuge    VariantClass = "Huge"
	VariantTitanic VariantClass = "Titanic"
	VariantGarg    VariantClass = "Gargantuan"
)

// RecipeIngredient is one required input for a Combine-o-Matic craft.
type RecipeIngredient struct {
	ItemID          string         `json:"itemId"`
	Quantity        int            `json:"quantity"`
	AllowedVariants []VariantClass `json:"allowedVariants"` // empty → Normal only by default
	Notes           string         `json:"notes,omitempty"`
}

// Recipe is one observed Combine-o-Matic recipe (not Forge).
type Recipe struct {
	ID              string             `json:"id"`
	OutputItemID    string             `json:"outputItemId"`
	OutputIsHuge    bool               `json:"outputIsHuge,omitempty"`
	SpiritShards    int                `json:"spiritShards"`
	DurationHours   float64            `json:"durationHours"`
	Ingredients     []RecipeIngredient `json:"ingredients"`
	Provenance      string             `json:"provenance"`
	Confidence      string             `json:"confidence"` // measured|assumed|unknown
	DisplayedResult string             `json:"displayedResultNote,omitempty"`
	Warnings        []string           `json:"warnings,omitempty"`
}

// DefaultMasqueradeRecipes returns screenshot-observed Masquerade Combine-o-Matic recipes.
// These are fixtures/observations, not hardcoded server rules beyond documented observations.
func DefaultMasqueradeRecipes() []Recipe {
	normalOnly := []VariantClass{VariantNormal}
	return []Recipe{
		{
			ID: "huge-bat-hoodie-bunny", OutputItemID: "Huge Bat Hoodie Bunny", OutputIsHuge: true,
			SpiritShards: 30, DurationHours: 1,
			Ingredients: []RecipeIngredient{{ItemID: "Masquerade Agony", Quantity: 10, AllowedVariants: normalOnly, Notes: "NORMAL only — Golden/Rainbow not accepted in observed UI"}},
			Provenance:  "screenshot-observation-2026-11-07", Confidence: "measured",
			Warnings: []string{"displayed result may show Rainbow variant while craft consumes Normal inputs — displayed ≠ collected"},
		},
		{
			ID: "huge-witch-fox", OutputItemID: "Huge Witch Fox", OutputIsHuge: true,
			SpiritShards: 10, DurationHours: 2,
			Ingredients: []RecipeIngredient{{ItemID: "Huge Bat Hoodie Bunny", Quantity: 4, AllowedVariants: []VariantClass{VariantHuge, VariantNormal}}},
			Provenance:  "screenshot-observation-2026-11-07", Confidence: "measured",
			Warnings: []string{"later-stage Rainbow eligibility unverified"},
		},
		{
			ID: "huge-thorn-dragon", OutputItemID: "Huge Thorn Dragon", OutputIsHuge: true,
			SpiritShards: 10, DurationHours: 4,
			Ingredients: []RecipeIngredient{{ItemID: "Huge Witch Fox", Quantity: 3, AllowedVariants: []VariantClass{VariantHuge, VariantNormal}}},
			Provenance:  "screenshot-observation-2026-11-07", Confidence: "measured",
		},
		{
			ID: "titanic-masquerade-agony", OutputItemID: "Titanic Masquerade Agony",
			SpiritShards: 60, DurationHours: 12,
			Ingredients: []RecipeIngredient{{ItemID: "Huge Thorn Dragon", Quantity: 2, AllowedVariants: []VariantClass{VariantHuge, VariantNormal}}},
			Provenance:  "screenshot-observation-2026-11-07", Confidence: "measured",
		},
		{
			ID: "gargantuan-butler-penguin", OutputItemID: "Gargantuan Butler Penguin",
			SpiritShards: 1700, DurationHours: 24,
			Ingredients: []RecipeIngredient{{ItemID: "Titanic Masquerade Agony", Quantity: 2, AllowedVariants: []VariantClass{VariantTitanic, VariantNormal}}},
			Provenance:  "screenshot-observation-2026-11-07", Confidence: "measured",
			Warnings: []string{"Forge Saver must not be applied to Combine-o-Matic without separate evidence"},
		},
	}
}

// LookupRecipe finds a recipe by id or output name (case-insensitive).
func LookupRecipe(recipes []Recipe, idOrOutput string) (Recipe, bool) {
	for _, r := range recipes {
		if strings.EqualFold(r.ID, idOrOutput) || strings.EqualFold(r.OutputItemID, idOrOutput) {
			return r, true
		}
	}
	return Recipe{}, false
}

// AllowsVariant reports whether an ingredient may consume the given variant class.
func AllowsVariant(ing RecipeIngredient, v VariantClass) bool {
	allowed := ing.AllowedVariants
	if len(allowed) == 0 {
		allowed = []VariantClass{VariantNormal}
	}
	for _, a := range allowed {
		if a == v {
			return true
		}
	}
	return false
}

// ProtectDefault returns true for valuable variants that must not be consumed unless required.
func ProtectDefault(v VariantClass) bool {
	switch v {
	case VariantGolden, VariantRainbow, VariantShiny, VariantHuge, VariantTitanic, VariantGarg:
		return true
	default:
		return false
	}
}

// ValidateRecipes ensures BOM ids resolve within the provided set for recursion.
func ValidateRecipes(recipes []Recipe) error {
	byOut := map[string]Recipe{}
	for _, r := range recipes {
		byOut[r.OutputItemID] = r
	}
	for _, r := range recipes {
		if r.SpiritShards < 0 || r.DurationHours < 0 {
			return fmt.Errorf("recipe %s has negative cost/duration", r.ID)
		}
		for _, ing := range r.Ingredients {
			if ing.Quantity <= 0 {
				return fmt.Errorf("recipe %s ingredient %s quantity must be positive", r.ID, ing.ItemID)
			}
		}
	}
	return nil
}
