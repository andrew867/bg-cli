package accountanalytics

import (
	"fmt"
	"math/big"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

const MethodIDCurrencyEcon = "accountanalytics.currency-economics.v1"

// CurrencyRunway models an event currency spend interval with explicit residuals.
type CurrencyRunway struct {
	MethodID            string   `json:"methodId"`
	CurrencyID          string   `json:"currencyId"`
	StartingBalance     string   `json:"startingBalance,omitempty"`
	EndingBalance       string   `json:"endingBalance,omitempty"`
	ObservedDelta       string   `json:"observedDelta,omitempty"`
	Purchases           *int64   `json:"purchases,omitempty"`
	EggsPerPurchase     *int     `json:"eggsPerPurchase,omitempty"`
	EggsHatched         *int64   `json:"eggsHatched,omitempty"`
	DisplayedUnitPrice  string   `json:"displayedUnitPriceRaw,omitempty"`
	EstimatedGrossSpend string   `json:"estimatedGrossSpend,omitempty"`
	ImpliedIncome       string   `json:"impliedIncomeOrAdjustments,omitempty"`
	ResidualNote        string   `json:"residualNote"`
	Warnings            []string `json:"warnings"`
	Assumptions         []string `json:"assumptions"`
}

// AnalyzeCurrencyRunway reconciles start + income - spend ≈ end with explicit residual.
// displayedPrice is treated as an approximation (e.g. "~720M").
func AnalyzeCurrencyRunway(
	currencyID string,
	start, end snapshot.ExactInt,
	purchases int64,
	eggsPerPurchase int,
	eggsHatched int64,
	displayedUnitPrice string,
	unitPriceExact *big.Int, // nil if only rounded display available
) CurrencyRunway {
	rep := CurrencyRunway{
		MethodID: MethodIDCurrencyEcon, CurrencyID: currencyID,
		DisplayedUnitPrice: displayedUnitPrice,
		ResidualNote:       "ending = start + income - spend + adjustments; residual must stay explicit",
		Assumptions: []string{
			"rounded displayed egg prices are approximations, not exact ledger entries",
			"do not invent why auto-hatch stopped when stop time is missing",
		},
		Warnings: []string{},
	}
	if start.ParseOK {
		rep.StartingBalance = start.Text
	}
	if end.ParseOK {
		rep.EndingBalance = end.Text
	}
	if start.ParseOK && end.ParseOK && start.Int != nil && end.Int != nil {
		d, _ := snapshot.SubExact(end, start)
		rep.ObservedDelta = d.Text
	}
	if purchases > 0 {
		rep.Purchases = &purchases
	}
	if eggsPerPurchase > 0 {
		rep.EggsPerPurchase = &eggsPerPurchase
	}
	if eggsHatched > 0 {
		rep.EggsHatched = &eggsHatched
	}
	if unitPriceExact != nil && purchases > 0 {
		spend := new(big.Int).Mul(unitPriceExact, big.NewInt(purchases))
		rep.EstimatedGrossSpend = spend.String()
		if start.ParseOK && end.ParseOK && start.Int != nil && end.Int != nil {
			// implied income = end - start + spend
			impl := new(big.Int).Sub(end.Int, start.Int)
			impl.Add(impl, spend)
			rep.ImpliedIncome = impl.String()
		}
	} else if displayedUnitPrice != "" {
		rep.Warnings = append(rep.Warnings, "unit price only available as rounded display text — estimatedGrossSpend omitted rather than fabricated")
	}
	if eggsPerPurchase > 0 && purchases > 0 && eggsHatched > 0 {
		expect := purchases * int64(eggsPerPurchase)
		if expect != eggsHatched {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf(
				"purchases×eggsPerPurchase=%d ≠ eggsHatched=%d — investigate EggHatchCount vs observed pull size",
				expect, eggsHatched,
			))
		}
	}
	return rep
}
