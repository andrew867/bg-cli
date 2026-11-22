package config

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestDiscoverProfileParsesPositiveIdentityOnly(t *testing.T) {
	body := json.RawMessage(`{
		"linkedAccountId": "abc123linked",
		"UserID": 1001,
		"Username": "PlayerOne",
		"DisplayName": "Player One",
		"AccountID": "9999",
		"Currency": {"Diamonds": {"_am": 1}}
	}`)
	got := DiscoverProfile(body)
	if got.LinkedAccountID != "abc123linked" || got.PS99UserID != "1001" {
		t.Fatalf("identity %+v", got)
	}
	if got.Username != "PlayerOne" || got.DisplayName != "Player One" {
		t.Fatalf("names %+v", got)
	}
	if !strings.Contains(got.Warning, "AccountID") {
		t.Fatalf("warning %q", got.Warning)
	}
	if strings.Contains(got.LinkedAccountID, "9999") || got.PS99UserID == "9999" {
		t.Fatal("legacy AccountID was reused")
	}
}

func TestDiscoverProfileIgnoresLegacyAccountIDAlone(t *testing.T) {
	got := DiscoverProfile(json.RawMessage(`{"AccountID":"555","Rank":1}`))
	if got.LinkedAccountID != "" || got.PS99UserID != "" {
		t.Fatalf("guessed identity %+v", got)
	}
	if got.Warning == "" {
		t.Fatal("expected an ambiguous legacy warning")
	}
}

func TestDiscoverProfileRejectsZeroUserID(t *testing.T) {
	got := DiscoverProfile(json.RawMessage(`{"UserID":0,"RobloxUserId":"-4"}`))
	if got.PS99UserID != "" {
		t.Fatalf("non-positive id accepted: %+v", got)
	}
}

func TestDiscoverProfileEmptyOfficialShape(t *testing.T) {
	got := DiscoverProfile(json.RawMessage(`{"Rank":12,"Currency":{"Diamonds":{"_am":10}}}`))
	if got.LinkedAccountID != "" || got.PS99UserID != "" || got.Username != "" {
		t.Fatalf("invented identity %+v", got)
	}
}

func TestReconcileDoesNotRetargetAlias(t *testing.T) {
	cfg := &File{Profiles: map[string]Profile{
		"main": {Alias: "main", PS99UserID: "111", LinkedAccountID: "link-main"},
		"alt1": {Alias: "alt1", PS99UserID: "222"},
	}}
	_, _, err := ReconcileGrant(cfg, "alt1", GrantIdentity{PS99UserID: "111", LinkedAccountID: "link-main"})
	if err == nil || !strings.Contains(err.Error(), "was not changed") {
		t.Fatalf("err %v", err)
	}
	if cfg.Profiles["alt1"].PS99UserID != "222" || cfg.Profiles["alt1"].LinkedAccountID != "" {
		t.Fatalf("alias retargeted: %+v", cfg.Profiles["alt1"])
	}
	if cfg.Profiles["main"].Alias != "main" {
		t.Fatal("main renamed")
	}
}

func TestReconcileMatchesKnownAliasWithoutRenaming(t *testing.T) {
	cfg := &File{Profiles: map[string]Profile{
		"main": {Alias: "main", PS99UserID: "111", Username: "OldName"},
	}}
	alias, notice, err := ReconcileGrant(cfg, "newalt", GrantIdentity{PS99UserID: "111", Username: "NewName"})
	if err != nil {
		t.Fatal(err)
	}
	if alias != "main" {
		t.Fatalf("alias %s notice %s", alias, notice)
	}
	if _, ok := cfg.Profiles["newalt"]; ok {
		t.Fatal("created a second alias")
	}
	updated, err := ApplyDiscovered(cfg.Profiles["main"], GrantIdentity{PS99UserID: "111", Username: "NewName"})
	if err != nil {
		t.Fatal(err)
	}
	if updated.Alias != "main" || updated.Username != "NewName" || updated.PS99UserID != "111" {
		t.Fatalf("updated %+v", updated)
	}
}

func TestReconcileAmbiguousStops(t *testing.T) {
	cfg := &File{Profiles: map[string]Profile{
		"main": {Alias: "main", PS99UserID: "111"},
		"copy": {Alias: "copy", PS99UserID: "111"},
	}}
	_, _, err := ReconcileGrant(cfg, "fresh", GrantIdentity{PS99UserID: "111"})
	if err == nil || !strings.Contains(err.Error(), "more than one") {
		t.Fatalf("err %v", err)
	}
}

func TestUpstreamConflictIsHard(t *testing.T) {
	err := UpstreamConflict(Profile{Alias: "alt1", PS99UserID: "222"}, json.RawMessage(`{"UserID":111}`))
	if err == nil {
		t.Fatal("expected conflict")
	}
	if _, ok := err.(*IdentityError); !ok {
		t.Fatalf("type %T", err)
	}
	if strings.Contains(err.Error(), "111") || strings.Contains(err.Error(), "222") {
		t.Fatalf("ids leaked: %s", err.Error())
	}
	if err := UpstreamConflict(Profile{Alias: "main", PS99UserID: "111"}, json.RawMessage(`{"Rank":1}`)); err != nil {
		t.Fatal(err)
	}
}

func TestLegacyWarningDoesNotTreatAccountIDAsLinked(t *testing.T) {
	warnings := LegacyIdentityWarnings(Profile{AccountID: "111", PS99UserID: "222"})
	text := strings.Join(warnings, " ")
	if !strings.Contains(text, "not the BIG Games linked-account id") {
		t.Fatalf("warnings %v", warnings)
	}
}
