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

func TestAccountSnapshotWritesFilesAndManifest(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	invBody, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json"))
	profBody, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	extBody, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(invBody)
		case "/v1/account/profile":
			_, _ = w.Write(profBody)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(extBody)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.FixedZone("NST", -3*3600-1800))
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed.UTC()},
		IsTTY: func() bool { return true },
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	// FixedClock returns UTC; nowLocal converts to Local for stamp.
	stamp = fixed.UTC().In(time.Local).Format("2006-01-02-150405")

	wantFiles := []string{
		stamp + "-ps99-inv.json",
		stamp + "-ps99-prof.json",
		stamp + "-ps99-extprof.json",
		stamp + "-ps99-inventory.csv",
		stamp + "-ps99-currencies.csv",
		stamp + "-ps99-manifest.json",
	}
	for _, name := range wantFiles {
		path := filepath.Join(outDir, name)
		fi, err := os.Stat(path)
		if err != nil {
			t.Fatalf("missing %s: %v (dir=%v)", name, err, listNames(t, outDir))
		}
		if fi.Size() == 0 {
			t.Fatalf("empty %s", name)
		}
	}

	manBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man map[string]any
	if err := json.Unmarshal(manBytes, &man); err != nil {
		t.Fatal(err)
	}
	if man["ok"] != true {
		t.Fatalf("manifest not ok: %s", manBytes)
	}
	summary := man["summary"].(map[string]any)
	inv := summary["inventory"].(map[string]any)
	if inv["petQuantitySum"] == nil {
		t.Fatalf("missing petQuantitySum in manifest: %v", summary)
	}
	if inv["snapshotAt"] == nil {
		t.Fatalf("missing inventory snapshotAt: %v", inv)
	}
	prof := summary["profile"].(map[string]any)
	currencies := prof["currencies"].(map[string]any)
	if currencies["Diamonds"] != "123456789" {
		t.Fatalf("currency exact amount: %v", currencies)
	}
	stats := prof["statistics"].(map[string]any)
	if stats["Eggs Opened"] != "42" {
		t.Fatalf("statistics: %v", stats)
	}

	csvBytes, _ := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-inventory.csv"))
	if !strings.Contains(string(csvBytes), "variant,golden,rainbow,shiny") {
		t.Fatalf("inventory csv missing variants:\n%s", csvBytes)
	}

	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Command != "account snapshot" || !env.OK {
		t.Fatalf("%+v", env)
	}
}

func listNames(t *testing.T, dir string) []string {
	t.Helper()
	ents, _ := os.ReadDir(dir)
	var names []string
	for _, e := range ents {
		names = append(names, e.Name())
	}
	return names
}
