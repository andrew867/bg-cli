package snapshot

import (
	"math"
	"math/big"
)

// HatchRateInput is user-selected egg/pet association for interval analysis.
type HatchRateInput struct {
	EggID string
	PetID string
	// EggsPurchasedDelta is the selected egg purchase delta (exact).
	EggsPurchasedDelta ExactInt
	// PetHatchDelta is the selected pet lifetime hatch delta (exact).
	PetHatchDelta ExactInt
	FutureEggs    int64 // optional N for P(at least one)
}

// HatchRateResult is observed-rate analysis (not official game odds).
type HatchRateResult struct {
	EggID                string   `json:"eggId"`
	PetID                string   `json:"petId"`
	Assumption           string   `json:"assumption"`
	EggsPurchasedDelta   string   `json:"eggsPurchasedDelta"`
	PetHatchDelta        string   `json:"petHatchDelta"`
	ObservedRate         *float64 `json:"observedRate"` // successes/trials; nil if undefined
	ObservedPercent      *float64 `json:"observedPercent"`
	AvgEggsPerSuccess    *float64 `json:"avgEggsPerSuccess"`
	ExpectedPer1000      *float64 `json:"expectedSuccessesPer1000"`
	ExpectedPer10000     *float64 `json:"expectedSuccessesPer10000"`
	WilsonLow95          *float64 `json:"wilsonLow95"`
	WilsonHigh95         *float64 `json:"wilsonHigh95"`
	FutureEggs           int64    `json:"futureEggs,omitempty"`
	ProbAtLeastOneFuture *float64 `json:"probAtLeastOneFuture,omitempty"`
	ProbModel            string   `json:"probModel"`
	ZeroSuccessNote      string   `json:"zeroSuccessNote,omitempty"`
	SmallSampleWarning   string   `json:"smallSampleWarning,omitempty"`
	OtherEggsChanged     []string `json:"otherEggsChanged,omitempty"`
	MethodID             string   `json:"methodId"`
}

// ComputeHatchRate calculates observed hatch rates and Wilson CI.
func ComputeHatchRate(in HatchRateInput, otherEggsChanged []string) HatchRateResult {
	out := HatchRateResult{
		EggID:            in.EggID,
		PetID:            in.PetID,
		Assumption:       "egg→pet association is user-specified unless verified by authoritative metadata; not inferred from two profile snapshots alone",
		ProbModel:        "stationary independent Bernoulli trials with p = observedRate; P(at least one in N) = 1-(1-p)^N",
		MethodID:         "observed-rate.v1+wilson95",
		OtherEggsChanged: otherEggsChanged,
		FutureEggs:       in.FutureEggs,
	}
	if in.EggsPurchasedDelta.ParseOK {
		out.EggsPurchasedDelta = in.EggsPurchasedDelta.Text
	}
	if in.PetHatchDelta.ParseOK {
		out.PetHatchDelta = in.PetHatchDelta.Text
	}
	if !in.EggsPurchasedDelta.ParseOK || !in.PetHatchDelta.ParseOK ||
		in.EggsPurchasedDelta.Int == nil || in.PetHatchDelta.Int == nil {
		out.SmallSampleWarning = "insufficient exact integer data for rate calculation"
		return out
	}
	trials := new(big.Int).Set(in.EggsPurchasedDelta.Int)
	succ := new(big.Int).Set(in.PetHatchDelta.Int)
	if trials.Sign() < 0 {
		out.SmallSampleWarning = "negative egg purchase delta; rate undefined"
		return out
	}
	if trials.Sign() == 0 {
		out.SmallSampleWarning = "zero egg purchase delta; rate undefined"
		return out
	}
	if succ.Sign() < 0 {
		out.SmallSampleWarning = "negative pet hatch delta; rate flagged, not treated as successes"
		return out
	}

	tf, _ := new(big.Float).SetInt(trials).Float64()
	sf, _ := new(big.Float).SetInt(succ).Float64()
	p := sf / tf
	out.ObservedRate = &p
	pct := p * 100
	out.ObservedPercent = &pct
	if succ.Sign() > 0 {
		avg := tf / sf
		out.AvgEggsPerSuccess = &avg
	} else {
		out.ZeroSuccessNote = "zero observed successes: does not establish a zero underlying probability; Wilson upper bound reported"
		out.AvgEggsPerSuccess = nil
	}
	e1k := p * 1000
	e10k := p * 10000
	out.ExpectedPer1000 = &e1k
	out.ExpectedPer10000 = &e10k

	lo, hi := wilsonScoreInterval(sf, tf, 1.959963984540054) // z≈1.96
	out.WilsonLow95 = &lo
	out.WilsonHigh95 = &hi

	if tf < 1000 {
		out.SmallSampleWarning = "small sample: observed rate is uncertain; confidence interval is wide"
	}

	if in.FutureEggs > 0 && p >= 0 && p <= 1 {
		prob := probAtLeastOne(p, float64(in.FutureEggs))
		out.ProbAtLeastOneFuture = &prob
	}
	return out
}

func wilsonScoreInterval(successes, trials, z float64) (low, high float64) {
	if trials <= 0 {
		return 0, 1
	}
	p := successes / trials
	z2 := z * z
	den := 1 + z2/trials
	center := p + z2/(2*trials)
	margin := z * math.Sqrt(p*(1-p)/trials+z2/(4*trials*trials))
	low = (center - margin) / den
	high = (center + margin) / den
	if low < 0 {
		low = 0
	}
	if high > 1 {
		high = 1
	}
	return low, high
}

func probAtLeastOne(p, n float64) float64 {
	if p <= 0 {
		return 0
	}
	if p >= 1 {
		return 1
	}
	// 1 - (1-p)^n = -expm1(n * log1p(-p))
	return -math.Expm1(n * math.Log1p(-p))
}
