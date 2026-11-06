package forge

import (
	"encoding/json"
	"math"
	"os"
	"sort"
)

const LedgerSchemaVersion = "1.0"

// Attempt is one executed Forge attempt with local ledger provenance.
type Attempt struct {
	AttemptID                 string               `json:"attemptId"`
	Timestamp                 string               `json:"timestamp"`
	Recipe                    []PetInput           `json:"recipe"`
	SpiritShardsConsumed      *int                 `json:"spiritShardsConsumed,omitempty"`
	DisplayedWinProbability   DisplayedProbability `json:"displayedWinProbability"`
	RewardProbabilities       []RewardProbability  `json:"rewardProbabilities,omitempty"`
	ActualOutcomeRewardID     string               `json:"actualOutcomeRewardId,omitempty"`
	ReturnedShards            *int                 `json:"returnedShards,omitempty"`
	MaterialsActuallyConsumed []PetInput           `json:"materialsActuallyConsumed,omitempty"`
	EventID                   string               `json:"eventId,omitempty"`
	ServerContext             string               `json:"serverContext,omitempty"`
	SnapshotBefore            string               `json:"snapshotBeforeManifest,omitempty"`
	SnapshotAfter             string               `json:"snapshotAfterManifest,omitempty"`
	Notes                     string               `json:"notes,omitempty"`
	Success                   *bool                `json:"success,omitempty"` // nil unknown
}

// LedgerFile stores local forge attempts — no telemetry.
type LedgerFile struct {
	SchemaVersion string    `json:"schemaVersion"`
	Attempts      []Attempt `json:"attempts"`
}

// LoadLedger reads a local ledger file.
func LoadLedger(path string) (LedgerFile, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return LedgerFile{}, err
	}
	var f LedgerFile
	if err := json.Unmarshal(b, &f); err != nil {
		return LedgerFile{}, err
	}
	if f.SchemaVersion == "" {
		f.SchemaVersion = LedgerSchemaVersion
	}
	sort.SliceStable(f.Attempts, func(i, j int) bool {
		return f.Attempts[i].Timestamp < f.Attempts[j].Timestamp
	})
	return f, nil
}

// OutcomeFrequency summarizes observed successes — not a validation of displayed p.
type OutcomeFrequency struct {
	NAttempts    int      `json:"nAttempts"`
	NSuccess     int      `json:"nSuccess"`
	NUnknown     int      `json:"nUnknown"`
	ObservedRate *float64 `json:"observedSuccessRate,omitempty"`
	WilsonLow95  *float64 `json:"wilsonLow95,omitempty"`
	WilsonHigh95 *float64 `json:"wilsonHigh95,omitempty"`
	Notes        []string `json:"notes"`
}

// SummarizeLedger computes observed frequencies with Wilson interval when enough labeled attempts exist.
func SummarizeLedger(f LedgerFile) OutcomeFrequency {
	out := OutcomeFrequency{
		Notes: []string{
			"observed frequency does not validate a displayed probability after one attempt",
			"inventory snapshot deltas can support but not always prove a single forge caused every change",
		},
	}
	for _, a := range f.Attempts {
		out.NAttempts++
		if a.Success == nil {
			out.NUnknown++
			continue
		}
		if *a.Success {
			out.NSuccess++
		}
	}
	labeled := out.NAttempts - out.NUnknown
	if labeled <= 0 {
		out.Notes = append(out.Notes, "no labeled success/failure attempts")
		return out
	}
	rate := float64(out.NSuccess) / float64(labeled)
	out.ObservedRate = &rate
	lo, hi := wilsonScore(float64(out.NSuccess), float64(labeled), 1.959963984540054)
	out.WilsonLow95 = &lo
	out.WilsonHigh95 = &hi
	return out
}

func wilsonScore(successes, trials, z float64) (low, high float64) {
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
