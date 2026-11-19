package snapview

import (
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestRenderHumanSummary(t *testing.T) {
	elapsed := int64(5*3600 + 23*60 + 46)
	place := int64(1343)
	rank := int64(2)
	now := time.Date(2026, 11, 18, 20, 12, 34, 0, time.UTC)
	sum := clan.Summary{
		Prefix:     "ps99",
		Profile:    "main",
		UserID:     "4092648666",
		ExportedAt: now.Format(time.RFC3339),
		Current: clan.Current{
			CurrenciesKnown: true,
			Currencies: map[string]clan.CurrencyAmount{
				"Diamonds":           {Amount: "11492509", DisplayName: "Diamonds"},
				"PumpkinTokens":      {Amount: "13446", DisplayName: "Pumpkin Tokens"},
				"PumpkinTycoonCoins": {Amount: "1276000000000", DisplayName: "Pumpkin Tycoon Coins"},
				"OtherCoins":         {Amount: "5", DisplayName: "Other Coins"},
			},
			StatisticsKnown: true,
			Statistics: map[string]string{
				"Eggs Opened":      "13358852",
				"Huge Pets Opened": "2",
				"Playtime":         "1982100",
			},
			Clan: clan.ClanCurrent{
				Tag:  "QNE",
				Name: "QNE",
				ActiveBattle: &clan.BattleCurrent{
					ID:                  "battle",
					Title:               "Live",
					Place:               &place,
					Points:              "76048",
					UserContribution:    "5091",
					UserContributorRank: &rank,
				},
			},
		},
		Windows: clan.Windows{
			Session: clan.Window{Valid: true, ElapsedSeconds: &elapsed, CurrentAt: now.Format(time.RFC3339)},
			Previous: clan.Window{
				Valid:          true,
				BaselineFile:   "prior-summary.json",
				ElapsedSeconds: &elapsed,
				ChangedCurrencies: map[string]clan.CurrencyDelta{
					"Diamonds":           {Current: "11492509", Delta: "161506"},
					"PumpkinTokens":      {Current: "13446", Delta: "8236"},
					"PumpkinTycoonCoins": {Current: "1276000000000", Delta: "136415000000"},
				},
				Clan: &clan.ClanWindow{
					Points:           &clan.MetricDelta{Delta: "6749"},
					UserContribution: &clan.MetricDelta{Delta: "2027"},
				},
			},
		},
	}
	text := Render(View{
		Summary: sum,
		Priors: []clan.Prior{{
			File:       "prior-summary.json",
			StatsKnown: true,
			Stats:      map[string]string{"Eggs Opened": "13159768", "Huge Pets Opened": "2", "Playtime": "1982100"},
		}},
		Manifest: snapshot.Manifest{
			ManifestFile: "2026-11-18-201234-ps99-manifest.json",
			Summary: map[string]any{
				"inventory": map[string]any{"petQuantitySum": 10963991, "rowCount": 1857},
			},
			Files: []snapshot.FileRecord{
				{Name: "2026-11-18-201234-ps99-inv.json", Kind: "inventory-json", View: "inventory", OK: true},
				{Name: "2026-11-18-201234-ps99-prof.json", Kind: "profile-json", View: "profile", OK: true},
				{Name: "2026-11-18-201234-ps99-extprof.json", Kind: "extended-profile-json", View: "extendedProfile", OK: true},
				{Name: "2026-11-18-201234-ps99-clan.json", Kind: "clan-json", OK: true},
				{Name: "2026-11-18-201234-ps99-clanbattle.json", Kind: "clan-battle-json", OK: true},
			},
		},
		Bundle: &Bundle{
			OK: true, Complete: true, Name: "2026-11-18-201234-ps99.zip",
			Dir: "/tmp/snaps", Bytes: 1200, SHA256: "0123456789abcdef0123456789abcdef", Members: 6,
		},
	})
	for _, want := range []string{
		"PS99 snapshot  2026-11-18 20:12:34",
		"Profile        main",
		"User           4092648666",
		"Session        5h 23m 46s",
		"Since previous",
		"Diamonds",
		"+161,506",
		"Pumpkin Tokens",
		"1.276T",
		"+136.415B",
		"Eggs opened",
		"Hatch rate",
		"~",
		"Playtime",
		"550h 35m",
		"Clan QNE",
		"#1,343",
		"76,048",
		"+6,749",
		"Contributor rank",
		"#2",
		"Pet quantity",
		"10,963,991",
		"Inventory",
		"OK",
		"2026-11-18-201234-ps99.zip",
		"Directory",
		"snaps",
		"sha256",
		"0123456789ab",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
	}
	eggHour := FormatRatePerHour("199084", elapsed)
	eggMinute := FormatRatePerMinute("199084", elapsed)
	if strings.Count(text, eggHour) < 2 || strings.Count(text, eggMinute) < 2 {
		t.Fatalf("egg line and hatch rate diverged: %s %s\n%s", eggHour, eggMinute, text)
	}
	diamond := "+161,506    " + FormatRatePerHour("161506", elapsed) + "    " + FormatRatePerMinute("161506", elapsed)
	if !strings.Contains(text, diamond) {
		t.Fatalf("diamond rates missing %q\n%s", diamond, text)
	}
	if strings.Contains(text, "Other Coins") {
		t.Fatalf("unchanged non-preferred currency leaked:\n%s", text)
	}
	if strings.Contains(text, "\x1b") {
		t.Fatal("color leaked into plain output")
	}
	all := Render(View{Summary: sum, ShowAllCurrencies: true})
	if !strings.Contains(all, "Other Coins") {
		t.Fatalf("show all omitted a currency:\n%s", all)
	}
}

func TestRenderSessionClanWarningAndNoColorDependence(t *testing.T) {
	sum := clan.Summary{
		Prefix:     "ps99",
		ExportedAt: "2026-11-18T20:12:34Z",
		Current: clan.Current{
			Clan: clan.ClanCurrent{Tag: "QNE", ActiveBattle: &clan.BattleCurrent{ID: "b", Points: "10"}},
		},
		Windows: clan.Windows{
			Session: clan.Window{
				Valid: true,
				Clan:  &clan.ClanWindow{Status: "unavailable", Reason: "baseline has no active battle"},
			},
			Previous: clan.Window{Reason: "no earlier snapshot in this session"},
		},
		Warnings: []string{"player user id is not configured; contribution and rank omitted"},
	}
	plain := Render(View{Summary: sum, Color: false})
	color := Render(View{Summary: sum, Color: true})
	if !strings.Contains(plain, "Session clan delta unavailable because the session baseline predates clan metadata.") {
		t.Fatalf("warning missing:\n%s", plain)
	}
	if strings.Contains(plain, "\x1b") {
		t.Fatal("plain render used color")
	}
	if !strings.Contains(color, "\x1b[33m") {
		t.Fatal("color render did not mark the warning state")
	}
	strip := strings.ReplaceAll(color, "\x1b[33m", "")
	strip = strings.ReplaceAll(strip, "\x1b[32m", "")
	strip = strings.ReplaceAll(strip, "\x1b[31m", "")
	strip = strings.ReplaceAll(strip, "\x1b[0m", "")
	if strip != plain {
		t.Fatalf("color changed the words\nplain:\n%s\nstrip:\n%s", plain, strip)
	}
}

func TestRenderDoesNotInventClanOrZeroActivity(t *testing.T) {
	sum := clan.Summary{
		Prefix:     "ps99",
		ExportedAt: "2026-11-18T20:12:34Z",
		Current: clan.Current{
			CurrenciesKnown: false,
			Clan:            clan.ClanCurrent{Reason: "clan tag is absent from profile and extended profile"},
		},
		Windows: clan.Windows{Previous: clan.Window{Reason: "no earlier snapshot in this session"}},
	}
	text := Render(View{Summary: sum})
	if strings.Contains(text, "Clan QNE") || strings.Contains(text, "\n  Place ") || strings.Contains(text, "Contributor rank") {
		t.Fatalf("fabricated clan:\n%s", text)
	}
	if !strings.Contains(text, "Clan\n  unavailable") {
		t.Fatalf("missing clan reason:\n%s", text)
	}
	if strings.Contains(text, " 0\n") || strings.Contains(text, "Eggs opened") {
		t.Fatalf("unknown activity shown as zero:\n%s", text)
	}
	if !strings.Contains(text, "unavailable") {
		t.Fatalf("missing unavailable:\n%s", text)
	}
}

func TestRenderRatesStayUnavailable(t *testing.T) {
	elapsed := int64(300)
	place := int64(4)
	text := Render(View{
		Summary: clan.Summary{
			Prefix:     "ps99",
			ExportedAt: "2026-11-18T20:12:34Z",
			Current: clan.Current{
				CurrenciesKnown: true,
				Currencies: map[string]clan.CurrencyAmount{
					"Diamonds": {Amount: "5", DisplayName: "Diamonds"},
				},
				StatisticsKnown: true,
				Statistics:      map[string]string{"Eggs Opened": "10"},
				Clan: clan.ClanCurrent{Tag: "QNE", ActiveBattle: &clan.BattleCurrent{
					ID: "battle", Place: &place, Points: "10",
				}},
			},
			Windows: clan.Windows{Previous: clan.Window{
				Valid:          true,
				ElapsedSeconds: &elapsed,
				Eggs:           &clan.CurrencyDelta{Status: "unavailable", Reason: "absent prior eggs opened"},
				ChangedCurrencies: map[string]clan.CurrencyDelta{
					"Diamonds": {Status: "unavailable", Reason: "newly observed"},
				},
				Clan: &clan.ClanWindow{Place: &clan.MetricDelta{Status: "unavailable", Reason: "absent prior place"}},
			}},
		},
	})
	if strings.Contains(text, "~0/") || strings.Contains(text, "0 since") || strings.Contains(text, "+0") {
		t.Fatalf("invented a zero change:\n%s", text)
	}
	if !strings.Contains(text, "Hatch rate") || !strings.Contains(text, "n/a") || !strings.Contains(text, "newly observed") {
		t.Fatalf("missing unavailable:\n%s", text)
	}
	if !strings.Contains(text, "#4") || strings.Contains(text, "#4    ") {
		t.Fatalf("place delta invented:\n%s", text)
	}
}

func TestMachineBundleFailureIsVisible(t *testing.T) {
	text := Render(View{
		Summary: clan.Summary{Prefix: "ps99", ExportedAt: "2026-11-18T20:12:34Z"},
		Bundle:  &Bundle{Error: "disk full"},
	})
	if !strings.Contains(text, "ZIP failed") || !strings.Contains(text, "disk full") {
		t.Fatalf("zip failure hidden:\n%s", text)
	}
	if strings.Contains(text, "sha256") {
		t.Fatalf("failed zip looked complete:\n%s", text)
	}
}

func TestNextRefreshUsesLocalZone(t *testing.T) {
	loc := time.FixedZone("LOC", -3*3600-30*60)
	text := Render(View{
		Summary:  clan.Summary{Prefix: "ps99", ExportedAt: "2026-11-06T15:45:30Z"},
		Location: loc,
		Manifest: snapshot.Manifest{Files: []snapshot.FileRecord{
			{Kind: "profile-json", Refresh: []byte(`{"resetsAt":"2026-11-07T00:00:00.000Z","nextRefreshEligibleAt":"2026-11-06T12:10:00.000Z"}`)},
			{Kind: "inventory-json", Refresh: []byte(`{"resetsAt":"2026-11-07T00:00:00.000Z","nextRefreshEligibleAt":"2026-11-06T12:05:00.000Z"}`)},
		}},
	})
	if !strings.Contains(text, "Next refresh   2026-11-06 08:35:00 LOC") {
		t.Fatalf("refresh line:\n%s", text)
	}
	if strings.Contains(text, "12:05:00") || strings.Contains(text, "12:10:00") || strings.Contains(text, "2026-11-07") {
		t.Fatalf("raw or resetsAt time leaked:\n%s", text)
	}
}

func TestNextRefreshAbsentDoesNotInvent(t *testing.T) {
	text := Render(View{
		Summary: clan.Summary{Prefix: "ps99", ExportedAt: "2026-11-06T15:45:30Z"},
		Manifest: snapshot.Manifest{Files: []snapshot.FileRecord{
			{Kind: "profile-json", Refresh: []byte(`{"resetsAt":"2026-11-07T00:00:00.000Z","quotaExhausted":false}`)},
		}},
	})
	if !strings.Contains(text, "Next refresh   n/a") || !strings.Contains(text, "next refresh time was not returned") {
		t.Fatalf("missing reason:\n%s", text)
	}
	if strings.Contains(text, "2026-11-07") {
		t.Fatalf("invented resetsAt:\n%s", text)
	}
	none := Render(View{Summary: clan.Summary{Prefix: "ps99"}})
	if !strings.Contains(none, "refresh time was not returned") {
		t.Fatalf("no refresh:\n%s", none)
	}
}

func TestWallElapsedStaysSeparateFromPlaytime(t *testing.T) {
	wall := int64(828)
	session := int64(12*3600 + 12*60 + 47)
	now := time.Date(2026, 11, 19, 6, 31, 35, 0, time.UTC)
	text := Render(View{Summary: clan.Summary{
		Prefix:     "ps99",
		Profile:    "main",
		ExportedAt: now.Format(time.RFC3339),
		Current: clan.Current{
			StatisticsKnown: true,
			Statistics:      map[string]string{"Playtime": "2005372"},
		},
		Windows: clan.Windows{
			Session:  clan.Window{Valid: true, ElapsedSeconds: &session},
			Previous: clan.Window{Valid: true, ElapsedSeconds: &wall, BaselineFile: "prior.json"},
		},
	}, Priors: []clan.Prior{{
		File: "prior.json", StatsKnown: true, Stats: map[string]string{"Playtime": "2004600"},
	}}})
	if !strings.Contains(text, "Since previous 13m 48s") {
		t.Fatalf("wall elapsed:\n%s", text)
	}
	if !strings.Contains(text, "Session        12h 12m 47s") {
		t.Fatalf("session replaced:\n%s", text)
	}
	if !strings.Contains(text, "+12m 52s") {
		t.Fatalf("playtime delta:\n%s", text)
	}
	if strings.Contains(text, "+13m 48s") || strings.Contains(text, "since previous") {
		t.Fatalf("playtime was normalized to wall elapsed:\n%s", text)
	}
}

func TestPlaceImprovementAndClanRates(t *testing.T) {
	elapsed := int64(3600)
	place := int64(1255)
	rank := int64(2)
	members := int64(50)
	capacity := int64(50)
	contributors := int64(36)
	now := time.Date(2026, 11, 19, 6, 31, 35, 0, time.UTC)
	finish := now.Add(33*time.Hour + 12*time.Minute)
	text := Render(View{Summary: clan.Summary{
		Prefix:     "ps99",
		ExportedAt: now.Format(time.RFC3339),
		Current: clan.Current{
			Clan: clan.ClanCurrent{
				Tag: "QNE", Name: "QNE",
				MemberCount: &members, MemberCapacity: &capacity, ContributorCount: &contributors,
				ActiveBattle: &clan.BattleCurrent{
					ID: "CozyBattle2026", Title: "CozyBattle2026", FinishTime: finish.Format(time.RFC3339),
					Place: &place, Points: "89442", UserContribution: "8659", UserContributorRank: &rank,
				},
			},
		},
		Windows: clan.Windows{Previous: clan.Window{
			Valid: true, ElapsedSeconds: &elapsed,
			Clan: &clan.ClanWindow{
				Place:            &clan.MetricDelta{Baseline: "1323", Current: "1255", Delta: "-68", NetPerHour: "-68"},
				Points:           &clan.MetricDelta{Delta: "377", NetPerHour: "377"},
				UserContribution: &clan.MetricDelta{Delta: "100", NetPerHour: "100"},
			},
		}},
	}})
	for _, want := range []string{
		"#1,255    +68 places    ~68 places/h",
		"Clan points",
		"+377    ~377/h",
		"Your contribution",
		"+100    ~100/h",
		"Share of clan gain",
		"26.5%",
		"Members                50/50",
		"Contributors           36",
		"Battle remaining       1d 9h 12m",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("missing %q\n%s", want, text)
		}
	}
	if strings.Contains(text, "-68") {
		t.Fatalf("raw place delta leaked:\n%s", text)
	}
	worse := int64(1280)
	worseText := Render(View{Summary: clan.Summary{
		Prefix:     "ps99",
		ExportedAt: now.Format(time.RFC3339),
		Current: clan.Current{Clan: clan.ClanCurrent{Tag: "QNE", ActiveBattle: &clan.BattleCurrent{
			ID: "b", Place: &worse, Points: "1",
		}}},
		Windows: clan.Windows{Previous: clan.Window{
			Valid: true, ElapsedSeconds: &elapsed,
			Clan: &clan.ClanWindow{Place: &clan.MetricDelta{Delta: "25", NetPerHour: "25"}},
		}},
	}})
	if !strings.Contains(worseText, "#1,280    -25 places    ~-25 places/h") {
		t.Fatalf("worsening:\n%s", worseText)
	}
	if strings.Contains(worseText, "+25 places") {
		t.Fatalf("raw worsening sign leaked:\n%s", worseText)
	}
}

func TestHumanCurrencyRateResetCases(t *testing.T) {
	a := time.Date(2026, 11, 19, 2, 0, 0, 0, time.UTC)
	b := a.Add(time.Hour)
	c := a.Add(2 * time.Hour)
	base := clan.Prior{
		File: "a.json", ExportedAt: a, ExportedAtText: a.Format(time.RFC3339),
		SessionID: "s", Profile: "main", CurrenciesKnown: true,
		Currencies: map[string]string{"PumpkinTokens": "30000", "Diamonds": "100"},
	}
	spend := clan.Build(clan.Input{
		Now: b, SessionID: "s", Profile: "main", Prefix: "ps99", CurrenciesKnown: true,
		Currencies: map[string]clan.CurrencyAmount{
			"PumpkinTokens": {Amount: "2900", DisplayName: "Pumpkin Tokens"},
			"Diamonds":      {Amount: "150", DisplayName: "Diamonds"},
		},
		Priors: []clan.Prior{base},
	})
	spendText := Render(View{Summary: spend, Priors: []clan.Prior{base}})
	if !strings.Contains(spendText, "-27,100 net    spend/reset detected") {
		t.Fatalf("spend interval:\n%s", spendText)
	}
	if strings.Contains(spendText, "/h") && strings.Contains(spendText, "Pumpkin") {
		// Diamonds still have a rate. The pumpkin line must not.
	}
	if strings.Contains(lineOf(spendText, "Pumpkin Tokens"), "/h") || strings.Contains(lineOf(spendText, "Pumpkin Tokens"), "/m") {
		t.Fatalf("spend line invented a rate:\n%s", spendText)
	}
	if !strings.Contains(lineOf(spendText, "Diamonds"), "/h") {
		t.Fatalf("diamond rate was cleared:\n%s", spendText)
	}
	reset := clan.Prior{
		File: "b.json", ExportedAt: b, ExportedAtText: b.Format(time.RFC3339),
		SessionID: "s", Profile: "main", CurrenciesKnown: true,
		Currencies: map[string]string{"PumpkinTokens": "2900", "Diamonds": "150"},
	}
	clean := clan.Build(clan.Input{
		Now: c, SessionID: "s", Profile: "main", Prefix: "ps99", CurrenciesKnown: true,
		Currencies: map[string]clan.CurrencyAmount{
			"PumpkinTokens": {Amount: "5000", DisplayName: "Pumpkin Tokens"},
			"Diamonds":      {Amount: "180", DisplayName: "Diamonds"},
		},
		Priors: []clan.Prior{base, reset},
	})
	cleanText := Render(View{Summary: clean, Priors: []clan.Prior{base, reset}})
	pumpkin := lineOf(cleanText, "Pumpkin Tokens")
	if !strings.Contains(pumpkin, "+2,100") || !strings.Contains(pumpkin, "/h") || strings.Contains(pumpkin, "spend/reset") {
		t.Fatalf("clean interval:\n%s", cleanText)
	}
	if clean.Windows.Session.ChangedCurrencies["PumpkinTokens"].NetPerHour != "" {
		t.Fatalf("session rate crossed the decrease: %+v", clean.Windows.Session.ChangedCurrencies["PumpkinTokens"])
	}
	earn := clan.Build(clan.Input{
		Now: b, SessionID: "s", Profile: "main", Prefix: "ps99", CurrenciesKnown: true,
		Currencies: map[string]clan.CurrencyAmount{
			"PumpkinTokens": {Amount: "31200", DisplayName: "Pumpkin Tokens"},
		},
		Priors: []clan.Prior{base},
	})
	earnLine := lineOf(Render(View{Summary: earn, Priors: []clan.Prior{base}}), "Pumpkin Tokens")
	if !strings.Contains(earnLine, "+1,200") || !strings.Contains(earnLine, "~1.2k/h") || strings.Contains(earnLine, "spend/reset") {
		t.Fatalf("normal earning:\n%s", earnLine)
	}
}

func lineOf(text, label string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.Contains(line, label) {
			return line
		}
	}
	return ""
}
