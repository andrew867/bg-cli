package clan

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestRateContinuityDoesNotCrossABalanceDecrease(t *testing.T) {
	a := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	b := a.Add(15 * time.Minute)
	c := a.Add(30 * time.Minute)
	d := a.Add(time.Hour)
	pa := balPrior(a, "30000", "100", "1000", 1323)
	pb := balPrior(b, "2900", "120", "900", 1255)
	pc := balPrior(c, "5000", "140", "950", 1250)

	sumB := buildBalances(b, "2900", "120", "900", 1255, pa)
	spent := sumB.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if spent.Baseline != "30000" || spent.Current != "2900" || spent.Delta != "-27100" {
		t.Fatalf("spend delta: %+v", spent)
	}
	if spent.NetPerHour != "" || spent.NetPerMinute != "" || spent.RateBasis != "" || spent.Estimated {
		t.Fatalf("spend interval exposed a production rate: %+v", spent)
	}
	if spent.RateStatus != "reset" || spent.RateReason != "observed balance decrease" || spent.RateResetAt != b.Format(time.RFC3339) {
		t.Fatalf("spend reset: %+v", spent)
	}
	diamonds := sumB.Windows.Previous.ChangedCurrencies["Diamonds"]
	if diamonds.Delta != "20" || diamonds.NetPerHour == "" || diamonds.RateStatus != "" {
		t.Fatalf("unrelated currency reset: %+v", diamonds)
	}
	eggs := sumB.Windows.Previous.Eggs
	if eggs == nil || eggs.Delta != "-100" || eggs.NetPerHour != "" || eggs.RateStatus != "reset" || eggs.RateReason != "counter decreased" {
		t.Fatalf("egg counter: %+v", eggs)
	}
	if eggs.RateReason == "observed balance decrease" {
		t.Fatal("egg decrease was called spending")
	}
	place := sumB.Windows.Previous.Clan.Place
	if place == nil || place.Baseline != "1323" || place.Current != "1255" || place.Delta != "-68" || place.NetPerHour == "" {
		t.Fatalf("place was treated as a counter reset: %+v", place)
	}

	sumC := buildBalances(c, "5000", "140", "950", 1250, pa, pb)
	clean := sumC.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if clean.Baseline != "2900" || clean.Current != "5000" || clean.Delta != "2100" || clean.NetPerHour == "" || clean.RateStatus != "" {
		t.Fatalf("post-reset rate contaminated: %+v", clean)
	}
	if clean.NetPerHour == "-27100" || clean.Baseline == "30000" {
		t.Fatalf("post-reset rate used the pre-spend balance: %+v", clean)
	}
	crossed := sumC.Windows.Session.ChangedCurrencies["PumpkinTokens"]
	if crossed.Delta != "-25000" || crossed.NetPerHour != "" || crossed.NetPerMinute != "" || crossed.RateStatus != "unavailable" || crossed.RateReason != "balance decrease occurred within interval" {
		t.Fatalf("session window crossed the reset: %+v", crossed)
	}
	if crossed.RateResetAt != b.Format(time.RFC3339) {
		t.Fatalf("reset boundary: %+v", crossed)
	}
	eggClean := sumC.Windows.Previous.Eggs
	if eggClean == nil || eggClean.Delta != "50" || eggClean.NetPerHour == "" || eggClean.RateStatus != "" {
		t.Fatalf("egg recovery: %+v", eggClean)
	}
	eggCross := sumC.Windows.Session.Eggs
	if eggCross == nil || eggCross.Delta != "-50" || eggCross.NetPerHour != "" || eggCross.RateReason != "counter decreased within interval" {
		t.Fatalf("egg window crossed the reset: %+v", eggCross)
	}

	sumD := buildBalances(d, "31000", "200", "1100", 1240, pa, pb, pc)
	hour := sumD.Windows.Hour.ChangedCurrencies["PumpkinTokens"]
	if hour.Baseline != "30000" || hour.Current != "31000" || hour.Delta != "1000" {
		t.Fatalf("raw hour delta: %+v", hour)
	}
	if hour.NetPerHour != "" || hour.NetPerMinute != "" || hour.RateStatus != "unavailable" || hour.RateReason != "balance decrease occurred within interval" {
		t.Fatalf("hour rate crossed the decrease: %+v", hour)
	}
	prev := sumD.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if prev.Baseline != "5000" || prev.Delta != "26000" || prev.NetPerHour == "" || prev.RateStatus != "" {
		t.Fatalf("previous after reset: %+v", prev)
	}
	diaHour := sumD.Windows.Hour.ChangedCurrencies["Diamonds"]
	if diaHour.Delta != "100" || diaHour.NetPerHour == "" || diaHour.RateStatus != "" {
		t.Fatalf("diamond hour rate was reset with tokens: %+v", diaHour)
	}
}

func TestFlatBalanceAcrossADecreaseIsNotAZeroRate(t *testing.T) {
	a := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	b := a.Add(15 * time.Minute)
	c := a.Add(30 * time.Minute)
	pa := balPrior(a, "30000", "10", "10", 4)
	pb := balPrior(b, "2900", "10", "10", 4)
	sum := buildBalances(c, "30000", "10", "11", 4, pa, pb)
	crossed := sum.Windows.Session.ChangedCurrencies["PumpkinTokens"]
	if crossed.Baseline != "30000" || crossed.Current != "30000" || crossed.Delta != "0" || crossed.NetPerHour != "" || crossed.NetPerMinute != "" {
		t.Fatalf("flat window kept a rate: %+v", crossed)
	}
	if crossed.RateStatus != "unavailable" || crossed.RateReason != "balance decrease occurred within interval" {
		t.Fatalf("flat window reason: %+v", crossed)
	}
	clean := sum.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if clean.Delta != "27100" || clean.NetPerHour == "" || clean.RateStatus != "" {
		t.Fatalf("recovery from the reset baseline: %+v", clean)
	}
}

func TestZeroBalanceIsARealZero(t *testing.T) {
	now := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	prior := balPrior(now.Add(-time.Hour), "1000", "1000", "10", 4)
	sum := buildBalances(now, "1000", "1000", "10", 4, prior)
	if _, ok := sum.Windows.Previous.ChangedCurrencies["PumpkinTokens"]; ok {
		t.Fatal("unchanged currency was marked changed")
	}
	if _, ok := sum.Windows.Previous.ChangedCurrencies["Diamonds"]; ok {
		t.Fatal("unchanged diamonds were marked unavailable")
	}
	eggs := sum.Windows.Previous.Eggs
	if eggs == nil || eggs.Delta != "0" || eggs.NetPerHour != "0" || eggs.NetPerMinute != "0" || eggs.RateStatus != "" || eggs.Status == "unavailable" {
		t.Fatalf("zero eggs: %+v", eggs)
	}
}

func TestHiddenSpendInsideNetGrowthIsNotDetected(t *testing.T) {
	now := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	prior := balPrior(now.Add(-time.Hour), "20000", "1", "10", 4)
	sum := buildBalances(now, "25000", "2", "11", 4, prior)
	got := sum.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if got.Delta != "5000" || got.NetPerHour == "" || got.RateStatus != "" || got.RateReason != "" {
		t.Fatalf("net growth treated as spending: %+v", got)
	}
}

func TestMissingCurrencyObservationDoesNotInventSpend(t *testing.T) {
	a := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	b := a.Add(15 * time.Minute)
	c := a.Add(30 * time.Minute)
	pa := balPrior(a, "1000", "5", "10", 4)
	pb := balPrior(b, "1000", "5", "10", 4)
	delete(pb.Currencies, "PumpkinTokens")
	sum := buildBalances(c, "1100", "6", "12", 4, pa, pb)
	prev := sum.Windows.Previous.ChangedCurrencies["PumpkinTokens"]
	if prev.Status != "unavailable" || prev.Reason != "newly observed" || prev.Delta != "" || prev.RateReason == "observed balance decrease" {
		t.Fatalf("reappearance invented a spend: %+v", prev)
	}
	crossed := sum.Windows.Session.ChangedCurrencies["PumpkinTokens"]
	if crossed.Delta != "100" || crossed.NetPerHour != "" || crossed.RateStatus != "unavailable" || crossed.RateReason != "rate continuity unavailable across missing observation" {
		t.Fatalf("rate crossed the gap: %+v", crossed)
	}
	if crossed.RateReason == "observed balance decrease" {
		t.Fatal("missing observation was called a balance decrease")
	}
}

func TestMemberCapacityIsNotInvented(t *testing.T) {
	now := time.Date(2026, 11, 19, 3, 0, 0, 0, time.UTC)
	got := Observe(now, "1", "QNE", json.RawMessage(`{"Name":"QNE","Members":[{},{}]}`), json.RawMessage(`{}`), true, true)
	if got.MemberCapacity != nil {
		t.Fatalf("invented capacity: %+v", got.MemberCapacity)
	}
	if got.MemberCount == nil || *got.MemberCount != 2 {
		t.Fatalf("members: %+v", got.MemberCount)
	}
	got = Observe(now, "1", "QNE", json.RawMessage(`{"Name":"QNE","Members":[{}],"MemberCapacity":50}`), json.RawMessage(`{}`), true, true)
	if got.MemberCount == nil || *got.MemberCount != 1 || got.MemberCapacity == nil || *got.MemberCapacity != 50 {
		t.Fatalf("capacity: count=%v cap=%v", got.MemberCount, got.MemberCapacity)
	}
}

func balPrior(at time.Time, tokens, diamonds, eggs string, place int64) Prior {
	return Prior{
		File:            at.Format(time.RFC3339),
		ExportedAt:      at,
		ExportedAtText:  at.Format(time.RFC3339),
		SessionID:       "s",
		Profile:         "main",
		ClanTag:         "QNE",
		BattleID:        "CozyBattle2026",
		Points:          "1000",
		PointsOK:        true,
		Place:           strconv.FormatInt(place, 10),
		PlaceOK:         place > 0,
		UserPoints:      "100",
		UserPointsOK:    true,
		CurrenciesKnown: true,
		Currencies: map[string]string{
			"PumpkinTokens": tokens,
			"Diamonds":      diamonds,
		},
		StatsKnown: true,
		Stats:      map[string]string{"Eggs Opened": eggs},
	}
}

func buildBalances(now time.Time, tokens, diamonds, eggs string, place int64, priors ...Prior) Summary {
	p := place
	return Build(Input{
		Now:             now,
		LocalStamp:      "s",
		Prefix:          "ps99",
		SessionID:       "s",
		Profile:         "main",
		CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{
			"PumpkinTokens": {Amount: tokens, DisplayName: "Pumpkin Tokens"},
			"Diamonds":      {Amount: diamonds, DisplayName: "Diamonds"},
		},
		StatisticsKnown: true,
		Statistics:      map[string]string{"Eggs Opened": eggs},
		Clan: ClanCurrent{
			Tag: "QNE",
			ActiveBattle: &BattleCurrent{
				ID:               "CozyBattle2026",
				Points:           "1000",
				Place:            &p,
				UserContribution: "100",
			},
		},
		Priors: priors,
	})
}
