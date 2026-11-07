package snapshot

import (
	"strings"
)

// CapFindingCapped is the stable message when Δbalance/Δt cannot identify income at a ceiling.
const CapFindingCapped = "CAPPED: income rate cannot be identified from balance differences"

// KnownCurrencyCapTexts maps currency IDs to observed exact ceiling balances.
// Provenance is observational (account snapshots), not an official API field.
var KnownCurrencyCapTexts = map[string]string{
	"FantasyCoins": "300000000000000000000",
}

// LookupKnownCurrencyCap returns the exact ceiling text when known.
func LookupKnownCurrencyCap(currencyID string) (string, bool) {
	for id, bal := range KnownCurrencyCapTexts {
		if strings.EqualFold(id, currencyID) {
			return bal, true
		}
	}
	return "", false
}

// CurrencyAtKnownCap reports whether both balances equal a known ceiling with a zero delta.
func CurrencyAtKnownCap(currencyID string, start, end, delta ExactInt) bool {
	capText, ok := LookupKnownCurrencyCap(currencyID)
	if !ok || !start.ParseOK || !end.ParseOK || start.Int == nil || end.Int == nil {
		return false
	}
	cap := ParseExactIntString(capText)
	if !cap.ParseOK || cap.Int == nil {
		return false
	}
	if start.Int.Cmp(cap.Int) != 0 || end.Int.Cmp(cap.Int) != 0 {
		return false
	}
	return delta.ParseOK && delta.Int != nil && delta.Int.Sign() == 0
}
