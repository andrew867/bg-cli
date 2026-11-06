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
	"github.com/andrew867/bg-cli/internal/snapshot"
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
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
		IsTTY: func() bool { return true },
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--note", "test run"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}

	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	manPath := filepath.Join(outDir, stamp+"-ps99-manifest.json")
	manBytes, err := os.ReadFile(manPath)
	if err != nil {
		t.Fatalf("manifest missing: %v files=%v", err, listNames(t, outDir))
	}
	var man snapshot.Manifest
	if err := json.Unmarshal(manBytes, &man); err != nil {
		t.Fatal(err)
	}
	if man.SchemaVersion != snapshot.ManifestSchemaVersion {
		t.Fatalf("schema=%s", man.SchemaVersion)
	}
	if man.Note != "test run" {
		t.Fatalf("note=%q", man.Note)
	}
	if man.ManifestFile == "" {
		t.Fatal("missing ManifestFile pointer")
	}
	for _, f := range man.Files {
		if f.Kind == "manifest" {
			t.Fatal("manifest must not list itself as a sized payload entry")
		}
		if f.OK && f.SHA256 == "" {
			t.Fatalf("missing sha256 for %s", f.Name)
		}
	}
	if !man.OK {
		t.Fatalf("manifest not ok: %s", manBytes)
	}
}

func TestAccountSnapshotRejectsBadPrefix(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: store},
		[]string{"account", "snapshot", "--profile", "main", "--prefix", "../x"})
	if code == 0 {
		t.Fatal("expected failure")
	}
	if !strings.Contains(stderr.String()+stdout.String(), "prefix") {
		t.Fatalf("%s %s", stdout.String(), stderr.String())
	}
}

func TestAccountSnapshotCollisionSuffix(t *testing.T) {
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
		}
	}))
	defer srv.Close()
	outDir := t.TempDir()
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	deps := Deps{
		TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed}, IsTTY: func() bool { return true },
	}
	run := func() int {
		var stdout, stderr bytes.Buffer
		deps.Stdout, deps.Stderr = &stdout, &stderr
		return Execute(deps, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
	}
	if code := run(); code != 0 {
		t.Fatalf("first snapshot failed: %d", code)
	}
	if code := run(); code != 0 {
		t.Fatalf("second snapshot failed: %d", code)
	}
	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-manifest.json")); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-0001-ps99-manifest.json")); err != nil {
		t.Fatalf("expected collision suffix file: %v names=%v", err, listNames(t, outDir))
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
