package auth

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/config"
)

func TestLoginSuccessDeterministic(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)

	tokenBody := loadFixture(t, "oauth-token-ok.json")

	// Reserve a loopback port for the redirect URI.
	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserve.Addr().String()
	_ = reserve.Close()
	redirectURI := "http://" + addr + "/callback"

	var authorizeHits int
	oauthMux := http.NewServeMux()
	oauthMux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		authorizeHits++
		q := r.URL.Query()
		code := "synthetic-auth-code"
		// Simulate browser redirect to loopback.
		cb := q.Get("redirect_uri") + "?code=" + url.QueryEscape(code) + "&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, cb, http.StatusFound)
	})
	oauthMux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") == "" {
			t.Error("missing basic auth")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(tokenBody)
	})
	oauthSrv := httptest.NewServer(oauthMux)
	defer oauthSrv.Close()

	store := NewMemoryStore()
	var stderr strings.Builder
	opened := make(chan string, 1)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	res, secrets, err := Login(ctx, LoginOptions{
		Profile:      "main",
		ClientID:     "test-client-id",
		RedirectURI:  redirectURI,
		Scopes:       DefaultScopes,
		AuthorizeURL: oauthSrv.URL + "/oauth/authorize",
		TokenURL:     oauthSrv.URL + "/oauth/token",
		HTTPClient:   oauthSrv.Client(),
		TokenStore:   store,
		SecretSource: SecretSource{
			Env: func(k string) string {
				if k == ClientSecretEnv {
					return "synthetic-client-secret"
				}
				return ""
			},
			Stderr: io.Discard,
			IsTTY:  func() bool { return false },
		},
		Stderr: &stderr,
		OpenURL: func(u string) error {
			opened <- u
			// Follow authorize URL which redirects to our loopback.
			go func() {
				client := &http.Client{
					CheckRedirect: func(req *http.Request, via []*http.Request) error {
						return http.ErrUseLastResponse
					},
				}
				resp, err := client.Get(u)
				if err != nil {
					t.Errorf("open authorize: %v", err)
					return
				}
				defer resp.Body.Close()
				loc := resp.Header.Get("Location")
				if loc == "" {
					t.Error("missing redirect location")
					return
				}
				resp2, err := http.Get(loc)
				if err != nil {
					t.Errorf("callback: %v", err)
					return
				}
				resp2.Body.Close()
			}()
			return nil
		},
		Now:          func() time.Time { return time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC) },
		CallbackWait: 5 * time.Second,
	})
	if err != nil {
		t.Fatalf("login: %v\nstderr=%s", err, stderr.String())
	}
	if res.Profile != "main" {
		t.Fatalf("profile %q", res.Profile)
	}
	tok, err := store.Get("main")
	if err != nil {
		t.Fatal(err)
	}
	if tok != "synthetic_test_access_token_do_not_use" {
		t.Fatalf("stored token %q", tok)
	}

	cfgPath := filepath.Join(dir, config.ConfigFile)
	raw, err := os.ReadFile(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "synthetic_test_access_token") || strings.Contains(string(raw), "synthetic-client-secret") {
		t.Fatalf("config leaked secret: %s", raw)
	}
	var cfg config.File
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg.Profiles["main"].ClientID != "test-client-id" {
		t.Fatal("client id missing from config")
	}

	combined := stderr.String() + tok
	if secrets.ContainsSecret(stderr.String()) {
		t.Fatalf("stderr leaked secret: %s", stderr.String())
	}
	_ = combined
	_ = authorizeHits
}

func TestLoginMissingScopeDoesNotStore(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)

	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserve.Addr().String()
	_ = reserve.Close()
	redirectURI := "http://" + addr + "/callback"

	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		cb := q.Get("redirect_uri") + "?code=c&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, cb, http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(loadFixture(t, "oauth-token-missing-scope.json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	store := NewMemoryStore()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _, err = Login(ctx, LoginOptions{
		Profile: "main", ClientID: "cid", RedirectURI: redirectURI, Scopes: DefaultScopes,
		AuthorizeURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token",
		HTTPClient: srv.Client(), TokenStore: store,
		SecretSource: SecretSource{Env: func(string) string { return "sec" }, IsTTY: func() bool { return false }, Stderr: io.Discard},
		Stderr:       io.Discard,
		OpenURL: func(u string) error {
			go func() {
				c := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
				resp, err := c.Get(u)
				if err != nil {
					return
				}
				loc := resp.Header.Get("Location")
				resp.Body.Close()
				r2, err := http.Get(loc)
				if err == nil {
					r2.Body.Close()
				}
			}()
			return nil
		},
		CallbackWait: 5 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "missing required scopes") {
		t.Fatalf("expected missing scope, got %v", err)
	}
	if _, gerr := store.Get("main"); gerr == nil {
		t.Fatal("token must not be stored when scopes missing")
	}
}

func TestLoginWrongStateNoExchange(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)

	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := reserve.Addr().String()
	_ = reserve.Close()
	redirectURI := "http://" + addr + "/callback"

	exchanges := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		cb := q.Get("redirect_uri") + "?code=c&state=forged-state"
		http.Redirect(w, r, cb, http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		exchanges++
		w.WriteHeader(200)
		_, _ = w.Write(loadFixture(t, "oauth-token-ok.json"))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	store := NewMemoryStore()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_, _, err = Login(ctx, LoginOptions{
		Profile: "main", ClientID: "cid", RedirectURI: redirectURI,
		AuthorizeURL: srv.URL + "/oauth/authorize", TokenURL: srv.URL + "/oauth/token",
		HTTPClient: srv.Client(), TokenStore: store,
		SecretSource: SecretSource{Env: func(string) string { return "sec" }, IsTTY: func() bool { return false }, Stderr: io.Discard},
		Stderr:       io.Discard,
		OpenURL: func(u string) error {
			go func() {
				c := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
				resp, err := c.Get(u)
				if err != nil {
					return
				}
				loc := resp.Header.Get("Location")
				resp.Body.Close()
				r2, err := http.Get(loc)
				if err == nil {
					r2.Body.Close()
				}
			}()
			return nil
		},
		CallbackWait: 5 * time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "state mismatch") {
		t.Fatalf("expected state mismatch, got %v", err)
	}
	if exchanges != 0 {
		t.Fatalf("token exchange must not run on state mismatch, ran %d", exchanges)
	}
}
