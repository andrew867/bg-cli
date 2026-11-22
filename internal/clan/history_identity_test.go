package clan

import (
	"strings"
	"testing"
	"time"
)

func TestHistoryRejectsDifferentLinkedAccount(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Hour), ExportedAtText: now.Add(-time.Hour).Format(time.RFC3339),
		SessionID: "sess", LinkedAccountID: "link-alt", PS99UserID: "222", Profile: "alt1",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "2026-11-22-120000", Prefix: "ps99", Profile: "main",
		LinkedAccountID: "link-main", PS99UserID: "111", UserID: "111", SessionID: "sess",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "9"}},
		Priors: []Prior{prior},
	})
	if sum.Windows.Previous.Valid {
		t.Fatal("different linked account was rated")
	}
	if sum.Windows.Previous.Reason != BaselineIdentityRejected {
		t.Fatalf("reason %q", sum.Windows.Previous.Reason)
	}
	if sum.Windows.Five.Valid || sum.Windows.Hour.Valid || sum.Windows.Session.Valid {
		t.Fatal("a rejected account was used as a window baseline")
	}
	if sum.ProfileAlias != "main" || sum.LinkedAccountID != "link-main" || sum.PS99UserID != "111" {
		t.Fatalf("summary identity %+v", sum)
	}
}

func TestHistoryMatchesPS99WhenLinkedIDAbsent(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Minute), ExportedAtText: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "sess", UserID: "111", Profile: "renamed",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "main",
		PS99UserID: "111", UserID: "111", SessionID: "sess",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "2"}},
		Priors: []Prior{prior},
	})
	if !sum.Windows.Previous.Valid {
		t.Fatalf("same ps99 user was rejected: %s", sum.Windows.Previous.Reason)
	}
}

func TestHistoryDifferentAliasWithoutIDsIsRejected(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Minute), ExportedAtText: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "sess", Profile: "alt1",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "main", SessionID: "sess",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "2"}},
		Priors: []Prior{prior},
	})
	if sum.Windows.Previous.Valid || !strings.Contains(sum.Windows.Previous.Reason, "account identity differs") {
		t.Fatalf("alias matched: valid=%v reason=%s", sum.Windows.Previous.Valid, sum.Windows.Previous.Reason)
	}
}

func TestHistoryBothIDsMustAgree(t *testing.T) {
	a := HistoryIdentity{LinkedAccountID: "link-main", PS99UserID: "111"}
	b := HistoryIdentity{LinkedAccountID: "link-main", PS99UserID: "222"}
	if SameHistory(a, b) {
		t.Fatal("linked id matched across different ps99 users")
	}
}

func TestLegacyAccountIDIsNotLinkedID(t *testing.T) {
	old := HistoryIdentity{LegacyAccountID: "111"}
	newer := HistoryIdentity{PS99UserID: "111", LegacyAccountID: "111"}
	if SameHistory(old, newer) {
		t.Fatal("legacy accountId was treated as the ps99 user id")
	}
	if !SameHistory(HistoryIdentity{LegacyAccountID: "111"}, HistoryIdentity{LegacyAccountID: "111"}) {
		t.Fatal("old snapshots with only accountId should still match each other")
	}
}

func TestHistoryNewEpochDoesNotContinueContaminatedPS99(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Hour), ExportedAtText: now.Add(-time.Hour).Format(time.RFC3339),
		SessionID: "sess", PS99UserID: "222", Profile: "alt1",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "alt1",
		PS99UserID: "222", UserID: "222", CredentialEpoch: "grant-new", SessionID: "sess",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "9"}},
		Priors: []Prior{prior},
	})
	if sum.Windows.Previous.Valid || sum.Windows.Previous.Reason != BaselineIdentityRejected {
		t.Fatalf("contaminated baseline was used: valid=%v reason=%s", sum.Windows.Previous.Valid, sum.Windows.Previous.Reason)
	}
	if sum.CredentialEpoch != "grant-new" {
		t.Fatalf("epoch %q", sum.CredentialEpoch)
	}
}

func TestHistoryLegacySummariesWithoutEpochStillMatch(t *testing.T) {
	a := HistoryIdentity{PS99UserID: "222", Profile: "alt1"}
	b := HistoryIdentity{PS99UserID: "222", Profile: "alt1"}
	if !SameHistory(a, b) {
		t.Fatal("legacy summaries without an epoch were split")
	}
}

func TestHistorySameEpochContinuesAndDifferentEpochDoesNot(t *testing.T) {
	same := HistoryIdentity{PS99UserID: "222", CredentialEpoch: "grant-a", Profile: "alt1"}
	if !SameHistory(same, same) {
		t.Fatal("same epoch was rejected")
	}
	other := HistoryIdentity{PS99UserID: "222", CredentialEpoch: "grant-b", Profile: "alt1"}
	if SameHistory(same, other) {
		t.Fatal("different grants shared history")
	}
}

func TestHistoryContributionTargetDoesNotSplitAccount(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Minute), ExportedAtText: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "sess", PS99UserID: "111", UserID: "111", CredentialEpoch: "epoch-a", Profile: "main",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "main",
		PS99UserID: "111", UserID: "999999", CredentialEpoch: "epoch-a", SessionID: "sess",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "1"}},
		Priors: []Prior{prior},
	})
	if sum.PS99UserID != "111" || sum.UserID != "999999" {
		t.Fatalf("owner %q contribution %q", sum.PS99UserID, sum.UserID)
	}
	if !sum.Windows.Previous.Valid {
		t.Fatalf("contribution target split history: %s", sum.Windows.Previous.Reason)
	}
}

func TestHistoryVerifiedLinkedAccountIgnoresEpoch(t *testing.T) {
	a := HistoryIdentity{LinkedAccountID: "link-alt", PS99UserID: "222", CredentialEpoch: "old"}
	b := HistoryIdentity{LinkedAccountID: "link-alt", PS99UserID: "222", CredentialEpoch: "new"}
	if !SameHistory(a, b) {
		t.Fatal("verified linked account was split by epoch")
	}
	c := HistoryIdentity{LinkedAccountID: "link-main", PS99UserID: "222", CredentialEpoch: "old"}
	if SameHistory(a, c) {
		t.Fatal("different linked accounts matched")
	}
}

func TestContributionUserDoesNotSplitEpoch(t *testing.T) {
	now := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
	priors := []Prior{
		epochPrior(now.Add(-time.Hour), "hour.json", "111"),
		epochPrior(now.Add(-30*time.Minute), "thirty.json", "111"),
		epochPrior(now.Add(-5*time.Minute), "five.json", "111"),
	}
	cur := NewHistoryIdentity("", "", "222", "epoch-a", "", "main")
	if MatchingSession(priors, cur) != "sess" {
		t.Fatal("contribution target changed the session")
	}
	sum := buildEpoch(now, "222", "epoch-a", priors)
	assertLineage(t, sum)
	if sum.PS99UserID != "" || sum.UserID != "222" {
		t.Fatalf("owner %q contribution %q", sum.PS99UserID, sum.UserID)
	}
}

func TestEpochSurvivesAddingAndRemovingUserID(t *testing.T) {
	now := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
	none := []Prior{epochPrior(now.Add(-time.Minute), "none.json", "")}
	sum := buildEpoch(now, "222", "epoch-a", none)
	assertLineage(t, sum)
	overridden := []Prior{epochPrior(now.Add(-time.Minute), "over.json", "222")}
	sum = buildEpoch(now, "", "epoch-a", overridden)
	assertLineage(t, sum)
	if sum.UserID != "" || sum.PS99UserID != "" {
		t.Fatalf("cleared override became identity user %q ps99 %q", sum.UserID, sum.PS99UserID)
	}
}

func TestDifferentEpochIsNewLineage(t *testing.T) {
	now := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
	prior := []Prior{epochPrior(now.Add(-time.Minute), "old.json", "111")}
	sum := buildEpoch(now, "111", "epoch-b", prior)
	if sum.Windows.Previous.Valid || sum.Windows.Previous.Reason != BaselineIdentityRejected {
		t.Fatalf("different epoch continued: %+v", sum.Windows.Previous)
	}
}

func TestKnownPS99StaysOwnerWhenContributionChanges(t *testing.T) {
	now := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Minute), ExportedAtText: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "sess", PS99UserID: "111", UserID: "111", CredentialEpoch: "epoch-a", Profile: "main",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "main", SessionID: "sess",
		PS99UserID: "111", UserID: "999999", CredentialEpoch: "epoch-a",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "1"}},
		Priors: []Prior{prior},
	})
	if sum.PS99UserID != "111" || sum.UserID != "999999" {
		t.Fatalf("owner %q contribution %q", sum.PS99UserID, sum.UserID)
	}
	assertLineage(t, sum)
	if SameHistory(NewHistoryIdentity("", "111", "111", "epoch-a", "", "main"), NewHistoryIdentity("", "222", "111", "epoch-a", "", "main")) {
		t.Fatal("disagreeing owner ids were ignored")
	}
}

func TestLegacyUserIDStillComparesWithoutEpoch(t *testing.T) {
	a := NewHistoryIdentity("", "", "111", "", "", "main")
	b := NewHistoryIdentity("", "", "111", "", "", "alt1")
	if !SameHistory(a, b) {
		t.Fatal("legacy summaries that stored the owner in userId were split")
	}
	if SameHistory(a, NewHistoryIdentity("", "", "222", "", "", "main")) {
		t.Fatal("different legacy user ids matched")
	}
}

func TestNewEpochDoesNotContinueLegacyUserID(t *testing.T) {
	now := time.Date(2026, 11, 22, 16, 0, 0, 0, time.UTC)
	prior := Prior{
		File: "old.json", ExportedAt: now.Add(-time.Minute), ExportedAtText: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "sess", UserID: "222", Profile: "alt1",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
	sum := Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "alt1", SessionID: "sess",
		UserID: "222", CredentialEpoch: "grant-new",
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "9"}},
		Priors: []Prior{prior},
	})
	if sum.Windows.Previous.Valid || sum.Windows.Previous.Reason != BaselineIdentityRejected {
		t.Fatalf("new epoch continued a legacy userId: %+v", sum.Windows.Previous)
	}
	if sum.PS99UserID != "" {
		t.Fatalf("contribution was stored as the owner %q", sum.PS99UserID)
	}
}

func TestScanKeyUsesEpochNotContributionUser(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	writeSummary(t, dir, "2026-11-22-110000-a-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "main", UserID: "111", CredentialEpoch: "epoch-a",
	})
	writeSummary(t, dir, "2026-11-22-110000-b-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "main", UserID: "222", CredentialEpoch: "epoch-a",
	})
	scanned, err := Scan(dir, "ps99", now, "keep-second.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned.Priors) != 1 {
		t.Fatalf("same epoch was split by userId: %+v", scanned.Priors)
	}
	writeSummary(t, dir, "2026-11-22-100000-a-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "main", UserID: "111",
	})
	writeSummary(t, dir, "2026-11-22-100000-b-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-2 * time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "main", UserID: "222",
	})
	scanned, err = Scan(dir, "ps99", now, "")
	if err != nil {
		t.Fatal(err)
	}
	legacy := 0
	for _, p := range scanned.Priors {
		if p.CredentialEpoch == "" {
			legacy++
		}
	}
	if legacy != 2 {
		t.Fatalf("legacy user ids collapsed: %+v", scanned.Priors)
	}
}

func epochPrior(at time.Time, file, user string) Prior {
	return Prior{
		File: file, ExportedAt: at, ExportedAtText: at.Format(time.RFC3339),
		SessionID: "sess", UserID: user, CredentialEpoch: "epoch-a", Profile: "main",
		CurrenciesKnown: true, Currencies: map[string]string{"Diamonds": "1"},
	}
}

func buildEpoch(now time.Time, user, epoch string, priors []Prior) Summary {
	return Build(Input{
		Now: now, LocalStamp: "stamp", Prefix: "ps99", Profile: "main", SessionID: "sess",
		UserID: user, CredentialEpoch: epoch,
		CurrenciesKnown: true, Currencies: map[string]CurrencyAmount{"Diamonds": {Amount: "1"}},
		Priors: priors,
	})
}

func assertLineage(t *testing.T, sum Summary) {
	t.Helper()
	windows := []Window{sum.Windows.Previous, sum.Windows.Five, sum.Windows.Thirty, sum.Windows.Hour, sum.Windows.Session}
	for _, w := range windows {
		if w.Reason == BaselineIdentityRejected || (w.RequestedSeconds == nil && !w.Valid) {
			t.Fatalf("lineage split: %+v", w)
		}
	}
}

func TestScanDoesNotCollapseDifferentAccounts(t *testing.T) {
	dir := t.TempDir()
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	writeSummary(t, dir, "2026-11-22-110000-main-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "main", ProfileAlias: "main", LinkedAccountID: "link-main", PS99UserID: "111",
	})
	writeSummary(t, dir, "2026-11-22-110000-alt1-ps99-summary.json", Summary{
		SchemaVersion: "1.0", ExportedAt: now.Add(-time.Minute).Format(time.RFC3339),
		SessionID: "s", Profile: "alt1", ProfileAlias: "alt1", LinkedAccountID: "link-alt", PS99UserID: "222",
	})
	scanned, err := Scan(dir, "ps99", now, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(scanned.Priors) != 2 {
		t.Fatalf("priors %+v", scanned.Priors)
	}
}
