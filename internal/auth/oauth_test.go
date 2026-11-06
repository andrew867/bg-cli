package auth

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	path := filepath.Join("..", "..", "testdata", "fixtures", name)
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestExchangeCodeOK(t *testing.T) {
	body := loadFixture(t, "oauth-token-ok.json")
	var gotAuth string
	var gotForm url.Values
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method %s", r.Method)
		}
		gotAuth = r.Header.Get("Authorization")
		b, _ := io.ReadAll(r.Body)
		gotForm, _ = url.ParseQuery(string(b))
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	tr, err := ExchangeCode(context.Background(), ExchangeParams{
		TokenURL:     srv.URL,
		ClientID:     "cid",
		ClientSecret: "csec",
		Code:         "one-time-code",
		RedirectURI:  "http://127.0.0.1:8765/callback",
		Verifier:     "verifier",
		HTTPClient:   srv.Client(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if tr.AccessToken != "synthetic_test_access_token_do_not_use" {
		t.Fatalf("token %q", tr.AccessToken)
	}
	wantBasic := "Basic " + base64.StdEncoding.EncodeToString([]byte("cid:csec"))
	if gotAuth != wantBasic {
		t.Fatalf("auth header %q", gotAuth)
	}
	if gotForm.Get("grant_type") != "authorization_code" {
		t.Fatal("grant_type")
	}
	if gotForm.Get("code") != "one-time-code" || gotForm.Get("code_verifier") != "verifier" {
		t.Fatalf("form %#v", gotForm)
	}
}

func TestExchangeCodeRejectsRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write(loadFixture(t, "oauth-token-ok.json"))
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL, http.StatusFound)
	}))
	defer redir.Close()

	_, err := ExchangeCode(context.Background(), ExchangeParams{
		TokenURL:     redir.URL,
		ClientID:     "cid",
		ClientSecret: "csec",
		Code:         "c",
		RedirectURI:  "http://127.0.0.1:8765/callback",
		Verifier:     "v",
	})
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("expected redirect error, got %v", err)
	}
}

func TestExchangeCodeInvalidGrant(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(400)
		_, _ = w.Write(loadFixture(t, "oauth-invalid-grant.json"))
	}))
	defer srv.Close()
	_, err := ExchangeCode(context.Background(), ExchangeParams{
		TokenURL: srv.URL, ClientID: "c", ClientSecret: "s", Code: "x",
		RedirectURI: "http://127.0.0.1:1/cb", Verifier: "v", HTTPClient: srv.Client(),
	})
	if err == nil || !strings.Contains(err.Error(), "invalid_grant") {
		t.Fatalf("got %v", err)
	}
}

func TestValidateGrantedScopesMissing(t *testing.T) {
	body := loadFixture(t, "oauth-token-missing-scope.json")
	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		t.Fatal(err)
	}
	err := ValidateGrantedScopes(tr, DefaultScopes)
	if err == nil {
		t.Fatal("expected missing scope error")
	}
}

func TestBuildAuthorizeURL(t *testing.T) {
	u, err := BuildAuthorizeURL(AuthorizeParams{
		ClientID:      "cid",
		RedirectURI:   "http://127.0.0.1:8765/callback",
		Scopes:        DefaultScopes,
		State:         "st",
		CodeChallenge: "ch",
		AuthorizeURL:  "https://db.biggames.io/oauth/authorize",
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := url.Parse(u)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	if q.Get("code_challenge_method") != "S256" {
		t.Fatal("method")
	}
	if q.Get("redirect_uri") != "http://127.0.0.1:8765/callback" {
		t.Fatal("redirect")
	}
	if !strings.Contains(q.Get("scope"), "inventory:read") {
		t.Fatal("scopes")
	}
}

func TestExchangeNoAutomaticRetry(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(503)
		_, _ = w.Write([]byte(`{"error":"unavailable"}`))
	}))
	defer srv.Close()
	_, err := ExchangeCode(context.Background(), ExchangeParams{
		TokenURL: srv.URL, ClientID: "c", ClientSecret: "s", Code: "x",
		RedirectURI: "http://127.0.0.1:1/cb", Verifier: "v",
		HTTPClient: &http.Client{Timeout: 2 * time.Second},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if calls != 1 {
		t.Fatalf("expected single attempt, got %d", calls)
	}
}
