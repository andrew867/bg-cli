package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestGetInventoryBearerOnlyExactPath(t *testing.T) {
	var gotAuth, gotPath string
	var gotHost string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		gotPath = r.URL.Path
		gotHost = r.Host
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-RateLimit-Limit", "48")
		w.Header().Set("X-RateLimit-Remaining", "45")
		_, _ = w.Write(fixture(t, "account-inventory-ok.json"))
	}))
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	res, err := c.GetInventory(context.Background(), "synthetic_test_access_token_do_not_use", AccountGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if gotAuth != "Bearer synthetic_test_access_token_do_not_use" {
		t.Fatalf("auth %q", gotAuth)
	}
	if gotPath != "/v1/account/inventory" {
		t.Fatalf("path %q host %q", gotPath, gotHost)
	}
	if res.HTTPStatus != 200 {
		t.Fatal(res.HTTPStatus)
	}
	snap, cached := SnapshotFromInventory(res.Data)
	if snap == nil || snap.UTC().Format(time.RFC3339) != "2026-11-06T12:00:00Z" {
		t.Fatalf("snapshot %#v", snap)
	}
	if cached == nil || !*cached {
		t.Fatal("cached")
	}
}

func TestAccountRejectsRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(fixture(t, "account-inventory-ok.json"))
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/v1/account/inventory", http.StatusFound)
	}))
	defer redir.Close()

	c := &Client{BaseURL: redir.URL, HTTPClient: redir.Client()}
	_, err := c.GetInventory(context.Background(), "tok", AccountGetOptions{})
	if err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("expected redirect error, got %v", err)
	}
}

func TestExtractCurrenciesMissingAndPresent(t *testing.T) {
	ext, err := ExtractCurrencies(json.RawMessage(`{"Rank":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if ext.CurrencyFieldPresent {
		t.Fatal("expected missing")
	}
	ext, err = ExtractCurrencies(json.RawMessage(`{"Currency":{}}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ext.CurrencyFieldPresent || ext.EntryCount != 0 {
		t.Fatalf("%+v", ext)
	}
	body := fixture(t, "account-profile-ok.json")
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &env)
	ext, err = ExtractCurrencies(env.Data)
	if err != nil {
		t.Fatal(err)
	}
	if !ext.CurrencyFieldPresent || ext.EntryCount != 2 {
		t.Fatalf("%+v", ext)
	}
	var diamonds map[string]any
	_ = json.Unmarshal(ext.Currencies["Diamonds"], &diamonds)
	if diamonds["id"] != "Diamonds" {
		t.Fatal(diamonds)
	}
	// large number preserved as json.Number-ish via RawMessage
	if !strings.Contains(string(ext.Currencies["Diamonds"]), "123456789") {
		t.Fatal(string(ext.Currencies["Diamonds"]))
	}
}

func TestFilterInventory(t *testing.T) {
	body := fixture(t, "account-inventory-ok.json")
	var env struct {
		Data json.RawMessage `json:"data"`
	}
	_ = json.Unmarshal(body, &env)
	res, err := FilterInventory(env.Data, "pet", "test cat", false)
	if err != nil {
		t.Fatal(err)
	}
	if res.RowCount != 1 || res.QuantitySum != "3" || res.PetQuantitySum != "3" || res.PetRowCount != 1 {
		t.Fatalf("%+v", res)
	}
	empty, err := FilterInventory(env.Data, "Weapon", "", false)
	if err != nil || empty.RowCount != 0 {
		t.Fatalf("rows=%d err=%v", empty.RowCount, err)
	}
}

func TestUnauthorizedMappingFixture(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(401)
		_, _ = w.Write(fixture(t, "account-unauthorized.json"))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	res, err := c.GetProfile(context.Background(), "tok", AccountGetOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.HTTPStatus != 401 || res.ErrorMsg == "" {
		t.Fatalf("%+v", res)
	}
}

func TestPublicNeverSendsBearer(t *testing.T) {
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{"status":"ok","data":[]}`))
	}))
	defer srv.Close()
	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	_, err := c.GetPublic(context.Background(), "/v1/leaderboards", nil, BodyLimitDefault)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		t.Fatalf("public request leaked auth: %q", auth)
	}
}
