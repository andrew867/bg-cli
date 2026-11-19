package cli

import (
	"archive/zip"
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
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

var inactiveClanBattleBody = []byte(`{"status":"ok","data":{"configName":"FinishedBattle","category":"GuildBattles","configData":{"Title":"Finished","StartTime":1,"FinishTime":2,"Enabled":true}}}`)

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
		case "/api/activeClanBattle":
			if r.Header.Get("Authorization") != "" {
				t.Error("bearer sent to public clan battle")
			}
			_, _ = w.Write(inactiveClanBattleBody)
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
	sumPath := filepath.Join(outDir, stamp+"-ps99-summary.json")
	if _, err := os.Stat(sumPath); err != nil {
		t.Fatalf("summary missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-clanbattle.json")); !os.IsNotExist(err) {
		t.Fatal("inactive battle wrote clanbattle.json")
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
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
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

func TestAccountSnapshotHumanJSONAndZip(t *testing.T) {
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
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	deps := func() Deps {
		return Deps{
			TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed}, IsTTY: func() bool { return false },
		}
	}

	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, ".env"), []byte("BG_CLI_CLIENT_SECRET=nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "config.json"), []byte(`{"token":"nope"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	d := deps()
	d.Stdout, d.Stderr = &stdout, &stderr
	code := Execute(d, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--zip"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "\x1b") {
		t.Fatal("color on a non-TTY")
	}
	if strings.HasPrefix(strings.TrimSpace(stdout.String()), "{") || strings.Contains(stdout.String(), `"schemaVersion"`) {
		t.Fatalf("default stdout is JSON:\n%s", stdout.String())
	}
	for _, want := range []string{"PS99 snapshot", "Eggs opened", "Inventory", "Files", "sha256"} {
		if !strings.Contains(stdout.String(), want) {
			t.Fatalf("missing %q\n%s", want, stdout.String())
		}
	}
	if strings.Contains(stdout.String(), "BG_CLI_CLIENT_SECRET") || strings.Contains(stdout.String(), ".env") {
		t.Fatalf("secret leaked:\n%s", stdout.String())
	}
	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	zipPath := filepath.Join(outDir, stamp+"-ps99.zip")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
		if strings.Contains(f.Name, "..") || strings.ContainsAny(f.Name, `/\`) {
			t.Fatalf("unsafe member %s", f.Name)
		}
	}
	if !names[stamp+"-ps99-manifest.json"] || !names[stamp+"-ps99-inv.json"] || !names[stamp+"-ps99-summary.json"] || !names[stamp+"-ps99-inventory-diff.json"] {
		t.Fatalf("members=%v", names)
	}
	diffBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-inventory-diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(diffBytes), `"status": "unavailable"`) || strings.Contains(string(diffBytes), `"delta"`) {
		t.Fatalf("first snapshot invented an inventory delta: %s", diffBytes)
	}
	if names[stamp+"-ps99.zip"] || names[".env"] || names["config.json"] {
		t.Fatalf("unexpected members=%v", names)
	}
	manBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(manBytes), `"sha256"`) && strings.Contains(string(manBytes), stamp+"-ps99.zip") {
		t.Fatal("manifest lists the zip")
	}

	stdout.Reset()
	stderr.Reset()
	jsonDir := t.TempDir()
	d = deps()
	d.Stdout, d.Stderr = &stdout, &stderr
	code = Execute(d, []string{"--output", "json", "account", "snapshot", "--profile", "main", "--dir", jsonDir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("json exit %d %s", code, stderr.String())
	}
	if strings.Contains(stdout.String(), "PS99 snapshot") {
		t.Fatalf("human table in machine mode:\n%s", stdout.String())
	}
	var env map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if env["ok"] != true {
		t.Fatalf("envelope=%v", env["ok"])
	}

	stdout.Reset()
	stderr.Reset()
	d = deps()
	d.Stdout, d.Stderr = &stdout, &stderr
	code = Execute(d, []string{"account", "snapshot", "--output", "csv", "--profile", "main", "--dir", t.TempDir()})
	if code != exitcode.Usage {
		t.Fatalf("csv exit %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
}

func TestAccountSnapshotZipPathRefusesExisting(t *testing.T) {
	deps, closeSrv := snapshotFixtureDeps(t)
	defer closeSrv()
	outDir := t.TempDir()
	dest := filepath.Join(outDir, "existing.zip")
	const sentinel = "not-a-real-zip"
	if err := os.WriteFile(dest, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code := Execute(deps, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--zip", "--zip-path", "existing.zip"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if !strings.Contains(stdout.String(), "ZIP destination already exists") {
		t.Fatalf("human summary hid the ZIP failure:\n%s", stdout.String())
	}
	if !strings.Contains(stderr.String(), "ZIP destination already exists") {
		t.Fatalf("stderr=%s", stderr.String())
	}
	got, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Fatalf("destination replaced: %q", got)
	}
	stamp := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC).In(time.Local).Format("2006-01-02-150405")
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-manifest.json")); err != nil {
		t.Fatalf("snapshot files missing: %v names=%v", err, listNames(t, outDir))
	}
	assertNoZipTemp(t, outDir)

	jsonDir := t.TempDir()
	jsonDest := filepath.Join(jsonDir, "existing.zip")
	if err := os.WriteFile(jsonDest, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code = Execute(deps, []string{"--output", "json", "account", "snapshot", "--profile", "main", "--dir", jsonDir, "--prefix", "ps99", "--zip", "--zip-path", "existing.zip"})
	if code != 0 {
		t.Fatalf("json exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	var env struct {
		OK   bool `json:"ok"`
		Data struct {
			OK  bool `json:"ok"`
			Zip struct {
				OK    bool   `json:"ok"`
				Error string `json:"error"`
			} `json:"zip"`
		} `json:"data"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatalf("stdout is not JSON: %v\n%s", err, stdout.String())
	}
	if !env.OK || !env.Data.OK || env.Data.Zip.OK || !strings.Contains(env.Data.Zip.Error, "ZIP destination already exists") {
		t.Fatalf("envelope=%+v", env)
	}
	got, err = os.ReadFile(jsonDest)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Fatalf("json run replaced destination: %q", got)
	}
	assertNoZipTemp(t, jsonDir)
}

func TestAccountSnapshotZipCollisionSkipsExistingArchive(t *testing.T) {
	deps, closeSrv := snapshotFixtureDeps(t)
	defer closeSrv()
	outDir := t.TempDir()
	stamp := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC).In(time.Local).Format("2006-01-02-150405")
	planted := filepath.Join(outDir, stamp+"-ps99.zip")
	const sentinel = "sentinel-zip"
	if err := os.WriteFile(planted, []byte(sentinel), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code := Execute(deps, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--zip"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	got, err := os.ReadFile(planted)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != sentinel {
		t.Fatalf("default zip was replaced: %q", got)
	}
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-manifest.json")); !os.IsNotExist(err) {
		t.Fatalf("collision suffix was not used: %v", listNames(t, outDir))
	}
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-0001-ps99-manifest.json")); err != nil {
		t.Fatalf("shifted snapshot missing: %v names=%v", err, listNames(t, outDir))
	}
	next := filepath.Join(outDir, stamp+"-0001-ps99.zip")
	r, err := zip.OpenReader(next)
	if err != nil {
		t.Fatalf("shifted zip: %v names=%v", err, listNames(t, outDir))
	}
	r.Close()
	if strings.Contains(stdout.String(), "ZIP failed") || strings.Contains(stdout.String(), "already exists") {
		t.Fatalf("collision looked like a ZIP failure:\n%s", stdout.String())
	}
}

func snapshotFixtureDeps(t *testing.T) (Deps, func()) {
	t.Helper()
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	invBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	profBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	extBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(invBody)
		case "/v1/account/profile":
			_, _ = w.Write(profBody)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(extBody)
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			w.WriteHeader(404)
		}
	}))
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	return Deps{
		TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed}, IsTTY: func() bool { return false },
	}, srv.Close
}

func assertNoZipTemp(t *testing.T, dir string) {
	t.Helper()
	ents, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, ent := range ents {
		if strings.HasPrefix(ent.Name(), ".bg-cli-zip-") || strings.HasSuffix(ent.Name(), ".tmp") {
			t.Fatalf("temp left behind: %s", ent.Name())
		}
	}
}

func TestAccountSnapshotZipsByDefaultAndShowsLocalRefresh(t *testing.T) {
	deps, closeSrv := snapshotFixtureDeps(t)
	defer closeSrv()
	outDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(outDir, ".env"), []byte("BG_CLI_CLIENT_SECRET=nope"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "config.json"), []byte(`{"token":"nope"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code := Execute(deps, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	stamp := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC).In(time.Local).Format("2006-01-02-150405")
	zipPath := filepath.Join(outDir, stamp+"-ps99.zip")
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("default zip missing: %v names=%v", err, listNames(t, outDir))
	}
	names := map[string]bool{}
	for _, f := range r.File {
		names[f.Name] = true
	}
	r.Close()
	if !names[stamp+"-ps99-manifest.json"] || !names[stamp+"-ps99-inv.json"] || !names[stamp+"-ps99-inventory-diff.json"] {
		t.Fatalf("members=%v", names)
	}
	if names[".env"] || names["config.json"] || names[stamp+"-ps99.zip"] {
		t.Fatalf("unexpected members=%v", names)
	}
	want := time.Date(2026, 11, 6, 12, 5, 0, 0, time.UTC).In(time.Local).Format("2006-01-02 15:04:05 MST")
	if !strings.Contains(stdout.String(), "Next refresh") || !strings.Contains(stdout.String(), want) {
		t.Fatalf("local refresh %q missing\n%s", want, stdout.String())
	}
	if strings.Contains(stdout.String(), "12:05:00.000Z") || strings.Contains(stdout.String(), "nextRefreshEligibleAt") {
		t.Fatalf("raw refresh dumped:\n%s", stdout.String())
	}
}

func TestAccountSnapshotNoZip(t *testing.T) {
	deps, closeSrv := snapshotFixtureDeps(t)
	defer closeSrv()
	outDir := t.TempDir()
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	code := Execute(deps, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--no-zip"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	stamp := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC).In(time.Local).Format("2006-01-02-150405")
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-manifest.json")); err != nil {
		t.Fatalf("snapshot files missing: %v", err)
	}
	for _, name := range listNames(t, outDir) {
		if strings.HasSuffix(name, ".zip") || strings.HasPrefix(name, ".bg-cli-zip-") {
			t.Fatalf("zip written despite --no-zip: %s", name)
		}
	}
	if strings.Contains(stdout.String(), "sha256") || strings.Contains(stdout.String(), "Bundle") {
		t.Fatalf("bundle section without a zip:\n%s", stdout.String())
	}

	stdout.Reset()
	stderr.Reset()
	code = Execute(deps, []string{"account", "snapshot", "--zip", "--no-zip", "--dir", t.TempDir()})
	if code != exitcode.Usage {
		t.Fatalf("both flags exit %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
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
