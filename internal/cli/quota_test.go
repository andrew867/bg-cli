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
	"github.com/andrew867/bg-cli/internal/output"
)

func TestAccountQuotaSummary(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account/profile" {
			t.Errorf("path %s", r.URL.Path)
		}
		w.Header().Set("X-RateLimit-Limit", "48")
		w.Header().Set("X-RateLimit-Remaining", "45")
		w.Header().Set("Retry-After", "12")
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
		IsTTY: func() bool { return true },
	}, []string{"account", "quota", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s %s", code, stderr.String(), stdout.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Command != "account quota" || !env.OK {
		t.Fatalf("%+v", env)
	}
	data := env.Data.(map[string]any)
	if data["refreshPresent"] != true {
		t.Fatalf("%#v", data)
	}
	// used from refresh fixture is 4
	switch u := data["used"].(type) {
	case float64:
		if u != 4 {
			t.Fatalf("used=%v", u)
		}
	case json.Number:
		if u != "4" {
			t.Fatalf("used=%v", u)
		}
	default:
		t.Fatalf("used type %T", data["used"])
	}
	if data["remaining"] != "45" {
		t.Fatalf("remaining=%v", data["remaining"])
	}
	if data["retryAfter"] != "12" {
		t.Fatalf("retryAfter=%v", data["retryAfter"])
	}
	if strings.Contains(stdout.String(), "Diamonds") {
		t.Fatal("quota must not dump currency/profile body")
	}
}

func TestAccountInventoryCSVJoinRAP(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	invBody, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json"))
	rapBody, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "rap-ok.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(invBody)
		case "/api/rap":
			w.Header().Set("Cache-Control", "public, max-age=60")
			_, _ = w.Write(rapBody)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	t.Setenv("BG_CLI_CACHE_DIR", t.TempDir())

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
		IsTTY: func() bool { return true },
	}, []string{"--output", "csv", "account", "inventory", "--profile", "main", "--join-rap"})
	if code != 0 {
		t.Fatalf("%d %s %s", code, stderr.String(), stdout.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "marketRap,marketRapMatched") {
		t.Fatalf("missing join header:\n%s", out)
	}
	if !strings.Contains(out, ",1250,true") {
		t.Fatalf("expected joined market RAP:\n%s", out)
	}
}
