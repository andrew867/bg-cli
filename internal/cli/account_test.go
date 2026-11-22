package cli

import (
	"bytes"
	"encoding/json"
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

func setupProfile(t *testing.T, store *auth.MemoryStore, scopes []string) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias:     "main",
				ClientID:  "cid",
				Scopes:    scopes,
				IssuedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	_ = store.Set("main", "synthetic_test_access_token_do_not_use")
}

func accountServer(t *testing.T, path string, status int, file string, headers map[string]string) *httptest.Server {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", file))
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing bearer")
		}
		if r.URL.Path != path {
			t.Errorf("path %s", r.URL.Path)
		}
		for k, v := range headers {
			w.Header().Set(k, v)
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
}

func TestAccountInventoryOK(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := accountServer(t, "/v1/account/inventory", 200, "account-inventory-ok.json", map[string]string{
		"Content-Type":          "application/json",
		"X-RateLimit-Limit":     "48",
		"X-RateLimit-Remaining": "45",
	})
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
		IsTTY: func() bool { return true },
	}, []string{"account", "inventory", "--profile", "main", "--pretty"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "synthetic_test_access_token") {
		t.Fatal("token leaked")
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Freshness.SnapshotAt == nil {
		t.Fatalf("%+v", env)
	}
	data := env.Data.(map[string]any)
	if data["rowCount"].(float64) != 7 {
		t.Fatalf("rowCount=%v", data["rowCount"])
	}
	// quantitySum includes the huge currency stack; petQuantitySum does not.
	// Assert on raw stdout so large integers are not float64-rounded by json.Unmarshal.
	out := stdout.String()
	if !strings.Contains(out, `"quantitySum":9007199254741014`) && !strings.Contains(out, `"quantitySum": 9007199254741014`) {
		t.Fatalf("quantitySum missing exact all-rows total:\n%s", out)
	}
	if !strings.Contains(out, `"petQuantitySum":21`) && !strings.Contains(out, `"petQuantitySum": 21`) {
		t.Fatalf("petQuantitySum missing:\n%s", out)
	}
	if data["petRowCount"].(float64) != 6 {
		t.Fatalf("petRowCount=%v", data["petRowCount"])
	}
}

func TestAccountCurrenciesOK(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := accountServer(t, "/v1/account/profile", 200, "account-profile-ok.json", nil)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "currencies", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	var env output.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data := env.Data.(map[string]any)
	if data["currencyFieldPresent"] != true {
		t.Fatal(data)
	}
	if int(data["entryCount"].(float64)) != 2 {
		t.Fatal(data)
	}
}

func TestAccountWithoutProfile(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: auth.NewMemoryStore()},
		[]string{"account", "inventory"})
	if code != exitcode.AuthRequired {
		t.Fatalf("exit %d", code)
	}
}

func TestAccountMissingScopeNoRequest(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, []string{"player-data:pet-simulator-99:profile:read"})
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
	}, []string{"account", "inventory", "--profile", "main"})
	if code != exitcode.AuthRequired {
		t.Fatalf("exit %d", code)
	}
	if called {
		t.Fatal("must not request without scope")
	}
}

func TestAccountUnauthorizedExit8(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := accountServer(t, "/v1/account/profile", 401, "account-unauthorized.json", nil)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
	}, []string{"account", "profile", "--profile", "main"})
	if code != exitcode.AuthRequired {
		t.Fatalf("exit %d stdout=%s", code, stdout.String())
	}
	if strings.Contains(stdout.String(), "synthetic_test_access_token") {
		t.Fatal("token leaked")
	}
}

func TestAccountRefreshRequiresYes(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		IsTTY: func() bool { return false },
	}, []string{"account", "inventory", "--refresh", "--profile", "main"})
	if code != exitcode.Usage {
		t.Fatalf("exit %d", code)
	}
	if called {
		t.Fatal("must not request")
	}
}

func TestAccountQuotaExhaustedStillSuccess(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := accountServer(t, "/v1/account/profile", 200, "account-quota-exhausted.json", map[string]string{
		"X-RateLimit-Quota-Exhausted": "true",
	})
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "currencies", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	var env output.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	data := env.Data.(map[string]any)
	if data["quotaExhausted"] != true {
		t.Fatal(data)
	}
}

func TestNoAccountDiskCache(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	cfgDir := os.Getenv("BG_CLI_CONFIG_DIR")
	srv := accountServer(t, "/v1/account/inventory", 200, "account-inventory-ok.json", nil)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	_ = Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "inventory", "--profile", "main"})
	entries, _ := os.ReadDir(cfgDir)
	for _, e := range entries {
		if e.Name() != config.ConfigFile {
			t.Fatalf("unexpected file in config dir: %s", e.Name())
		}
	}
}

func TestHandAddedProfileDoesNotBorrowToken(t *testing.T) {
	const mainToken = "synthetic_main_client_token"
	const altToken = "synthetic_alt_own_token"
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias:       "main",
				ClientID:    "same-client",
				Scopes:      auth.DefaultScopes,
				IssuedAt:    time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				ExpiresAt:   time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
				RedirectURI: "http://127.0.0.1:8765/callback",
				AccountID:   "111",
				PS99UserID:  "111",
			},
			"alt1": {
				Alias:       "alt1",
				ClientID:    "same-client",
				Scopes:      auth.DefaultScopes,
				AccountID:   "222",
				PS99UserID:  "222",
				PS99ClanTag: "ALT",
			},
			"other": {
				Alias:      "other",
				ClientID:   "other-client",
				AccountID:  "333",
				PS99UserID: "333",
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	if err := store.Set("main", mainToken); err != nil {
		t.Fatal(err)
	}

	var accountCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		accountCalls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "currencies", "--profile", "alt1"})
	if code != exitcode.AuthRequired {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if accountCalls != 0 {
		t.Fatalf("different Roblox id reused the shared token for an account request (%d)", accountCalls)
	}
	if strings.Contains(stdout.String(), mainToken) || strings.Contains(stderr.String(), mainToken) {
		t.Fatal("token leaked")
	}
	if !strings.Contains(stderr.String(), `profile "alt1"`) || !strings.Contains(stderr.String(), "auth login --profile alt1") {
		t.Fatalf("reason missing:\n%s", stderr.String())
	}
	if strings.Contains(stdout.String(), "123456789") || strings.Contains(stderr.String(), "123456789") {
		t.Fatal("main profile body was emitted for alt1")
	}

	raw, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), mainToken) || strings.Contains(string(raw), "access_token") {
		t.Fatal("token written into config")
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	alt := loaded.Profiles["alt1"]
	if alt.PS99UserID != "222" || alt.AccountID != "222" || alt.RedirectURI != "" || len(alt.Scopes) != len(auth.DefaultScopes) {
		t.Fatalf("stored alt1 changed: %+v", alt)
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}, []string{"auth", "status", "--profile", "alt1"})
	if code != 0 {
		t.Fatalf("status exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), mainToken) {
		t.Fatal("token leaked in status")
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["tokenPresent"] != false || data["credentialOwner"] != "" || data["credentialProfile"] != "" {
		t.Fatalf("different Roblox id reported main's token as alt1's credential: %#v", data)
	}
	if data["redirectUri"] != "" {
		t.Fatalf("redirect %#v", data["redirectUri"])
	}
	if data["ps99UserId"] != "222" || data["accountId"] != "222" {
		t.Fatalf("roblox id overwritten: %#v", data)
	}

	called := false
	denied := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer denied.Close()
	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: denied.URL, HTTPClient: denied.Client(),
	}, []string{"account", "currencies", "--profile", "other"})
	if code != exitcode.AuthRequired {
		t.Fatalf("other client exit %d stdout=%s", code, stdout.String())
	}
	if called {
		t.Fatal("different clientId must not use the shared token")
	}

	if err := store.Set("alt1", altToken); err != nil {
		t.Fatal(err)
	}
	own := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+altToken {
			t.Error("own token was not preferred")
		}
		body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
		if err != nil {
			t.Error(err)
			return
		}
		_, _ = w.Write(body)
	}))
	defer own.Close()
	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: own.URL, HTTPClient: own.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "currencies", "--profile", "alt1"})
	if code != 0 {
		t.Fatalf("own token exit %d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), altToken) || strings.Contains(stdout.String(), mainToken) {
		t.Fatal("token leaked")
	}
}
