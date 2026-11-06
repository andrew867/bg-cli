package loadout

import "strings"

// EquipBestComparison contrasts game-observed order with model-visible order.
type EquipBestComparison struct {
	WorldID             string   `json:"worldId"`
	GameOrder           []string `json:"gameOrder"`
	EquippedOrder       []string `json:"equippedInventoryOrder"`
	AgreementPrefix     int      `json:"agreementPrefixLength"`
	Disagreements       []string `json:"disagreements"`
	MissingCoefficients []string `json:"missingCoefficientsThatCouldExplainGaps"`
	Notes               []string `json:"notes"`
}

// CompareEquipBest records disagreements without declaring the game wrong or the model right.
func CompareEquipBest(gameOrder []string, pl PlayerLoadout, pets []PetPowerBreakdown, world string) EquipBestComparison {
	eq := idsOf(pl.Pets.List)
	cmp := EquipBestComparison{
		WorldID:       world,
		GameOrder:     append([]string{}, gameOrder...),
		EquippedOrder: eq,
		Notes: []string{
			"Equip Best (Update 100) is treated as an external observation, not an infallible oracle",
			"inventory UI sorting and equipped team order may use different calculations",
		},
		MissingCoefficients: []string{
			"per-instance Huge level",
			"attached charms",
			"verified event share model (A–D)",
			"variant multipliers",
			"event team slot limit",
		},
	}
	n := len(gameOrder)
	if len(eq) < n {
		n = len(eq)
	}
	for i := 0; i < n; i++ {
		if strings.EqualFold(gameOrder[i], eq[i]) {
			cmp.AgreementPrefix++
			continue
		}
		cmp.Disagreements = append(cmp.Disagreements,
			"position "+itoa(i)+": game="+gameOrder[i]+" equipped="+eq[i])
		break
	}
	for _, p := range pets {
		if len(p.Missing) > 0 {
			cmp.Notes = append(cmp.Notes, "pet "+p.PetID+" missing: "+strings.Join(p.Missing, ","))
		}
	}
	return cmp
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	pos := len(b)
	for i > 0 {
		pos--
		b[pos] = byte('0' + i%10)
		i /= 10
	}
	return string(b[pos:])
}
