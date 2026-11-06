package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestAccountLoadoutAnalyzeOfflineManifest(t *testing.T) {
	dir := t.TempDir()
	invBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-loadout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var top map[string]json.RawMessage
	_ = json.Unmarshal(invBody, &top)
	var dataObj map[string]any
	_ = json.Unmarshal(top["data"], &dataObj)
	invEnv := map[string]any{
		"schemaVersion": "1.0", "ok": true, "command": "account inventory",
		"source": map[string]any{
			"endpoint": "/v1/account/inventory", "httpStatus": 200, "retrievedAt": "2026-11-06T15:00:00Z",
			"fromLocalCache": false, "provider": "BIG Games", "classification": "official", "baseUrl": "https://example",
		},
		"freshness": map[string]any{"snapshotAt": "2026-11-06T15:00:00.000Z", "meaning": "cached"},
		"data":      map[string]any{"inventory": dataObj},
	}
	invName := "test-ps99-inv.json"
	invPath := filepath.Join(dir, invName)
	b, _ := json.MarshalIndent(invEnv, "", "  ")
	if err := os.WriteFile(invPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	snap := "2026-11-06T15:00:00.000Z"
	man := snapshot.Manifest{
		SchemaVersion: snapshot.ManifestSchemaVersion,
		Command:       "account snapshot",
		OK:            true,
		LocalStamp:    "test",
		ExportedAt:    "2026-11-06T15:00:00Z",
		OutputDir:     dir,
		Prefix:        "ps99",
		ManifestFile:  "test-ps99-manifest.json",
		Files: []snapshot.FileRecord{
			{Name: invName, RelPath: invName, Kind: "inventory-json", OK: true, Bytes: int64(len(b)), SHA256: hex.EncodeToString(sum[:]), SnapshotAt: &snap},
		},
	}
	manPath := filepath.Join(dir, man.ManifestFile)
	mb, _ := json.MarshalIndent(man, "", "  ")
	if err := os.WriteFile(manPath, mb, 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{
		"account", "loadout", "analyze",
		"--manifest", manPath,
		"--world", "halloween-games",
		"--objective", "max-pet-damage",
		"--pretty",
	})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "scoreFullyResolved") {
		t.Fatalf("missing estimate fields: %s", stdout.String())
	}
	if strings.Contains(stdout.String(), `"scoreFullyResolved": true`) {
		t.Fatal("must not claim fully resolved invented score")
	}
}

func TestAccountLoadoutOptimizeLiveInventory(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	body, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-loadout.json"))
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/account/inventory" {
			t.Errorf("path %s", r.URL.Path)
		}
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(), IsTTY: func() bool { return true },
	}, []string{"account", "loadout", "optimize", "--profile", "main", "--world", "general", "--objective", "max-pet-damage", "--pretty"})
	if code != 0 {
		t.Fatalf("exit %d %s %s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "candidates") {
		t.Fatal(stdout.String())
	}
}
