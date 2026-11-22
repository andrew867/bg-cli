package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestAuthStatusLocalNoNetwork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias:     "main",
				ClientID:  "cid",
				Scopes:    auth.DefaultScopes,
				IssuedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 11, 20, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", "synthetic_test_access_token_do_not_use")

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		TokenStore: store,
		Clock:      output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}, []string{"auth", "status", "--profile", "main"})
	if code != exitcode.OK {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "synthetic_test_access_token") {
		t.Fatal("token leaked to stdout")
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Command != "auth status" {
		t.Fatalf("%+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data["tokenPresent"] != true {
		t.Fatalf("data %#v", data)
	}
	if data["expired"] != false {
		t.Fatalf("injected clock is before expiry, expired=%#v", data["expired"])
	}
}

func TestAuthStatusExpiryUsesInjectedClock(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias:     "main",
				ClientID:  "cid",
				ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		TokenStore: auth.NewMemoryStore(),
		Clock:      output.FixedClock{T: time.Date(2026, 12, 15, 0, 0, 0, 0, time.UTC)},
	}, []string{"auth", "status", "--profile", "main"})
	if code != exitcode.OK {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data, _ := env.Data.(map[string]any)
	if data["expired"] != true {
		t.Fatalf("injected clock is after expiry, expired=%#v", data["expired"])
	}
}

func TestAuthLogout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {Alias: "main", ClientID: "cid", Scopes: auth.DefaultScopes, ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", "tok")
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: store}, []string{"auth", "logout", "--profile", "main"})
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	if _, err := store.Get("main"); err == nil {
		t.Fatal("token still present")
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Profiles["main"]; ok {
		t.Fatal("profile metadata remain")
	}
}

func TestNoClientSecretFlag(t *testing.T) {
	root := NewRoot(Deps{Stdout: ioDiscard{}, Stderr: ioDiscard{}})
	for _, c := range root.Commands() {
		if c.Name() != "auth" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() != "login" {
				continue
			}
			if sub.Flags().Lookup("client-secret") != nil || sub.Flags().Lookup("secret") != nil {
				t.Fatal("client secret must not be a flag")
			}
			if sub.PersistentFlags().Lookup("client-secret") != nil {
				t.Fatal("client secret must not be a persistent flag")
			}
		}
	}
}

func TestAuthRawRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: auth.NewMemoryStore()},
		[]string{"--output", "raw", "auth", "profiles"})
	if code != exitcode.Usage {
		t.Fatalf("exit %d", code)
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{"version"})
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout.String(), Version) {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestConfigNeverContainsTokenAfterManualWriteScan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{Profiles: map[string]config.Profile{"main": {Alias: "main", ClientID: "cid"}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"access_token", "client_secret", "ClientSecret", "Bearer "}
	for _, f := range forbidden {
		if strings.Contains(string(raw), f) {
			t.Fatalf("config contains %q", f)
		}
	}
}

func TestAuthProfileSetAndUnset(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {Alias: "main", ClientID: "cid", RedirectURI: "http://127.0.0.1:8765/callback"},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{
		"auth", "profile", "set", "--profile", "main", "--ps99-user-id", "1234", "--clan-tag", "SYN",
	})
	if code != 0 {
		t.Fatalf("set exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := loaded.Profiles["main"]
	if p.PS99UserID != "1234" || p.PS99ClanTag != "SYN" || p.ClientID != "cid" || p.RedirectURI == "" {
		t.Fatalf("profile %+v", p)
	}
	if strings.Contains(string(mustRead(t, filepath.Join(dir, config.ConfigFile))), "access_token") {
		t.Fatal("token leaked into config")
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{
		"auth", "profile", "set", "--profile", "main", "--clan-tag", "SYN", "--dry-run",
	})
	if code != 0 {
		t.Fatalf("dry-run exit %d stderr=%s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"dryRun": true`) && !strings.Contains(stdout.String(), `"dryRun":true`) {
		t.Fatalf("stdout %s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{
		"auth", "profile", "unset", "--profile", "main", "--clan-tag",
	})
	if code != 0 {
		t.Fatalf("unset exit %d stderr=%s", code, stderr.String())
	}
	loaded, err = config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p = loaded.Profiles["main"]
	if p.PS99UserID != "1234" || p.PS99ClanTag != "" {
		t.Fatalf("after unset %+v", p)
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{"auth", "profile", "set", "--profile", "main"})
	if code != exitcode.Usage || !strings.Contains(stderr.String()+stdout.String(), "auth profile set --profile main") {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestAuthLoginReauthStoresSeparateToken(t *testing.T) {
	const mainToken = "synthetic_main_client_token"
	const altToken = "synthetic_alt_reauth_token"
	const clientSecret = "synthetic-client-secret"

	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	redirectURI := "http://" + reserve.Addr().String() + "/callback"
	_ = reserve.Close()

	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	if err := config.Save(&config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias: "main", ClientID: "same-client", Scopes: auth.DefaultScopes,
				RedirectURI: redirectURI, AccountID: "111", PS99UserID: "111",
				IssuedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			},
			"alt1": {
				Alias: "alt1", ClientID: "same-client", RedirectURI: "http://127.0.0.1:8765/callback",
				AccountID: "222", PS99UserID: "222", PS99ClanTag: "ALT",
			},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	if err := store.Set("main", mainToken); err != nil {
		t.Fatal(err)
	}

	var opened string
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "same-client" {
			t.Errorf("client_id %q", q.Get("client_id"))
		}
		if strings.Contains(r.URL.RawQuery, mainToken) || strings.Contains(r.URL.RawQuery, clientSecret) {
			t.Error("authorize URL contained a secret or the other profile's token")
		}
		cb := q.Get("redirect_uri") + "?code=" + url.QueryEscape("synthetic-auth-code") + "&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, cb, http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":2592000,"scope":%q}`, altToken, auth.ScopeString(auth.DefaultScopes))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		HTTPClient:        srv.Client(),
		BaseURL:           srv.URL,
		OAuthAuthorizeURL: srv.URL + "/oauth/authorize",
		OAuthTokenURL:     srv.URL + "/oauth/token",
		Env: func(k string) string {
			if k == auth.ClientSecretEnv {
				return clientSecret
			}
			return ""
		},
		IsTTY: func() bool { return false },
		OpenURL: func(u string) error {
			opened = u
			go func() {
				c := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
				resp, err := c.Get(u)
				if err != nil {
					return
				}
				loc := resp.Header.Get("Location")
				resp.Body.Close()
				if r2, err := http.Get(loc); err == nil {
					r2.Body.Close()
				}
			}()
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) },
	}, []string{"auth", "login", "--profile", "alt1"})
	if code != 0 {
		t.Fatalf("login exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(opened, mainToken) || strings.Contains(stdout.String()+stderr.String(), mainToken) || strings.Contains(stdout.String()+stderr.String(), altToken) || strings.Contains(stdout.String()+stderr.String(), clientSecret) {
		t.Fatal("token or client secret was printed or copied into the authorize URL")
	}
	gotMain, err := store.Get("main")
	if err != nil || gotMain != mainToken {
		t.Fatalf("main token changed: %q err=%v", gotMain, err)
	}
	gotAlt, err := store.Get("alt1")
	if err != nil {
		t.Fatal(err)
	}
	if gotAlt != altToken || gotAlt == mainToken {
		t.Fatalf("alt token = %q", gotAlt)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	alt := loaded.Profiles["alt1"]
	if alt.AccountID != "222" || alt.PS99UserID != "222" || alt.ClientID != "same-client" || alt.PS99ClanTag != "ALT" {
		t.Fatalf("reauth rewrote alt identity: %+v", alt)
	}
	raw, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), mainToken) || strings.Contains(string(raw), altToken) || strings.Contains(string(raw), clientSecret) {
		t.Fatal("secret written into config")
	}
}

func TestAuthLoginAltInheritsDeveloperApp(t *testing.T) {
	const mainToken = "synthetic_main_client_token"
	const altToken = "synthetic_alt_inherited_token"
	const clientSecret = "synthetic-client-secret"

	reserve, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	redirectURI := "http://" + reserve.Addr().String() + "/callback"
	_ = reserve.Close()

	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	if err := config.Save(&config.File{
		DefaultProfile: "main",
		OAuth:          config.OAuthApp{ClientID: "same-client", RedirectURI: redirectURI},
		Profiles: map[string]config.Profile{
			"main": {
				Alias: "main", ClientID: "same-client", Scopes: auth.DefaultScopes,
				RedirectURI: redirectURI, PS99UserID: "111", CredentialEpoch: "main-epoch",
			},
			"alt1": {Alias: "alt1", PS99UserID: "222", PS99ClanTag: "ALT"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	if err := store.Set("main", mainToken); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/oauth/authorize", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		if q.Get("client_id") != "same-client" || q.Get("redirect_uri") != redirectURI {
			t.Errorf("app client %q redirect %q", q.Get("client_id"), q.Get("redirect_uri"))
		}
		cb := q.Get("redirect_uri") + "?code=" + url.QueryEscape("synthetic-auth-code") + "&state=" + url.QueryEscape(q.Get("state"))
		http.Redirect(w, r, cb, http.StatusFound)
	})
	mux.HandleFunc("/oauth/token", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"access_token":%q,"token_type":"Bearer","expires_in":2592000,"scope":%q}`, altToken, auth.ScopeString(auth.DefaultScopes))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		HTTPClient:        srv.Client(),
		BaseURL:           srv.URL,
		OAuthAuthorizeURL: srv.URL + "/oauth/authorize",
		OAuthTokenURL:     srv.URL + "/oauth/token",
		Env: func(k string) string {
			if k == auth.ClientSecretEnv {
				return clientSecret
			}
			return ""
		},
		IsTTY: func() bool { return false },
		OpenURL: func(u string) error {
			go func() {
				c := &http.Client{CheckRedirect: func(req *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}
				resp, err := c.Get(u)
				if err != nil {
					return
				}
				loc := resp.Header.Get("Location")
				resp.Body.Close()
				if r2, err := http.Get(loc); err == nil {
					r2.Body.Close()
				}
			}()
			return nil
		},
		Now: func() time.Time { return time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC) },
	}, []string{"auth", "login", "--profile", "alt1"})
	if code != 0 {
		t.Fatalf("login exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String()+stderr.String(), mainToken) || strings.Contains(stdout.String()+stderr.String(), altToken) || strings.Contains(stdout.String()+stderr.String(), clientSecret) {
		t.Fatal("token or client secret was printed")
	}
	if strings.Contains(stderr.String(), "grant matches") || !strings.Contains(stderr.String(), "consent screen") {
		t.Fatalf("stderr=%s", stderr.String())
	}
	gotMain, err := store.Get("main")
	if err != nil || gotMain != mainToken {
		t.Fatalf("main token changed: %q err=%v", gotMain, err)
	}
	gotAlt, err := store.Get("alt1")
	if err != nil || gotAlt != altToken {
		t.Fatalf("alt token %q err=%v", gotAlt, err)
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	alt := loaded.Profiles["alt1"]
	if alt.PS99UserID != "222" || alt.PS99ClanTag != "ALT" || alt.ClientID != "same-client" || alt.RedirectURI != redirectURI {
		t.Fatalf("alt profile %+v", alt)
	}
	if alt.CredentialEpoch == "" || alt.CredentialEpoch == "main-epoch" || strings.Contains(alt.CredentialEpoch, altToken) {
		t.Fatalf("epoch %q", alt.CredentialEpoch)
	}
	if loaded.Profiles["main"].CredentialEpoch != "main-epoch" {
		t.Fatal("main epoch changed")
	}
	if loaded.OAuth.ClientID != "same-client" || loaded.OAuth.RedirectURI != redirectURI {
		t.Fatalf("oauth %+v", loaded.OAuth)
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
