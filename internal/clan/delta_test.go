package clan

import (
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestPositiveNegativeUnchangedAndMultipleCurrencies(t *testing.T) {
	now := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	baseAt := now.Add(-30 * time.Minute)
	prior := Prior{
		File:            "2026-11-18-143000-ps99-summary.json",
		ExportedAt:      baseAt,
		ExportedAtText:  baseAt.Format(time.RFC3339),
		SessionID:       "ps99-session",
		BattleID:        "CozyBattle2026",
		Points:          "1000",
		PointsOK:        true,
		UserPoints:      "100",
		UserPointsOK:    true,
		CurrenciesKnown: true,
		Currencies: map[string]string{
			"PumpkinTokens":      "4500",
			"PumpkinTycoonCoins": "970000000000",
			"Diamonds":           "10",
			"Spent":              "80",
		},
	}
	sum := Build(Input{
		Now:             now,
		LocalStamp:      "2026-11-18-150000",
		Prefix:          "ps99",
		SessionID:       "ps99-session",
		CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{
			"PumpkinTokens":      {Amount: "6009", DisplayName: "Pumpkin Tokens"},
			"PumpkinTycoonCoins": {Amount: "960976550608", DisplayName: "Pumpkin Tycoon Coins"},
			"Diamonds":           {Amount: "10", DisplayName: "Diamonds"},
			"Spent":              {Amount: "20", DisplayName: "Spent"},
		},
		Clan:   clanWith("CozyBattle2026", "2000", "160"),
		Priors: []Prior{prior},
	})
	w := sum.Windows.Thirty
	if !w.Valid {
		t.Fatalf("30m invalid: %s", w.Reason)
	}
	if w.BaselineAt != prior.ExportedAtText || w.BaselineFile != prior.File {
		t.Fatalf("baseline = %s %s", w.BaselineAt, w.BaselineFile)
	}
	if w.Clan == nil || w.Clan.Points == nil || w.Clan.Points.Baseline != "1000" || w.Clan.Points.Current != "2000" {
		t.Fatalf("clan baseline not shared: %+v", w.Clan)
	}
	tok := w.ChangedCurrencies["PumpkinTokens"]
	if tok.Baseline != "4500" || tok.Current != "6009" || tok.Delta != "1509" {
		t.Fatalf("positive delta: %+v", tok)
	}
	if tok.NetPerHour != "3018" {
		t.Fatalf("netPerHour=%s want 3018", tok.NetPerHour)
	}
	if tok.RateBasis != CurrencyRateBasis || !tok.Estimated {
		t.Fatalf("currency rate was not marked as an observation estimate: %+v", tok)
	}
	spent := w.ChangedCurrencies["Spent"]
	if spent.Delta != "-60" || spent.NetPerHour != "-120" {
		t.Fatalf("negative delta: %+v", spent)
	}
	if _, ok := w.ChangedCurrencies["Diamonds"]; ok {
		t.Fatal("unchanged Diamonds must be omitted")
	}
	if len(w.ChangedCurrencies) != 3 {
		t.Fatalf("changed=%d %+v", len(w.ChangedCurrencies), w.ChangedCurrencies)
	}
	tycoon := w.ChangedCurrencies["PumpkinTycoonCoins"]
	if tycoon.Delta == "" || tycoon.Delta[0] != '-' {
		t.Fatalf("tycoon spend not negative: %+v", tycoon)
	}
	if w.Clan.Points.NetPerHour == "" || w.Clan.UserContribution.Baseline != "100" {
		t.Fatalf("clan rates missing: %+v", w.Clan)
	}
	text := FormatConsole(sum)
	if contains(text, "Diamonds") {
		t.Fatalf("console listed unchanged currency:\n%s", text)
	}
	if !contains(text, "PumpkinTokens") || !contains(text, "Spent") {
		t.Fatalf("console missing changed currencies:\n%s", text)
	}
	if strings.Index(text, "Previous:") > strings.Index(text, "Session:") || strings.Index(text, "Previous:") < 0 {
		t.Fatalf("console printed Session before Previous:\n%s", text)
	}
}

func TestVeryLargeCurrencyBeyond64Bit(t *testing.T) {
	huge := "12345678901234567890123456789012345678901234567890"
	plus := new(big.Int)
	plus.SetString(huge, 10)
	plus.Add(plus, big.NewInt(1))
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	baseAt := now.Add(-time.Hour)
	sum := Build(Input{
		Now: now, LocalStamp: "s", Prefix: "ps99", SessionID: "s",
		CurrenciesKnown: true,
		Currencies:      map[string]CurrencyAmount{"Huge": {Amount: plus.String(), DisplayName: "Huge"}},
		Priors: []Prior{{
			File: "a-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
			SessionID: "s", CurrenciesKnown: true, Currencies: map[string]string{"Huge": huge},
		}},
	})
	got := sum.Windows.Hour.ChangedCurrencies["Huge"]
	if got.Delta != "1" || got.NetPerHour != "1" {
		t.Fatalf("large delta=%+v", got)
	}
	n, ok := ParseExactInt("2.8132764485372915e+39")
	if !ok {
		t.Fatal("scientific integer rejected")
	}
	if n.String() != "2813276448537291500000000000000000000000" {
		t.Fatalf("exact=%s", n.String())
	}
	f, _ := strconv.ParseFloat("2.8132764485372915e+39", 64)
	floated := strconv.FormatFloat(f, 'f', 0, 64)
	if floated == n.String() {
		t.Fatal("exact parse matched float64")
	}
	if _, ok := ParseExactInt("2.5"); ok {
		t.Fatal("fraction accepted")
	}
	exact, approx, ok := NetPerHourParts("1", 7)
	if !ok || exact != "3600/7" || approx != "514.285714" {
		t.Fatalf("fractional rate exact=%s approx=%s", exact, approx)
	}
}

func TestSharedBaselineAndIrregularElapsed(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	baseAt := now.Add(-330 * time.Second)
	prior := Prior{
		File: "b-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
		SessionID: "sess", BattleID: "ActiveBattle", Points: "1000", PointsOK: true,
		UserPoints: "10", UserPointsOK: true,
		CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "100"},
	}
	sum := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "430", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanWith("ActiveBattle", "1330", "340"),
		Priors:     []Prior{prior},
	})
	w := sum.Windows.Five
	if !w.Valid || w.ElapsedSeconds == nil || *w.ElapsedSeconds != 330 {
		t.Fatalf("elapsed=%v valid=%v reason=%s", w.ElapsedSeconds, w.Valid, w.Reason)
	}
	if w.ChangedCurrencies["PumpkinTokens"].NetPerHour != "3600" {
		t.Fatalf("rate used nominal window: %+v", w.ChangedCurrencies["PumpkinTokens"])
	}
	if w.Clan.Points.NetPerHour != "3600" || w.BaselineFile != prior.File {
		t.Fatalf("clan did not share baseline: %+v", w.Clan)
	}
	if w.Clan.Points.Baseline != prior.Points || w.ChangedCurrencies["PumpkinTokens"].Baseline != "100" {
		t.Fatal("metrics came from different baselines")
	}
}

func TestMissingFiveMinuteAndSparseAndFirstSnapshot(t *testing.T) {
	now := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	first := Build(Input{Now: now, Prefix: "ps99", LocalStamp: "2026-11-18-180000", CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{}})
	for _, w := range []Window{first.Windows.Five, first.Windows.Thirty, first.Windows.Hour, first.Windows.Session, first.Windows.Previous} {
		if w.Valid || !contains(w.Reason, "no earlier snapshot") {
			t.Fatalf("first snapshot window valid=%v reason=%s", w.Valid, w.Reason)
		}
	}
	if first.SessionID != "ps99-2026-11-18-180000" {
		t.Fatalf("session=%s", first.SessionID)
	}

	sparseAt := now.Add(-18 * time.Minute)
	sparse := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "2", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanWith("B", "2", "2"),
		Priors: []Prior{{
			File: "old-ps99-summary.json", ExportedAt: sparseAt, ExportedAtText: sparseAt.Format(time.RFC3339),
			SessionID: "sess", BattleID: "B", Points: "1", PointsOK: true, UserPoints: "1", UserPointsOK: true,
			CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "1"},
		}},
	})
	if sparse.Windows.Five.Valid {
		t.Fatal("18 minute sample was treated as a 5 minute measurement")
	}
	if !contains(sparse.Windows.Five.Reason, "tolerance") {
		t.Fatalf("reason=%s", sparse.Windows.Five.Reason)
	}
	if sparse.Windows.Five.ChangedCurrencies != nil {
		t.Fatal("invalid 5m window included currency rates")
	}
	if !sparse.Windows.Session.Valid || sparse.Windows.Session.BaselineFile != "old-ps99-summary.json" {
		t.Fatalf("session=%+v", sparse.Windows.Session)
	}
	if sparse.Windows.Thirty.Valid || sparse.Windows.Hour.Valid {
		t.Fatal("sparse history validated a long nominal window")
	}
}

func TestWholeSessionBaselineIsSharedAndStable(t *testing.T) {
	now := time.Date(2026, 11, 18, 20, 0, 0, 0, time.UTC)
	early := now.Add(-2 * time.Hour)
	mid := now.Add(-5 * time.Minute)
	other := now.Add(-5 * time.Minute)
	sum := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess-a", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "50", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanWith("Battle", "50", "5"),
		Priors: []Prior{
			{
				File: "mid-ps99-summary.json", ExportedAt: mid, ExportedAtText: mid.Format(time.RFC3339),
				SessionID: "sess-a", BattleID: "Battle", Points: "40", PointsOK: true, UserPoints: "4", UserPointsOK: true,
				CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "40"},
			},
			{
				File: "early-ps99-summary.json", ExportedAt: early, ExportedAtText: early.Format(time.RFC3339),
				SessionID: "sess-a", BattleID: "Battle", Points: "10", PointsOK: true, UserPoints: "1", UserPointsOK: true,
				CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "10"},
			},
			{
				File: "other-ps99-summary.json", ExportedAt: other, ExportedAtText: other.Format(time.RFC3339),
				SessionID: "sess-b", BattleID: "Battle", Points: "9999", PointsOK: true, UserPoints: "9999", UserPointsOK: true,
				CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "9999"},
			},
		},
	})
	sess := sum.Windows.Session
	if !sess.Valid || sess.BaselineFile != "early-ps99-summary.json" {
		t.Fatalf("session baseline=%s", sess.BaselineFile)
	}
	if sess.ChangedCurrencies["PumpkinTokens"].Baseline != "10" || sess.Clan.Points.Baseline != "10" {
		t.Fatal("session currency and clan baselines diverged")
	}
	if sum.Windows.Five.BaselineFile != "mid-ps99-summary.json" {
		t.Fatalf("5m used %s", sum.Windows.Five.BaselineFile)
	}
	if sum.Windows.Previous.BaselineFile != "mid-ps99-summary.json" || sum.Windows.Previous.ChangedCurrencies["PumpkinTokens"].Baseline != "40" || sum.Windows.Previous.Clan.Points.Baseline != "40" {
		t.Fatalf("previous did not use the latest shared baseline: %+v", sum.Windows.Previous)
	}
	if sum.Windows.Five.ChangedCurrencies["PumpkinTokens"].Baseline == "9999" {
		t.Fatal("5m crossed sessions")
	}
}

func TestBattleIDChangeBlocksClanDeltaOnly(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	baseAt := now.Add(-5 * time.Minute)
	sum := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "8", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanWith("BattleB", "90", "9"),
		Priors: []Prior{{
			File: "a-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
			SessionID: "sess", BattleID: "BattleA", Points: "10", PointsOK: true, UserPoints: "1", UserPointsOK: true,
			CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "3"},
		}},
	})
	w := sum.Windows.Five
	if !w.Valid {
		t.Fatal(w.Reason)
	}
	if w.ChangedCurrencies["PumpkinTokens"].Delta != "5" {
		t.Fatalf("currency delta blocked: %+v", w.ChangedCurrencies["PumpkinTokens"])
	}
	if w.Clan == nil || w.Clan.Status != "unavailable" || !contains(w.Clan.Reason, "differs") {
		t.Fatalf("clan=%+v", w.Clan)
	}
	if w.Clan.Points != nil {
		t.Fatal("cross-battle points were calculated")
	}
}

func TestCurrencyPresentOnOneEndpointAndExplicitZero(t *testing.T) {
	now := time.Date(2026, 11, 18, 12, 0, 0, 0, time.UTC)
	baseAt := now.Add(-5 * time.Minute)
	sum := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{
			"NewCoin": {Amount: "4", DisplayName: "NewCoin"},
			"Zeroed":  {Amount: "0", DisplayName: "Zeroed"},
		},
		Priors: []Prior{{
			File: "a-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
			SessionID: "sess", CurrenciesKnown: true,
			Currencies: map[string]string{
				"OldCoin": "7",
				"Zeroed":  "5",
			},
		}},
	})
	w := sum.Windows.Five
	if w.ChangedCurrencies["NewCoin"].Reason != "newly observed" || w.ChangedCurrencies["NewCoin"].Delta != "" {
		t.Fatalf("new=%+v", w.ChangedCurrencies["NewCoin"])
	}
	if w.ChangedCurrencies["OldCoin"].Reason != "no longer returned" || w.ChangedCurrencies["OldCoin"].Delta != "" {
		t.Fatalf("old=%+v", w.ChangedCurrencies["OldCoin"])
	}
	if w.ChangedCurrencies["Zeroed"].Delta != "-5" {
		t.Fatalf("explicit zero was not a real amount: %+v", w.ChangedCurrencies["Zeroed"])
	}
}

func TestUserContributionRank(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	clanRaw := json.RawMessage(`{
		"Name":"Synthetic","Members":[{},{}],
		"Battles":{"ActiveBattle":{
			"BattleID":"ActiveBattle","Points":56704,"Place":1602,
			"PointContributions":[
				{"UserID":111,"Points":500},
				{"UserID":1001,"Points":428},
				{"UserID":333,"Points":428},
				{"UserID":222,"Points":100}
			]
		}}
	}`)
	battleRaw := json.RawMessage(`{"configName":"ActiveBattle","configData":{"Title":"Nine Eyed Clan Battle!","StartTime":` + unix(now.Add(-time.Hour)) + `,"FinishTime":` + unix(now.Add(time.Hour)) + `,"Enabled":null}}`)
	got := Observe(now, "1001", "SYN", clanRaw, battleRaw, true, true)
	if got.Tag != "SYN" || got.Name != "Synthetic" || got.MemberCount == nil || *got.MemberCount != 2 {
		t.Fatalf("identity=%+v", got)
	}
	if got.ContributorCount == nil || *got.ContributorCount != 4 {
		t.Fatalf("contributors=%v", got.ContributorCount)
	}
	b := got.ActiveBattle
	if b == nil || b.ID != "ActiveBattle" || b.Title != "Nine Eyed Clan Battle!" || b.Points != "56704" {
		t.Fatalf("battle=%+v", b)
	}
	if b.Place == nil || *b.Place != 1602 || b.UserContribution != "428" || b.UserContributorRank == nil || *b.UserContributorRank != 2 {
		t.Fatalf("rank=%+v", b)
	}
	anon := Observe(now, "", "SYN", clanRaw, battleRaw, true, true)
	if anon.ActiveBattle == nil || anon.ActiveBattle.Points != "56704" || anon.ActiveBattle.UserContribution != "" || anon.ActiveBattle.UserContributorRank != nil {
		t.Fatalf("unconfigured user received a contribution: %+v", anon.ActiveBattle)
	}
	if anon.ActiveBattle == nil || !contains(anon.ActiveBattle.Reason, "not configured") {
		t.Fatalf("missing unconfigured reason: %+v", anon.ActiveBattle)
	}
	off := Observe(now, "1001", "SYN", clanRaw, json.RawMessage(`{"configName":"ActiveBattle","configData":{"Title":"Done","StartTime":1,"FinishTime":2}}`), true, true)
	if off.ActiveBattle != nil {
		t.Fatal("finished battle reported as active")
	}
	disabled := Observe(now, "1001", "SYN", clanRaw, json.RawMessage(`{"configName":"ActiveBattle","configData":{"Title":"X","StartTime":1,"FinishTime":`+unix(now.Add(time.Hour))+`,"Enabled":false}}`), true, true)
	if disabled.ActiveBattle != nil || !contains(disabled.Reason, "not enabled") {
		t.Fatalf("disabled=%+v", disabled)
	}
}

func TestExtractTagIgnoresNestedNames(t *testing.T) {
	if _, _, ok := ExtractTag(json.RawMessage(`{"Pets":{"Guild":"Guilded Raven"}}`), nil); ok {
		t.Fatal("nested pet name used as a clan tag")
	}
	tag, field, ok := ExtractTag(json.RawMessage(`{"Clan":{"Name":"SYN"}}`), nil)
	if !ok || tag != "SYN" || field != "Clan" {
		t.Fatalf("tag=%s field=%s ok=%v", tag, field, ok)
	}
	if _, _, ok := ExtractTag(json.RawMessage(`{"Clan":"../x"}`), nil); ok {
		t.Fatal("unsafe tag accepted")
	}
}

func TestScanHistoryRules(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 18, 15, 0, 0, 0, time.UTC)
	writeSummary(t, dir, "2026-11-18-140000-ps99-summary.json", summaryDoc(now.Add(-time.Hour), "sess", "Battle", "10", "1", map[string]string{"PumpkinTokens": "10"}))
	writeSummary(t, dir, "2026-11-18-145900-ps99-summary.json", summaryDoc(now, "sess", "Battle", "99", "9", map[string]string{"PumpkinTokens": "99"}))
	writeSummary(t, dir, "2026-11-18-145500-ps99-summary.json", summaryDoc(now.Add(-5*time.Minute), "sess", "Battle", "20", "2", map[string]string{"PumpkinTokens": "20"}))
	writeSummary(t, dir, "dup-a-ps99-summary.json", summaryDoc(now.Add(-5*time.Minute), "sess", "Battle", "1", "1", map[string]string{"PumpkinTokens": "1"}))
	writeSummary(t, dir, "dup-z-ps99-summary.json", summaryDoc(now.Add(-5*time.Minute), "sess", "Battle", "20", "2", map[string]string{"PumpkinTokens": "20"}))
	if err := os.WriteFile(filepath.Join(dir, "bad-ps99-summary.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	writeSummary(t, dir, "future-ps99-summary.json", summaryDoc(now.Add(time.Hour), "sess", "Battle", "1000", "1000", map[string]string{"PumpkinTokens": "1000"}))
	if err := os.WriteFile(filepath.Join(dir, "2026-11-18-120000-ps99-currencies.csv"), []byte("key,id,_am,amount\nPumpkinTokens,PumpkinTokens,1,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	inv := filepath.Join(dir, "2026-11-18-120000-ps99-inv.json")
	if err := os.WriteFile(inv, make([]byte, 1<<20+10), 0o644); err != nil {
		t.Fatal(err)
	}
	scanned, err := Scan(dir, "ps99", now, "2026-11-18-150000-ps99-summary.json")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range scanned.Opened {
		if contains(name, "inv.json") || contains(name, "currencies.csv") {
			t.Fatalf("scan opened %s", name)
		}
	}
	warn := join(scanned.Warnings)
	if !contains(warn, "malformed") || !contains(warn, "future-dated") {
		t.Fatalf("warnings=%s", warn)
	}
	var five Prior
	found := false
	for _, p := range scanned.Priors {
		if p.ExportedAt.Equal(now.Add(-5 * time.Minute)) {
			five = p
			found = true
		}
		if !p.ExportedAt.Before(now) {
			t.Fatalf("kept non-prior %s", p.File)
		}
	}
	if !found || five.File != "dup-z-ps99-summary.json" || five.Currencies["PumpkinTokens"] != "20" {
		t.Fatalf("duplicate timestamp winner=%+v", five)
	}
	if scanned.SessionID != "sess" {
		t.Fatalf("session=%s", scanned.SessionID)
	}
}

func TestScanKeepsSameSecondSummariesFromDifferentAccounts(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	at := now.Add(-time.Minute)
	a := summaryDoc(at, "sess-a", "Battle", "10", "1", map[string]string{"PumpkinTokens": "10"})
	a.AccountID = "acct-a"
	a.Profile = "main"
	b := summaryDoc(at, "sess-b", "Battle", "99", "9", map[string]string{"PumpkinTokens": "99"})
	b.AccountID = "acct-b"
	b.Profile = "alt"
	writeSummary(t, dir, "a-ps99-summary.json", a)
	writeSummary(t, dir, "b-ps99-summary.json", b)
	scanned, err := Scan(dir, "ps99", now, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned.Priors) != 2 {
		t.Fatalf("same-second accounts collapsed: %+v", scanned.Priors)
	}
	got := map[string]string{}
	for _, p := range scanned.Priors {
		got[p.AccountID] = p.Currencies["PumpkinTokens"]
	}
	if got["acct-a"] != "10" || got["acct-b"] != "99" {
		t.Fatalf("kept=%v", got)
	}
}

func TestPreviousWindowUsesLatestRegardlessOfAge(t *testing.T) {
	now := time.Date(2026, 11, 18, 18, 0, 0, 0, time.UTC)
	early := now.Add(-3 * time.Hour)
	latest := now.Add(-40 * time.Minute)
	sum := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "30", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanWith("Battle", "30", "3"),
		Priors: []Prior{
			{
				File: "early-ps99-summary.json", ExportedAt: early, ExportedAtText: early.Format(time.RFC3339),
				SessionID: "sess", CurrenciesKnown: true, BattleID: "Battle", Points: "10", PointsOK: true,
				UserPoints: "1", UserPointsOK: true, Currencies: map[string]string{"PumpkinTokens": "10"},
			},
			{
				File: "latest-ps99-summary.json", ExportedAt: latest, ExportedAtText: latest.Format(time.RFC3339),
				SessionID: "sess", CurrenciesKnown: true, BattleID: "Battle", Points: "20", PointsOK: true,
				UserPoints: "2", UserPointsOK: true, Currencies: map[string]string{"PumpkinTokens": "20"},
			},
		},
	})
	if sum.Windows.Five.Valid || sum.Windows.Thirty.Valid || sum.Windows.Hour.Valid {
		t.Fatal("a 40 minute gap was treated as a nominal window")
	}
	prev := sum.Windows.Previous
	if !prev.Valid || prev.BaselineFile != "latest-ps99-summary.json" {
		t.Fatalf("previous=%+v", prev)
	}
	if prev.ChangedCurrencies["PumpkinTokens"].Baseline != "20" || prev.Clan == nil || prev.Clan.Points == nil || prev.Clan.Points.Baseline != "20" {
		t.Fatalf("previous did not share the latest baseline: %+v", prev)
	}
	if prev.ChangedCurrencies["PumpkinTokens"].RateBasis != CurrencyRateBasis || !prev.ChangedCurrencies["PumpkinTokens"].Estimated {
		t.Fatalf("currency rate unmarked: %+v", prev.ChangedCurrencies["PumpkinTokens"])
	}
	if sum.Windows.Session.BaselineFile != "early-ps99-summary.json" {
		t.Fatalf("session=%s", sum.Windows.Session.BaselineFile)
	}
}

func TestCurrenciesFromProfileStayExact(t *testing.T) {
	raw := json.RawMessage(`{"Currency":{"PumpkinTokens":{"id":"PumpkinTokens","_am":6009,"displayName":"Pumpkin Tokens"},"Huge":{"id":"Huge","_am":123456789012345678901234567890}}}`)
	got, known := CurrenciesFromProfile(raw)
	if !known || got["PumpkinTokens"].Amount != "6009" || got["PumpkinTokens"].DisplayName != "Pumpkin Tokens" {
		t.Fatalf("%+v", got)
	}
	if got["Huge"].Amount != "123456789012345678901234567890" || got["Huge"].DisplayName != "Huge" {
		t.Fatalf("huge=%+v", got["Huge"])
	}
	if _, known := CurrenciesFromProfile(json.RawMessage(`{"Rank":1}`)); known {
		t.Fatal("missing Currency field was treated as an empty list")
	}
}

func TestResolveUserID(t *testing.T) {
	id, ok := ResolveUserID("", "")
	if ok || id != "" {
		t.Fatalf("unconfigured id=%q ok=%v", id, ok)
	}
	id, ok = ResolveUserID("42", "7")
	if !ok || id != "42" {
		t.Fatalf("flag=%s", id)
	}
	id, ok = ResolveUserID("", "7")
	if !ok || id != "7" {
		t.Fatalf("profile=%s", id)
	}
}

func TestHistoryDoesNotCrossAccountsClansOrUsers(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	closeAt := now.Add(-5 * time.Minute)
	sameAt := now.Add(-6 * time.Minute)
	other := Prior{
		File: "other-ps99-summary.json", ExportedAt: closeAt, ExportedAtText: closeAt.Format(time.RFC3339),
		SessionID: "sess", AccountID: "acct-b", Profile: "alt", UserID: "222", ClanTag: "SYN",
		BattleID: "Battle", Points: "1", PointsOK: true, UserPoints: "1", UserPointsOK: true,
		CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "1"},
	}
	same := Prior{
		File: "same-ps99-summary.json", ExportedAt: sameAt, ExportedAtText: sameAt.Format(time.RFC3339),
		SessionID: "sess", AccountID: "acct-a", Profile: "main", UserID: "111", ClanTag: "SYN",
		BattleID: "Battle", Points: "1000", PointsOK: true, UserPoints: "10", UserPointsOK: true,
		CurrenciesKnown: true, Currencies: map[string]string{"PumpkinTokens": "10"},
	}
	current := Input{
		Now: now, Prefix: "ps99", SessionID: "sess", AccountID: "acct-a", Profile: "main",
		UserID: "111", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "20", DisplayName: "Pumpkin Tokens"}},
		Clan:       clanTagged("SYN", "Battle", "2000", "40"),
		Priors:     []Prior{other, same},
	}
	sum := Build(current)
	w := sum.Windows.Five
	if !w.Valid || w.BaselineFile != "same-ps99-summary.json" || w.ChangedCurrencies["PumpkinTokens"].Baseline != "10" || w.ChangedCurrencies["PumpkinTokens"].Delta != "10" {
		t.Fatalf("crossed accounts: file=%s tok=%+v", w.BaselineFile, w.ChangedCurrencies["PumpkinTokens"])
	}
	if w.Clan == nil || w.Clan.Points == nil || w.Clan.Points.Delta != "1000" || w.Clan.UserContribution == nil || w.Clan.UserContribution.Delta != "30" {
		t.Fatalf("same account continuation: %+v", w.Clan)
	}

	onlyOther := current
	onlyOther.Priors = []Prior{other}
	sum = Build(onlyOther)
	if sum.Windows.Five.Valid || sum.Windows.Five.ChangedCurrencies != nil || sum.Windows.Five.Clan != nil && sum.Windows.Five.Clan.Points != nil {
		t.Fatalf("other account became a baseline: %+v", sum.Windows.Five)
	}

	users := current
	users.UserID = "222"
	users.Clan = clanTagged("SYN", "Battle", "2000", "50")
	users.Priors = []Prior{same}
	sum = Build(users)
	w = sum.Windows.Five
	if w.ChangedCurrencies["PumpkinTokens"].Delta != "10" || w.Clan == nil || w.Clan.Points == nil || w.Clan.Points.Delta != "1000" {
		t.Fatalf("shared totals blocked: tok=%+v clan=%+v", w.ChangedCurrencies["PumpkinTokens"], w.Clan)
	}
	if w.Clan.UserContribution == nil || w.Clan.UserContribution.Status != "unavailable" || w.Clan.UserContribution.Delta != "" || !contains(w.Clan.UserContribution.Reason, "differs") {
		t.Fatalf("user delta leaked: %+v", w.Clan.UserContribution)
	}

	moved := current
	moved.Clan = clanTagged("OTHER", "Battle", "2000", "40")
	moved.Priors = []Prior{same}
	sum = Build(moved)
	w = sum.Windows.Five
	if w.ChangedCurrencies["PumpkinTokens"].Delta != "10" {
		t.Fatal("currency delta blocked by a clan change")
	}
	if w.Clan == nil || w.Clan.Status != "unavailable" || w.Clan.Points != nil || !contains(w.Clan.Reason, "clan tag") {
		t.Fatalf("clan change produced a delta: %+v", w.Clan)
	}
}

func TestMatchingSessionStaysOnTheSameAccount(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	priors := []Prior{
		{File: "same.json", ExportedAt: now.Add(-time.Hour), SessionID: "sess-a", AccountID: "acct-a", Profile: "main"},
		{File: "other.json", ExportedAt: now.Add(-time.Minute), SessionID: "sess-b", AccountID: "acct-b", Profile: "alt"},
	}
	if got := MatchingSession(priors, "acct-a", "main"); got != "sess-a" {
		t.Fatalf("session=%s", got)
	}
}

func TestCurrencyListValidity(t *testing.T) {
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	baseAt := now.Add(-5 * time.Minute)
	priorKnown := Prior{
		File: "known-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
		SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]string{"PumpkinTokens": "10"},
	}
	currentUnknown := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: false,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "99", DisplayName: "Pumpkin Tokens"}},
		Priors:     []Prior{priorKnown},
	})
	w := currentUnknown.Windows.Five
	if !w.Valid || w.CurrenciesStatus != "unavailable" || !contains(w.CurrenciesReason, "current") || w.ChangedCurrencies != nil {
		t.Fatalf("current unknown: %+v", w)
	}

	priorUnknown := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"PumpkinTokens": {Amount: "20", DisplayName: "Pumpkin Tokens"}},
		Priors: []Prior{{
			File: "failed-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
			SessionID: "sess", CurrenciesKnown: false,
			Currencies: map[string]string{"PumpkinTokens": "1"},
		}},
	})
	w = priorUnknown.Windows.Five
	if w.CurrenciesStatus != "unavailable" || !contains(w.CurrenciesReason, "baseline") || w.ChangedCurrencies != nil {
		t.Fatalf("prior unknown: %+v", w)
	}

	knownEmpty := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{},
		Priors:     []Prior{priorKnown},
	})
	w = knownEmpty.Windows.Five
	if w.CurrenciesStatus != "" || w.ChangedCurrencies["PumpkinTokens"].Reason != "no longer returned" || w.ChangedCurrencies["PumpkinTokens"].Delta != "" {
		t.Fatalf("known empty: %+v", w.ChangedCurrencies["PumpkinTokens"])
	}

	fresh := Build(Input{
		Now: now, Prefix: "ps99", SessionID: "sess", CurrenciesKnown: true,
		Currencies: map[string]CurrencyAmount{"Fresh": {Amount: "3", DisplayName: "Fresh"}},
		Priors: []Prior{{
			File: "empty-ps99-summary.json", ExportedAt: baseAt, ExportedAtText: baseAt.Format(time.RFC3339),
			SessionID: "sess", CurrenciesKnown: true, Currencies: map[string]string{},
		}},
	})
	if got := fresh.Windows.Five.ChangedCurrencies["Fresh"]; got.Reason != "newly observed" || got.Delta != "" {
		t.Fatalf("newly observed: %+v", got)
	}

	raw, err := json.Marshal(Build(Input{Now: now, Prefix: "ps99", LocalStamp: "s", CurrenciesKnown: true}))
	if err != nil || !contains(string(raw), `"currenciesKnown":true`) {
		t.Fatalf("known flag missing: %s", raw)
	}
	raw, err = json.Marshal(Build(Input{Now: now, Prefix: "ps99", LocalStamp: "s", CurrenciesKnown: false}))
	if err != nil || !contains(string(raw), `"currenciesKnown":false`) {
		t.Fatalf("unknown flag missing: %s", raw)
	}
}

func TestDecodeUnknownCurrencyList(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	doc := summaryDoc(now.Add(-5*time.Minute), "sess", "Battle", "10", "1", map[string]string{"PumpkinTokens": "10"})
	doc.Current.CurrenciesKnown = false
	writeSummary(t, dir, "old-ps99-summary.json", doc)
	scanned, err := Scan(dir, "ps99", now, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned.Priors) != 1 || scanned.Priors[0].CurrenciesKnown || len(scanned.Priors[0].Currencies) != 0 {
		t.Fatalf("unknown list decoded as currencies: %+v", scanned.Priors)
	}
}

func TestExtractAccountID(t *testing.T) {
	if got := ExtractAccountID(json.RawMessage(`{"UserID":1001,"Pets":{"UserID":9}}`)); got != "1001" {
		t.Fatalf("id=%s", got)
	}
	if got := ExtractAccountID(json.RawMessage(`{"Rank":12,"Currency":{"Diamonds":{"_am":1}}}`)); got != "" {
		t.Fatalf("rank used as account id: %s", got)
	}
}

func clanWith(id, points, user string) ClanCurrent {
	return ClanCurrent{ActiveBattle: &BattleCurrent{ID: id, Points: points, UserContribution: user}}
}

func clanTagged(tag, id, points, user string) ClanCurrent {
	c := clanWith(id, points, user)
	c.Tag = tag
	return c
}

func unix(t time.Time) string {
	return strconv.FormatInt(t.Unix(), 10)
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()))
}

func join(parts []string) string {
	out := ""
	for _, p := range parts {
		out += p + "\n"
	}
	return out
}

func writeSummary(t *testing.T, dir, name string, doc Summary) {
	t.Helper()
	body, err := json.Marshal(doc)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name), body, 0o644); err != nil {
		t.Fatal(err)
	}
}

func summaryDoc(at time.Time, session, battle, points, user string, curs map[string]string) Summary {
	cur := map[string]CurrencyAmount{}
	for id, amount := range curs {
		cur[id] = CurrencyAmount{Amount: amount, DisplayName: id}
	}
	return Summary{
		SchemaVersion: SchemaVersion,
		ExportedAt:    at.Format(time.RFC3339),
		SessionID:     session,
		Current: Current{
			CurrenciesKnown: true,
			Currencies:      cur,
			Clan:            clanWith(battle, points, user),
		},
	}
}
