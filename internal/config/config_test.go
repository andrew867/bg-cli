package config

import (
	"testing"
	"time"
)

func TestBindAccountSharesClientAcrossRobloxIDs(t *testing.T) {
	issued := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	expires := time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)
	f := &File{
		DefaultProfile: "main",
		Profiles: map[string]Profile{
			"main": {
				Alias:       "main",
				ClientID:    "same-client",
				Scopes:      []string{"player-data:pet-simulator-99:profile:read"},
				IssuedAt:    issued,
				ExpiresAt:   expires,
				RedirectURI: "http://127.0.0.1:8765/callback",
				AccountID:   "111",
				PS99UserID:  "111",
			},
			"alt1": {
				ClientID:    "same-client",
				AccountID:   "222",
				PS99UserID:  "222",
				PS99ClanTag: "ALT",
			},
		},
	}

	p, aliases, ok := BindAccount(f, "alt1")
	if !ok {
		t.Fatal("expected alt1")
	}
	if p.Alias != "alt1" {
		t.Fatalf("alias %q", p.Alias)
	}
	if p.AccountID != "222" || p.PS99UserID != "222" || p.PS99ClanTag != "ALT" {
		t.Fatalf("roblox identity changed: %+v", p)
	}
	if p.RedirectURI != "http://127.0.0.1:8765/callback" {
		t.Fatalf("redirect %q", p.RedirectURI)
	}
	if p.ClientID != "same-client" || len(p.Scopes) != 1 || !p.ExpiresAt.Equal(expires) || !p.IssuedAt.Equal(issued) {
		t.Fatalf("auth metadata %+v", p)
	}
	if len(aliases) < 2 || aliases[0] != "alt1" || aliases[1] != "main" {
		t.Fatalf("aliases %v", aliases)
	}
	if f.Profiles["alt1"].RedirectURI != "" || len(f.Profiles["alt1"].Scopes) != 0 {
		t.Fatal("bind wrote inherited fields back onto the stored profile")
	}
}

func TestBindAccountDoesNotShareDifferentClient(t *testing.T) {
	f := &File{
		DefaultProfile: "main",
		Profiles: map[string]Profile{
			"main": {
				Alias:       "main",
				ClientID:    "client-a",
				RedirectURI: "http://127.0.0.1:8765/callback",
				Scopes:      []string{"player-data:pet-simulator-99:profile:read"},
				AccountID:   "111",
			},
			"alt1": {
				Alias:     "alt1",
				ClientID:  "client-b",
				AccountID: "111",
			},
		},
	}
	p, aliases, ok := BindAccount(f, "alt1")
	if !ok {
		t.Fatal("expected alt1")
	}
	if p.RedirectURI != "" || len(p.Scopes) != 0 {
		t.Fatalf("different client inherited auth metadata: %+v", p)
	}
	if len(aliases) != 1 || aliases[0] != "alt1" {
		t.Fatalf("aliases %v", aliases)
	}
}
