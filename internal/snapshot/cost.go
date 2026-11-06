package snapshot

import (
	"math/big"
	"time"
)

// CostInput holds explicit user-supplied pricing/time parameters.
type CostInput struct {
	EggPrice     ExactInt // required for spend calcs
	CurrencyID   string
	FutureEggs   int64
	BatchSize    int64
	HasEggPrice  bool
	HasCurrency  bool
	HasFuture    bool
	HasBatchSize bool
}

// CostResult is currency and timing estimates under stated assumptions.
type CostResult struct {
	CurrencyID               string   `json:"currencyId,omitempty"`
	EggPrice                 string   `json:"eggPrice,omitempty"`
	EggsPurchasedDelta       string   `json:"eggsPurchasedDelta,omitempty"`
	GrossEggSpend            string   `json:"grossEggSpend,omitempty"`
	StartingBalance          string   `json:"startingBalance,omitempty"`
	EndingBalance            string   `json:"endingBalance,omitempty"`
	CurrencyDelta            string   `json:"currencyDelta,omitempty"`
	ImpliedOtherNetFlows     string   `json:"impliedOtherNetFlows,omitempty"`
	ImpliedOtherNetFlowsNote string   `json:"impliedOtherNetFlowsNote"`
	CurrencyForFutureEggs    string   `json:"currencyRequiredForFutureEggs,omitempty"`
	RemainingPurchasableEggs *string  `json:"remainingPurchasableEggsFromLatestBalance,omitempty"`
	ExpectedPetsAtObserved   *float64 `json:"expectedPetsAtObservedRate,omitempty"`
	ProbAtLeastOneFuture     *float64 `json:"probAtLeastOneFuture,omitempty"`
	ElapsedSeconds           *float64 `json:"elapsedSeconds,omitempty"`
	ElapsedBasis             string   `json:"elapsedBasis,omitempty"`
	EggsPerMinute            *float64 `json:"eggsPerMinute,omitempty"`
	EggsPerHour              *float64 `json:"eggsPerHour,omitempty"`
	BatchDurationSeconds     *float64 `json:"batchDurationSeconds,omitempty"`
	FutureDurationSeconds    *float64 `json:"estimatedDurationForFutureEggsSeconds,omitempty"`
	Warnings                 []string `json:"warnings,omitempty"`
}

// ComputeCostTiming derives spend and optional timing metrics.
func ComputeCostTiming(
	in CostInput,
	eggsDelta ExactInt,
	startBal, endBal ExactInt,
	observedRate *float64,
	fromSnapAt, toSnapAt *string,
) CostResult {
	out := CostResult{
		CurrencyID:               in.CurrencyID,
		ImpliedOtherNetFlowsNote: "endingCurrency - startingCurrency + grossEggSpend; may include farming, rewards, gifts, purchases, and other sources/sinks — not labeled as farming income",
		Warnings:                 []string{},
	}
	if eggsDelta.ParseOK {
		out.EggsPurchasedDelta = eggsDelta.Text
	}
	if in.HasEggPrice && in.EggPrice.ParseOK && eggsDelta.ParseOK {
		out.EggPrice = in.EggPrice.Text
		spend := MulInt64(eggsDelta, 0)
		// MulInt64 with egg price: eggs * price
		if eggsDelta.Int != nil && in.EggPrice.Int != nil {
			s := new(big.Int).Mul(eggsDelta.Int, in.EggPrice.Int)
			spend = ExactInt{Int: s, Text: s.String(), ParseOK: true, SourceText: s.String()}
			out.GrossEggSpend = spend.Text
		}
		if in.HasCurrency && startBal.ParseOK && endBal.ParseOK {
			out.StartingBalance = startBal.Text
			out.EndingBalance = endBal.Text
			if d, err := SubExact(endBal, startBal); err == nil {
				out.CurrencyDelta = d.Text
				// implied = ending - starting + gross
				imp := new(big.Int).Add(d.Int, spend.Int)
				out.ImpliedOtherNetFlows = imp.String()
			}
		}
		if in.HasFuture && in.FutureEggs > 0 && in.EggPrice.Int != nil {
			need := new(big.Int).Mul(big.NewInt(in.FutureEggs), in.EggPrice.Int)
			out.CurrencyForFutureEggs = need.String()
		}
		if in.HasCurrency && endBal.ParseOK && endBal.Int != nil && in.EggPrice.Int != nil && in.EggPrice.Int.Sign() > 0 {
			q := new(big.Int).Div(endBal.Int, in.EggPrice.Int)
			s := q.String()
			out.RemainingPurchasableEggs = &s
			out.Warnings = append(out.Warnings, "remaining purchasable eggs assumes no additional income and ignores other currency sinks")
		}
	}
	if observedRate != nil && in.HasFuture && in.FutureEggs > 0 {
		exp := (*observedRate) * float64(in.FutureEggs)
		out.ExpectedPetsAtObserved = &exp
		prob := probAtLeastOne(*observedRate, float64(in.FutureEggs))
		out.ProbAtLeastOneFuture = &prob
	}

	// Timing from inventory snapshot timestamps when both present.
	if fromSnapAt != nil && toSnapAt != nil {
		t0, err0 := time.Parse(time.RFC3339Nano, *fromSnapAt)
		t1, err1 := time.Parse(time.RFC3339Nano, *toSnapAt)
		if err0 != nil {
			t0, err0 = time.Parse(time.RFC3339, *fromSnapAt)
		}
		if err1 != nil {
			t1, err1 = time.Parse(time.RFC3339, *toSnapAt)
		}
		if err0 == nil && err1 == nil {
			sec := t1.Sub(t0).Seconds()
			if sec > 0 && eggsDelta.ParseOK && eggsDelta.Int != nil {
				out.ElapsedSeconds = &sec
				out.ElapsedBasis = "inventory snapshotAt proxy; profile counters may have been saved at different times — rates are not labeled exact"
				ef, _ := new(big.Float).SetInt(eggsDelta.Int).Float64()
				epm := ef / (sec / 60)
				eph := ef / (sec / 3600)
				out.EggsPerMinute = &epm
				out.EggsPerHour = &eph
				if in.HasBatchSize && in.BatchSize > 0 && epm > 0 {
					bd := float64(in.BatchSize) / epm * 60
					out.BatchDurationSeconds = &bd
				}
				if in.HasFuture && in.FutureEggs > 0 && eph > 0 {
					fd := float64(in.FutureEggs) / eph * 3600
					out.FutureDurationSeconds = &fd
				}
			} else if sec <= 0 {
				out.Warnings = append(out.Warnings, "non-positive elapsed snapshotAt interval; timing rates null")
			}
		} else {
			out.Warnings = append(out.Warnings, "could not parse snapshotAt timestamps for timing")
		}
	} else {
		out.Warnings = append(out.Warnings, "missing inventory snapshotAt; timing rates null")
	}
	return out
}
