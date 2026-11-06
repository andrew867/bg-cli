package cli

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/output"
)

func TestInventoryPreservesLargeIntegerCounts(t *testing.T) {
	const huge = "9007199254740993" // 2^53+1, not exact in float64
	body := []byte(`{
  "status":"ok",
  "data":{
    "items":[{"class":"Pet","id":"Huge","count":` + huge + `}],
    "equipped":{},
    "fetchedAt":"2026-11-06T12:00:00.000Z",
    "cached":false
  },
  "refresh":{"quotaExhausted":false,"used":1,"limit":48}
}`)
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
		IsTTY: func() bool { return true },
	}, []string{"account", "inventory", "--profile", "main"})
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, `"count":`+huge) && !strings.Contains(out, `"count": `+huge) {
		t.Fatalf("large count altered or missing:\n%s", out)
	}
	if !strings.Contains(out, `"quantitySum":`+huge) && !strings.Contains(out, `"quantitySum": `+huge) {
		t.Fatalf("quantitySum altered:\n%s", out)
	}
	// Ensure float rounding did not silently happen to 9007199254740992
	if strings.Contains(out, "9007199254740992") {
		t.Fatal("count appears rounded to float64 mantissa")
	}
}

func TestProfilePreservesLargeStatistic(t *testing.T) {
	const huge = "9007199254740993"
	body := []byte(`{"status":"ok","data":{"Statistics":{"Eggs Opened":` + huge + `}},"refresh":null}`)
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "profile", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), huge) {
		t.Fatalf("large statistic lost:\n%s", stdout.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
}

func TestPublicRejectsCrossHostRedirect(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","data":{"leaderboards":[]}}`))
	}))
	defer final.Close()
	redir := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, final.URL+"/v1/leaderboards", http.StatusFound)
	}))
	defer redir.Close()
	t.Setenv("BG_CLI_CACHE_DIR", t.TempDir())
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, BaseURL: redir.URL, HTTPClient: redir.Client()},
		[]string{"leaderboard", "list"})
	if code == 0 {
		t.Fatalf("expected failure on cross-host redirect, got success: %s", stdout.String())
	}
	if !strings.Contains(stderr.String()+stdout.String(), "redirect") &&
		!strings.Contains(stderr.String()+stdout.String(), "same host") {
		t.Fatalf("expected redirect error, got exit=%d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}
