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
