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
	if data["rowCount"].(float64) != 1 {
		t.Fatalf("%v", data["rowCount"])
	}
	switch q := data["quantitySum"].(type) {
	case float64:
		if q != 3 {
			t.Fatalf("quantitySum=%v", q)
		}
	case json.Number:
		if q != "3" {
			t.Fatalf("quantitySum=%v", q)
		}
	default:
		t.Fatalf("quantitySum type %T", data["quantitySum"])
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
