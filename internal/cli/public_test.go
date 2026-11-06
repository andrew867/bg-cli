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
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func TestLeaderboardListNoAuth(t *testing.T) {
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "leaderboards-list-ok.json"))
	var sawAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sawAuth = r.Header.Get("Authorization")
		w.Header().Set("Cache-Control", "public, max-age=60")
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Setenv("BG_CLI_CACHE_DIR", dir)
	store := auth.NewMemoryStore()
	_ = store.Set("main", "synthetic_test_access_token_do_not_use")
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}, []string{"leaderboard", "list"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if sawAuth != "" {
		t.Fatal("account token must not attach to public routes")
	}
	var env output.Envelope
	_ = json.Unmarshal(stdout.Bytes(), &env)
	if !env.OK {
		t.Fatal(env)
	}
}

func TestLeaderboardPlayerNotRetained(t *testing.T) {
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "leaderboard-player-not-found.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("BG_CLI_CACHE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, BaseURL: srv.URL, HTTPClient: srv.Client()},
		[]string{"leaderboard", "player", "SyntheticLiveBoard", "123456789"})
	if code != exitcode.NotFound {
		t.Fatalf("exit %d", code)
	}
	if !strings.Contains(stdout.String(), "not_ranked_or_not_retained") {
		t.Fatal(stdout.String())
	}
}

func TestPlayerViewNotPublic(t *testing.T) {
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "player-inventory-not-public.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("BG_CLI_CACHE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, BaseURL: srv.URL, HTTPClient: srv.Client()},
		[]string{"player", "show", "test_user", "--include", "inventory"})
	if code != exitcode.NotFound {
		t.Fatalf("exit %d %s", code, stdout.String())
	}
}

func TestInvalidJSONExit5(t *testing.T) {
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "invalid-json.txt"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	t.Setenv("BG_CLI_CACHE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, BaseURL: srv.URL, HTTPClient: srv.Client()},
		[]string{"leaderboard", "list"})
	if code != exitcode.Upstream && code != exitcode.Network {
		// mapNet uses Upstream for invalid JSON
		t.Fatalf("exit %d stdout=%s", code, stdout.String())
	}
}

func TestAccountExtendedRequiresScope(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called = true }))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client()},
		[]string{"account", "extended-profile", "--profile", "main"})
	if code != exitcode.AuthRequired {
		t.Fatalf("%d", code)
	}
	if called {
		t.Fatal("must not call without scope")
	}
}

func TestAccountMailWithScope(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-mail-ok.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account/mail" {
			t.Fatalf("path %s", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true }},
		[]string{"account", "mail", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
}
