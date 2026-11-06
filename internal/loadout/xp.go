package loadout

import "math"

// HugeXPCurveID identifies a versioned displayed level→damage bonus model.
const HugeXPCurveCommunityV1 = "community-huge-xp-display-v1"

// FixtureDisplayedHugeXPBonus are screenshot / observation targets (percent points).
var FixtureDisplayedHugeXPBonus = map[int]float64{
	59: 42.1,
	76: 56.9,
	78: 59.3,
	58: 41.5, // Golden Huge Dalmatian observation under same display curve
}

// DisplayedHugeXPBonusPercent returns the displayed XP damage bonus percent for a Huge level.
// Source: fixture table for known levels; power-law fit community-huge-xp-display-v1 otherwise.
// Confidence is measured for fixture levels, assumed for interpolated/extrapolated values.
// Level 1 → 0. Level 99 is extrapolated and flagged.
func DisplayedHugeXPBonusPercent(level int) (percent float64, curveID string, confidence Confidence, note string) {
	curveID = HugeXPCurveCommunityV1
	if level <= 1 {
		return 0, curveID, ConfidenceMeasured, "level <= 1 treated as +0% displayed bonus"
	}
	if v, ok := FixtureDisplayedHugeXPBonus[level]; ok {
		return v, curveID, ConfidenceMeasured, "exact fixture / observation point"
	}
	// Power-law fit to fixtures: bonus ≈ a * (level-1)^b
	// Calibrated to L59/L76/L78 (see docs/research/pet-power-and-events.md).
	const a, b = 0.34914734420176946, 1.1809687697654687 // least-squares on L58/59/76/78 fixtures
	x := float64(level - 1)
	percent = a * math.Pow(x, b)
	conf := ConfidenceAssumed
	note = "interpolated/extrapolated via " + curveID + "; not an official BIG Games formula"
	if level >= 99 {
		note += "; level 99 boundary is extrapolated"
	}
	return percent, curveID, conf, note
}

// LevelFactor converts displayed bonus percent to a multiplier (1 + p/100).
func LevelFactor(bonusPercent float64) float64 {
	return 1 + bonusPercent/100
}
