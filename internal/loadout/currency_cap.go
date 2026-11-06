package loadout

import (
	"math/big"
	"strings"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

// CapFindingCapped re-exports the snapshot package constant for loadout callers.
const CapFindingCapped = snapshot.CapFindingCapped

// CurrencyCapInfo describes one known or override currency ceiling.
type CurrencyCapInfo struct {
	BalanceText string
	Source      string
	Notes       string
}

// KnownCurrencyCaps are observed balance ceilings where excess production is discarded.
var KnownCurrencyCaps = map[string]CurrencyCapInfo{
	"FantasyCoins": {
		BalanceText: snapshot.KnownCurrencyCapTexts["FantasyCoins"],
		Source:      "observed-account-snapshots",
		Notes:       "Repeated identical FantasyCoins balances across sessions (e.g. 2026-11-06 12:48 and 13:59 NST) with continued play; excess production appears discarded at this ceiling",
	},
}

// CapAssessment is the calibration finding for a currency interval.
type CapAssessment struct {
	Capped                      bool   `json:"capped"`
	Status                      string `json:"status,omitempty"` // CAPPED | ""
	CapBalance                  string `json:"capBalance,omitempty"`
	CapSource                   string `json:"capSource,omitempty"`
	Finding                     string `json:"finding,omitempty"`
	IncomeRateIdentifiable      bool   `json:"incomeRateIdentifiable"`
	ProductionMayOffsetSpending bool   `json:"productionMayOffsetSpending,omitempty"`
	ProductionOffsetNote        string `json:"productionOffsetNote,omitempty"`
	BothBalancesEqualCap        bool   `json:"bothBalancesEqualCap,omitempty"`
	ZeroDelta                   bool   `json:"zeroDelta,omitempty"`
}

func resolveCapBalance(currencyID, overrideCap string) (CurrencyCapInfo, *big.Int, bool) {
	if strings.TrimSpace(overrideCap) != "" {
		ei := snapshot.ParseExactIntString(strings.TrimSpace(overrideCap))
		if !ei.ParseOK || ei.Int == nil {
			return CurrencyCapInfo{}, nil, false
		}
		info := CurrencyCapInfo{
			BalanceText: ei.Text,
			Source:      "user-override",
			Notes:       "explicit --currency-cap / CalibrationInput.CurrencyCap override",
		}
		return info, ei.Int, true
	}
	if bal, ok := snapshot.LookupKnownCurrencyCap(currencyID); ok {
		info, found := KnownCurrencyCaps[currencyID]
		if !found {
			for id, v := range KnownCurrencyCaps {
				if strings.EqualFold(id, currencyID) {
					info, found = v, true
					break
				}
			}
		}
		if !found {
			info = CurrencyCapInfo{BalanceText: bal, Source: "observed-account-snapshots"}
		}
		ei := snapshot.ParseExactIntString(bal)
		if !ei.ParseOK || ei.Int == nil {
			return CurrencyCapInfo{}, nil, false
		}
		return info, ei.Int, true
	}
	return CurrencyCapInfo{}, nil, false
}

// AssessCurrencyCap classifies whether a balance delta can identify income rate under a known ceiling.
func AssessCurrencyCap(currencyID string, start, end, delta snapshot.ExactInt, overrideCap string, spendConfirmed bool) CapAssessment {
	out := CapAssessment{IncomeRateIdentifiable: true}
	if !start.ParseOK || !end.ParseOK || start.Int == nil || end.Int == nil {
		return out
	}
	info, capBal, ok := resolveCapBalance(currencyID, overrideCap)
	if !ok {
		return out
	}
	atStart := start.Int.Cmp(capBal) == 0
	atEnd := end.Int.Cmp(capBal) == 0
	out.BothBalancesEqualCap = atStart && atEnd
	out.ZeroDelta = delta.ParseOK && delta.Int != nil && delta.Int.Sign() == 0
	out.CapBalance = info.BalanceText
	out.CapSource = info.Source

	if out.BothBalancesEqualCap && out.ZeroDelta {
		out.Capped = true
		out.Status = "CAPPED"
		out.Finding = CapFindingCapped
		out.IncomeRateIdentifiable = false
		if spendConfirmed {
			out.ProductionMayOffsetSpending = true
			out.ProductionOffsetNote = "simultaneous spending confirmed while balance remained at the known ceiling; production appears sufficient to offset spending, but no precise income rate is identifiable from balance differences"
		}
		return out
	}
	if atEnd && delta.ParseOK && delta.Int != nil && delta.Int.Sign() > 0 {
		out.Capped = true
		out.Status = "CAPPED"
		out.Finding = CapFindingCapped + " (ending balance sits at known ceiling; gross production may exceed observed delta)"
		out.IncomeRateIdentifiable = false
		return out
	}
	return out
}

// ConcurrentSpendSignals reports whether profile counters imply outbound activity during an interval.
// A positive Eggs Opened or PurchasedEggs total increase is treated as confirmed concurrent sink activity
// for the purpose of the capped-offset note — not as proof of the exact currency spent.
func ConcurrentSpendSignals(from, to *snapshot.Bundle) bool {
	if from == nil || to == nil || from.ProfileData == nil || to.ProfileData == nil {
		return false
	}
	fp, err1 := snapshot.ExtractProfileView(from.ProfileData)
	tp, err2 := snapshot.ExtractProfileView(to.ProfileData)
	if err1 != nil || err2 != nil {
		return false
	}
	eggs := snapshot.LookupCI(snapshot.DiffExactMaps(tp.Statistics, fp.Statistics), "Eggs Opened")
	if eggs.ParseOK && eggs.Int != nil && eggs.Int.Sign() > 0 {
		return true
	}
	for _, d := range snapshot.DiffExactMaps(tp.PurchasedEggs, fp.PurchasedEggs) {
		if d.ParseOK && d.Int != nil && d.Int.Sign() > 0 {
			return true
		}
	}
	return false
}
