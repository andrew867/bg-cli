package forge

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// DisplayedProbability preserves UI percentage text exactly, including censoring.
type DisplayedProbability struct {
	Raw           string   `json:"raw"`                     // exact UI string, e.g. "2.58%", "<0.001%", "75.0%"
	PercentPoints *float64 `json:"percentPoints,omitempty"` // nil when censored/unknown
	Fraction      *float64 `json:"fraction,omitempty"`      // percent/100 when known
	Censored      bool     `json:"censored"`
	UpperBoundPct *float64 `json:"upperBoundPercentPoints,omitempty"` // for "<0.001%"
	ParseOK       bool     `json:"parseOk"`
	Notes         string   `json:"notes,omitempty"`
}

// ParseDisplayedProbability parses UI probability strings without inventing zeros for censored values.
func ParseDisplayedProbability(raw string) DisplayedProbability {
	out := DisplayedProbability{Raw: strings.TrimSpace(raw)}
	s := out.Raw
	if s == "" {
		out.Notes = "empty probability"
		return out
	}
	lower := strings.ToLower(s)
	if strings.HasPrefix(lower, "<") {
		out.Censored = true
		rest := strings.TrimSpace(s[1:])
		rest = strings.TrimSuffix(rest, "%")
		rest = strings.TrimSpace(rest)
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			out.Notes = "censored value unparsable: " + err.Error()
			return out
		}
		out.UpperBoundPct = &v
		out.ParseOK = true
		out.Notes = "censored upper bound only; not replaced with zero or exact probability"
		return out
	}
	rest := strings.TrimSuffix(s, "%")
	rest = strings.TrimSpace(rest)
	v, err := strconv.ParseFloat(rest, 64)
	if err != nil {
		out.Notes = err.Error()
		return out
	}
	out.PercentPoints = &v
	f := v / 100
	out.Fraction = &f
	out.ParseOK = true
	return out
}

func (p DisplayedProbability) FractionOrNaN() float64 {
	if p.Fraction == nil {
		return math.NaN()
	}
	return *p.Fraction
}

func FormatPercent(frac float64, decimals int) string {
	if math.IsNaN(frac) || math.IsInf(frac, 0) {
		return "unknown"
	}
	pct := frac * 100
	return fmt.Sprintf("%.*f%%", decimals, pct)
}
