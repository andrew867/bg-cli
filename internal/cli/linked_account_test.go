package cli

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func TestExpiredTokenDoesNotFallBack(t *testing.T) {
	const mainToken = "synthetic_main_live_token"
	const altToken = "synthetic_alt_expired_token"
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	now := time.Date(2026, 12, 15, 12, 0, 0, 0, time.UTC)
	if err := config.Save(&config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias: "main", ClientID: "same-client", Scopes: auth.DefaultScopes,
				ExpiresAt: now.Add(time.Hour), PS99UserID: "111",
			},
			"alt1": {
				Alias: "alt1", ClientID: "same-client", Scopes: auth.DefaultScopes,
				ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC), PS99UserID: "222",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", mainToken)
	_ = store.Set("alt1", altToken)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Header.Get("Authorization") == "Bearer "+mainToken {
			t.Error("expired alt used main's bearer")
		}
	}))
	defer srv.Close()
	var stdout, stderr strings.Builder
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
		Clock: output.FixedClock{T: now},
	}, []string{"account", "currencies", "--profile", "alt1"})
	if code != exitcode.AuthRequired || calls != 0 {
		t.Fatalf("exit %d calls %d stderr=%s", code, calls, stderr.String())
	}
	if !strings.Contains(stderr.String(), "expired") || strings.Contains(stdout.String()+stderr.String(), mainToken) || strings.Contains(stdout.String()+stderr.String(), altToken) {
		t.Fatalf("stderr=%s", stderr.String())
	}
}

func TestCredentialProfileDoesNotBorrowUnderModelB(t *testing.T) {
	const mainToken = "synthetic_main_live_token"
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	raw := `{
		"defaultProfile": "main",
		"profiles": {
			"main": {"alias": "main", "clientId": "same-client", "ps99UserId": "111"},
			"alt1": {"alias": "alt1", "clientId": "same-client", "ps99UserId": "222", "credentialProfile": "main"}
		}
	}`
	if err := os.WriteFile(filepath.Join(dir, config.ConfigFile), []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(config.LegacyIdentityWarnings(loaded.Profiles["alt1"]), " "), "credentialProfile") {
		t.Fatal("credentialProfile is no longer a warning")
	}
	loaded.Profiles["main"] = config.Profile{
		Alias: "main", ClientID: "same-client", Scopes: auth.DefaultScopes,
		ExpiresAt: time.Now().UTC().Add(time.Hour), PS99UserID: "111",
	}
	alt := loaded.Profiles["alt1"]
	alt.Scopes = auth.DefaultScopes
	alt.ExpiresAt = time.Now().UTC().Add(time.Hour)
	loaded.Profiles["alt1"] = alt
	if err := config.Save(loaded); err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "credentialProfile") {
		t.Fatal("old credentialProfile was written back")
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", mainToken)
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
	}))
	defer srv.Close()
	var stdout, stderr strings.Builder
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "inventory", "--profile", "alt1"})
	if code != exitcode.AuthRequired || calls != 0 {
		t.Fatalf("exit %d calls %d stderr=%s", code, calls, stderr.String())
	}
	if !strings.Contains(stderr.String(), "has no access token") || strings.Contains(stderr.String(), "credentialProfile") || strings.Contains(stdout.String()+stderr.String(), mainToken) {
		t.Fatalf("stderr=%s", stderr.String())
	}
}

func TestCrossAccountTradeDiffersAtTheRequest(t *testing.T) {
	const mainToken = "synthetic_main_trade_token"
	const altToken = "synthetic_alt_trade_token"
	const created = "2024-06-01T00:00:00Z"
	mainInv := `{
		"status":"ok",
		"data":{"items":[
			{"class":"Pet","id":"Titanic Kraken","count":1,"rawData":{"id":"Titanic Kraken","createdAt":"` + created + `","huge":false}}
		]}
	}`
	altInv := `{
		"status":"ok",
		"data":{"items":[
			{"class":"Pet","id":"Huge Cat","count":1,"rawData":{"id":"Huge Cat","createdAt":"` + created + `","huge":true}},
			{"class":"Pet","id":"Huge Dog","count":1,"rawData":{"id":"Huge Dog","createdAt":"` + created + `","huge":true}},
			{"class":"Pet","id":"Huge Owl","count":1,"rawData":{"id":"Huge Owl","createdAt":"` + created + `","huge":true}}
		]}
	}`
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	if err := config.Save(&config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias: "main", ClientID: "same-client", Scopes: auth.DefaultScopes,
				ExpiresAt: time.Now().UTC().Add(time.Hour), PS99UserID: "111", LinkedAccountID: "link-main",
			},
			"alt1": {
				Alias: "alt1", ClientID: "same-client", Scopes: auth.DefaultScopes,
				ExpiresAt: time.Now().UTC().Add(time.Hour), PS99UserID: "222", LinkedAccountID: "link-alt",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", mainToken)
	_ = store.Set("alt1", altToken)
	var sawMain, sawAlt bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account/inventory" {
			t.Errorf("path %s", r.URL.Path)
		}
		if strings.Contains(r.URL.RawQuery, "account=all") {
			t.Error("account=all was requested")
		}
		switch r.Header.Get("Authorization") {
		case "Bearer " + mainToken:
			sawMain = true
			_, _ = w.Write([]byte(mainInv))
		case "Bearer " + altToken:
			sawAlt = true
			_, _ = w.Write([]byte(altInv))
		default:
			t.Errorf("unexpected authorization")
			w.WriteHeader(http.StatusUnauthorized)
		}
	}))
	defer srv.Close()
	run := func(profile string) string {
		t.Helper()
		var stdout, stderr strings.Builder
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
			Clock: output.FixedClock{T: time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)},
		}, []string{"account", "inventory", "--profile", profile})
		if code != 0 {
			t.Fatalf("%s exit %d stderr=%s", profile, code, stderr.String())
		}
		return stdout.String()
	}
	mainOut := run("main")
	altOut := run("alt1")
	if !sawMain || !sawAlt {
		t.Fatal("server did not see both bearers")
	}
	if !strings.Contains(mainOut, "Titanic Kraken") || strings.Contains(mainOut, "Huge Cat") {
		t.Fatalf("main inventory was not the post-trade body")
	}
	if !strings.Contains(altOut, "Huge Cat") || !strings.Contains(altOut, "Huge Dog") || !strings.Contains(altOut, "Huge Owl") || strings.Contains(altOut, "Titanic Kraken") {
		t.Fatalf("alt inventory was not the post-trade body")
	}
	if !strings.Contains(mainOut, created) || !strings.Contains(altOut, created) {
		t.Fatal("createdAt was dropped")
	}
	if strings.Contains(mainOut, altToken) || strings.Contains(altOut, mainToken) {
		t.Fatal("token leaked")
	}
}
