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

func TestAccountInventoryCSVExactCount(t *testing.T) {
	const huge = "9007199254740993"
	body := []byte(`{
  "status":"ok",
  "data":{
    "items":[{"class":"Pet","id":"Huge","count":` + huge + `,"displayName":"Huge","rap":1,"rapApproximate":false,"exists":2,"rarity":"Basic","rarityNumber":1,"collection":"Pets"}],
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
	}, []string{"--output", "csv", "account", "inventory", "--profile", "main"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	out := stdout.String()
	if !strings.Contains(out, "class,id,count,") {
		t.Fatalf("missing CSV header:\n%s", out)
	}
	if !strings.Contains(out, ","+huge+",") {
		t.Fatalf("exact count missing from CSV:\n%s", out)
	}
	if strings.Contains(out, "schemaVersion") {
		t.Fatal("CSV mode should not wrap in JSON envelope")
	}
}

func TestAccountCurrenciesCSVExactAM(t *testing.T) {
	const huge = "9007199254740993"
	body := []byte(`{"status":"ok","data":{"Currency":{"Diamonds":{"id":"Diamonds","_am":` + huge + `}}},"refresh":null}`)
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
	}, []string{"--output", "csv", "account", "currencies", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), huge) {
		t.Fatalf("exact _am missing:\n%s", stdout.String())
	}
}

func TestAccountInventoryTableIncludesColumns(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	srv := accountServer(t, "/v1/account/inventory", 200, "account-inventory-ok.json", nil)
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
		IsTTY: func() bool { return true },
	}, []string{"--output", "table", "account", "inventory", "--profile", "main"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	out := stdout.String()
	for _, want := range []string{"command\taccount inventory", "displayName", "Test Cat", "1250", "Pets"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in:\n%s", want, out)
		}
	}
	if !strings.Contains(out, "class") || !strings.Contains(out, "count") {
		t.Fatalf("missing inventory column headers:\n%s", out)
	}
}

func TestDoctorHealthyWithMemoryStore(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		Clock: output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}, []string{"doctor"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Command != "doctor" || !env.OK {
		t.Fatalf("%+v", env)
	}
	data := env.Data.(map[string]any)
	if data["healthy"] != true {
		t.Fatalf("expected healthy: %#v", data)
	}
	if _, err := store.Get("doctorprobe"); err == nil {
		t.Fatal("doctor probe token must be deleted")
	}
}

func TestCompletionBashGeneratesScript(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{"completion", "bash"})
	if code != 0 {
		t.Fatalf("%d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "bg-cli") {
		t.Fatalf("unexpected completion output: %s", stdout.String()[:min(200, len(stdout.String()))])
	}
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
