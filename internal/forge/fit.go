package forge

import (
	"fmt"
	"math"
	"sort"
	"strings"
)

const FitMethodCappedLinearOLS = "capped-linear-ols.v1"

// FitPoint is one (n, p) observation used for fitting.
type FitPoint struct {
	N              int
	P              float64 // fraction
	RawPercent     string
	ObservationID  string
	RoundingHalfUL float64 // half unit in last place of displayed percent, as fraction
}

// CappedLinearModel is P = min(slope * n + intercept, cap).
type CappedLinearModel struct {
	MethodID      string    `json:"methodId"`
	Slope         float64   `json:"slopePerUnit"` // ΔP per ingredient (fraction)
	Intercept     float64   `json:"intercept"`
	Cap           *float64  `json:"capFraction,omitempty"`
	CapSource     string    `json:"capSource,omitempty"`
	NPoints       int       `json:"nPoints"`
	RMSE          float64   `json:"rmse"`
	MaxAbsResid   float64   `json:"maxAbsResidual"`
	Residuals     []float64 `json:"residuals,omitempty"`
	Provenance    string    `json:"provenance"`
	Limitations   []string  `json:"limitations"`
	CompatibleKey string    `json:"comparableKey"`
}

// AnalyzeReport summarizes forge observation analysis.
type AnalyzeReport struct {
	OK       bool            `json:"ok"`
	Groups   []GroupAnalysis `json:"groups"`
	Warnings []string        `json:"warnings"`
	MethodID string          `json:"methodId"`
	Notes    []string        `json:"notes"`
}

// GroupAnalysis is one comparable observation cluster.
type GroupAnalysis struct {
	Key             string             `json:"key"`
	NObservations   int                `json:"nObservations"`
	Model           *CappedLinearModel `json:"model,omitempty"`
	MarginalPerUnit *float64           `json:"marginalDisplayedProbabilityPerUnit,omitempty"`
	Incompatible    []string           `json:"incompatibleNotes,omitempty"`
	Points          []map[string]any   `json:"points"`
}

// AnalyzeObservations clusters and fits where justified.
func AnalyzeObservations(file ObservationFile) AnalyzeReport {
	rep := AnalyzeReport{
		OK:       true,
		MethodID: "forge-analyze.v1",
		Notes: []string{
			"Fits describe displayed UI probabilities, not proven server RNG",
			"Do not derive confidence intervals for real forge outcomes from screenshots alone",
			"Butler-only and mixed recipes are never averaged together",
		},
		Warnings: []string{},
	}
	groups := map[string][]Observation{}
	for _, o := range file.Observations {
		k := o.ComparableKey()
		groups[k] = append(groups[k], o)
	}
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		obs := groups[k]
		ga := GroupAnalysis{Key: k, NObservations: len(obs)}
		var pts []FitPoint
		for _, o := range obs {
			row := map[string]any{
				"observationId":   o.ObservationID,
				"recipeKind":      o.RecipeKind,
				"winRaw":          o.DisplayedWinProbability.Raw,
				"attemptExecuted": o.AttemptExecuted,
			}
			if n, ok := o.ButlerOnlyQuantity(); ok && o.DisplayedWinProbability.Fraction != nil {
				pts = append(pts, FitPoint{
					N: n, P: *o.DisplayedWinProbability.Fraction,
					RawPercent: o.DisplayedWinProbability.Raw, ObservationID: o.ObservationID,
					RoundingHalfUL: estimateRoundingHalfUL(o.DisplayedWinProbability),
				})
				row["butlerN"] = n
				row["winFraction"] = *o.DisplayedWinProbability.Fraction
			} else if o.RecipeKind == "mixed" {
				ga.Incompatible = append(ga.Incompatible, "mixed recipe excluded from butler-only linear fit")
			} else if o.DisplayedWinProbability.Fraction == nil {
				ga.Incompatible = append(ga.Incompatible, "missing numeric win probability")
			}
			ga.Points = append(ga.Points, row)
		}
		if len(pts) >= 2 && stringsHasButlerOnly(k) {
			model := FitCappedLinear(pts, detectCap(obs))
			model.CompatibleKey = k
			ga.Model = &model
			m := model.Slope
			ga.MarginalPerUnit = &m
		} else if len(pts) == 1 {
			ga.Incompatible = append(ga.Incompatible, "insufficient points to fit; need >=2 comparable butler-only observations")
		}
		rep.Groups = append(rep.Groups, ga)
	}
	return rep
}

func stringsHasButlerOnly(k string) bool {
	return strings.Contains(k, "butler-only")
}

func detectCap(obs []Observation) *float64 {
	for _, o := range obs {
		if o.DisplayedSuccessCapPct != nil {
			c := *o.DisplayedSuccessCapPct / 100
			return &c
		}
	}
	// If any observation sits at exactly 0.75 with note of cap, use it — only from data.
	for _, o := range obs {
		if o.DisplayedWinProbability.Fraction != nil && math.Abs(*o.DisplayedWinProbability.Fraction-0.75) < 1e-9 {
			c := 0.75
			return &c
		}
	}
	return nil
}

func estimateRoundingHalfUL(p DisplayedProbability) float64 {
	raw := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(p.Raw), "%"))
	dec := 0
	if i := strings.IndexByte(raw, '.'); i >= 0 {
		dec = len(raw) - i - 1
	}
	ulpPct := math.Pow10(-dec)
	return (ulpPct / 2) / 100
}

// FitCappedLinear fits uncapped OLS on points below the cap, then applies min(., cap).
// Intercept forced near 0 when justified by butler-only physical model (0 pets → 0).
func FitCappedLinear(pts []FitPoint, cap *float64) CappedLinearModel {
	m := CappedLinearModel{
		MethodID:   FitMethodCappedLinearOLS,
		Provenance: "least-squares on displayed win fractions for comparable butler-only observations",
		Limitations: []string{
			"empirical approximation of displayed probabilities only",
			"displayed rounding may absorb residuals",
			"linearity not assumed for other pet types or mixed recipes",
			"cap may be event/version specific",
		},
		Cap: cap,
	}
	if cap != nil {
		m.CapSource = "observed displayedSuccessCap or 75.0% plateau in data"
	}
	// Use points strictly below cap for slope (if cap known), else all.
	var xs, ys []float64
	for _, p := range pts {
		if cap != nil && p.P >= *cap-1e-12 {
			continue
		}
		xs = append(xs, float64(p.N))
		ys = append(ys, p.P)
	}
	if len(xs) < 1 {
		// all at cap — slope unknown
		m.NPoints = len(pts)
		m.Limitations = append(m.Limitations, "all points at/above cap; slope not identifiable")
		return m
	}
	// Through-origin OLS: slope = sum(x y)/sum(x^2)
	var sxy, sxx float64
	for i := range xs {
		sxy += xs[i] * ys[i]
		sxx += xs[i] * xs[i]
	}
	if sxx == 0 {
		return m
	}
	m.Slope = sxy / sxx
	m.Intercept = 0
	m.NPoints = len(pts)
	var sse, maxAbs float64
	m.Residuals = make([]float64, len(pts))
	for i, p := range pts {
		pred := m.Predict(p.N)
		r := p.P - pred
		m.Residuals[i] = r
		sse += r * r
		if math.Abs(r) > maxAbs {
			maxAbs = math.Abs(r)
		}
	}
	m.RMSE = math.Sqrt(sse / float64(len(pts)))
	m.MaxAbsResid = maxAbs
	return m
}

// Predict returns capped linear prediction.
func (m CappedLinearModel) Predict(n int) float64 {
	y := m.Intercept + m.Slope*float64(n)
	if m.Cap != nil && y > *m.Cap {
		return *m.Cap
	}
	if y < 0 {
		return 0
	}
	return y
}

// ExpectedButlersPerSuccess returns n/p for displayed probability p.
func ExpectedButlersPerSuccess(n int, p float64) (float64, error) {
	if n <= 0 {
		return 0, fmt.Errorf("n must be positive")
	}
	if p <= 0 || math.IsNaN(p) {
		return 0, fmt.Errorf("probability must be positive")
	}
	return float64(n) / p, nil
}

// ExpectedShardsPerSuccess returns shards/p.
func ExpectedShardsPerSuccess(shards int, p float64) (float64, error) {
	if shards < 0 {
		return 0, fmt.Errorf("shards must be non-negative")
	}
	if p <= 0 || math.IsNaN(p) {
		return 0, fmt.Errorf("probability must be positive")
	}
	return float64(shards) / p, nil
}
