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
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestClanShowAndBattleArePublic(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	var sawAuth bool
	var sawRefresh bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		if r.URL.Query().Get("refresh") == "true" {
			sawRefresh = true
		}
		switch r.URL.Path {
		case "/api/clan/SYN":
			_, _ = w.Write([]byte(`{"status":"ok","data":{"Name":"Synthetic","Battles":{"Christmas2023":{"BattleID":"Christmas2023","Points":10}}}}`))
		case "/api/activeClanBattle":
			_, _ = w.Write([]byte(`{"status":"ok","data":{"configName":"ActiveBattle","configData":{"Title":"Live","StartTime":1,"FinishTime":2}}}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()
	deps := Deps{TokenStore: store, BaseURL: srv.URL, HTTPClient: srv.Client(), Clock: output.FixedClock{T: time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)}}
	var stdout, stderr bytes.Buffer
	deps.Stdout, deps.Stderr = &stdout, &stderr
	if code := Execute(deps, []string{"clan", "show", "--tag", "SYN"}); code != 0 {
		t.Fatalf("show exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Christmas2023") {
		t.Fatalf("show dropped Battles: %s", stdout.String())
	}
	stdout.Reset()
	if code := Execute(deps, []string{"clan", "battle"}); code != 0 {
		t.Fatalf("battle exit %d %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), `"active": false`) && !strings.Contains(stdout.String(), `"active":false`) {
		t.Fatalf("finished battle not inactive: %s", stdout.String())
	}
	if sawAuth || sawRefresh {
		t.Fatalf("auth=%v refresh=%v", sawAuth, sawRefresh)
	}
}

func TestClanShowMissingTag(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	body, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	ext, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/profile":
			if r.URL.Query().Get("refresh") == "true" {
				t.Error("refresh on clan tag lookup")
			}
			_, _ = w.Write(body)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(ext)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
		}
	}))
	defer srv.Close()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
	}, []string{"clan", "show", "--profile", "main"})
	if code != 2 {
		t.Fatalf("exit %d stdout=%s stderr=%s", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stdout.String()+stderr.String(), "clan show --tag") {
		t.Fatalf("missing example: %s %s", stdout.String(), stderr.String())
	}
}

func TestSnapshotSummaryManifestAndSharedBaseline(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	fixed := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	inv, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	ext, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	profile := []byte(`{"status":"ok","data":{"Clan":"SYN","Currency":{"Diamonds":{"id":"Diamonds","_am":10,"displayName":"Diamonds"},"PumpkinTokens":{"id":"PumpkinTokens","_am":6009,"displayName":"Pumpkin Tokens"},"PumpkinTycoonCoins":{"id":"PumpkinTycoonCoins","_am":960976550608}}}}`)
	start := strconv.FormatInt(fixed.Add(-time.Hour).Unix(), 10)
	finish := strconv.FormatInt(fixed.Add(time.Hour).Unix(), 10)
	battle := []byte(`{"status":"ok","data":{"configName":"ActiveBattle","configData":{"Title":"Live","StartTime":` + start + `,"FinishTime":` + finish + `,"Enabled":null}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Members":[{},{}],"Battles":{"Christmas2023":{"BattleID":"Christmas2023","Points":10},"ActiveBattle":{"BattleID":"ActiveBattle","Points":2000,"Place":4,"PointContributions":[{"UserID":111,"Points":500},{"UserID":1001,"Points":428}]}}}}`)

	var sawAuth bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Header.Get("Authorization") != "" {
			sawAuth = true
		}
		if r.URL.Query().Get("refresh") == "true" {
			t.Error("refresh on snapshot clan path")
		}
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(inv)
		case "/v1/account/profile":
			_, _ = w.Write(profile)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(ext)
		case "/api/clan/SYN":
			_, _ = w.Write(clanBody)
		case "/api/activeClanBattle":
			_, _ = w.Write(battle)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	priorAt := fixed.Add(-5 * time.Minute)
	prior := clan.Summary{
		SchemaVersion: clan.SchemaVersion,
		ExportedAt:    priorAt.Format(time.RFC3339),
		SessionID:     "ps99-session",
		Profile:       "main",
		Current: clan.Current{
			CurrenciesKnown: true,
			Currencies: map[string]clan.CurrencyAmount{
				"Diamonds":           {Amount: "10", DisplayName: "Diamonds"},
				"PumpkinTokens":      {Amount: "4500", DisplayName: "Pumpkin Tokens"},
				"PumpkinTycoonCoins": {Amount: "970000000000", DisplayName: "Pumpkin Tycoon Coins"},
			},
			Clan: clan.ClanCurrent{Tag: "SYN", ActiveBattle: &clan.BattleCurrent{ID: "ActiveBattle", Points: "1000", UserContribution: "400"}},
		},
	}
	raw, _ := json.Marshal(prior)
	if err := os.WriteFile(filepath.Join(outDir, "2026-11-18-155500-ps99-summary.json"), raw, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "2026-11-18-155500-ps99-currencies.csv"), []byte("key,id,_am,amount\nPumpkinTokens,PumpkinTokens,1,1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "2026-11-18-120000-ps99-inv.json"), bytes.Repeat([]byte("x"), 1<<20+8), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(outDir, "bad-ps99-summary.json"), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--user-id", "1001"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if sawAuth {
		t.Fatal("bearer sent to a public clan route")
	}
	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	sumPath := filepath.Join(outDir, stamp+"-ps99-summary.json")
	sumBytes, err := os.ReadFile(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	var sum clan.Summary
	if err := json.Unmarshal(sumBytes, &sum); err != nil {
		t.Fatal(err)
	}
	w := sum.Windows.Five
	if !w.Valid || w.ChangedCurrencies["PumpkinTokens"].Baseline != "4500" || w.ChangedCurrencies["PumpkinTokens"].Delta != "1509" {
		t.Fatalf("5m=%+v tokens=%+v", w, w.ChangedCurrencies["PumpkinTokens"])
	}
	if w.ChangedCurrencies["PumpkinTokens"].NetPerHour != "18108" {
		t.Fatalf("netPerHour=%s", w.ChangedCurrencies["PumpkinTokens"].NetPerHour)
	}
	if _, ok := w.ChangedCurrencies["Diamonds"]; ok {
		t.Fatal("unchanged currency included")
	}
	if w.Clan == nil || w.Clan.Points == nil || w.Clan.Points.Baseline != "1000" || w.Clan.Points.Delta != "1000" {
		t.Fatalf("clan=%+v", w.Clan)
	}
	if w.BaselineFile != "2026-11-18-155500-ps99-summary.json" || sum.Windows.Session.BaselineFile != w.BaselineFile || sum.Windows.Previous.BaselineFile != w.BaselineFile {
		t.Fatalf("baselines diverged 5m=%s session=%s previous=%s", w.BaselineFile, sum.Windows.Session.BaselineFile, sum.Windows.Previous.BaselineFile)
	}
	if sum.Current.Clan.ActiveBattle == nil || sum.Current.Clan.ActiveBattle.UserContributorRank == nil || *sum.Current.Clan.ActiveBattle.UserContributorRank != 2 {
		t.Fatalf("rank=%+v", sum.Current.Clan.ActiveBattle)
	}
	if sum.UserID != "1001" || !sum.Current.CurrenciesKnown {
		t.Fatalf("user=%s known=%v", sum.UserID, sum.Current.CurrenciesKnown)
	}
	if !strings.Contains(stderr.String(), "PumpkinTokens") || strings.Contains(stderr.String(), "Diamonds") {
		t.Fatalf("console=%s", stderr.String())
	}
	clanFile := filepath.Join(outDir, stamp+"-ps99-clan.json")
	clanBytes, err := os.ReadFile(clanFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(clanBytes), "Christmas2023") {
		t.Fatal("clan file dropped Battles history")
	}
	if _, err := os.Stat(filepath.Join(outDir, stamp+"-ps99-clanbattle.json")); err != nil {
		t.Fatal(err)
	}
	manBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man snapshot.Manifest
	if err := json.Unmarshal(manBytes, &man); err != nil {
		t.Fatal(err)
	}
	var rec snapshot.FileRecord
	found := false
	for _, f := range man.Files {
		if f.Kind == "snapshot-summary" {
			rec = f
			found = true
		}
	}
	if !found || !rec.OK || rec.SHA256 == "" || rec.Bytes == 0 {
		t.Fatalf("manifest summary record=%+v", rec)
	}
	sumInfo, err := os.Stat(sumPath)
	if err != nil {
		t.Fatal(err)
	}
	if sumInfo.Size() != rec.Bytes {
		t.Fatalf("size manifest=%d file=%d", rec.Bytes, sumInfo.Size())
	}
	h := sha256.Sum256(sumBytes)
	if hex.EncodeToString(h[:]) != rec.SHA256 {
		t.Fatal("summary sha256 does not match the manifest")
	}
	ents, _ := os.ReadDir(outDir)
	for _, ent := range ents {
		if strings.Contains(ent.Name(), ".tmp") || strings.HasPrefix(ent.Name(), ".bg-cli-snap-") {
			t.Fatalf("partial file left: %s", ent.Name())
		}
	}
}

func TestNewSessionAndSessionIDAreExclusive(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
	}, []string{"account", "snapshot", "--profile", "main", "--new-session", "--session-id", "scripted-1"})
	if code != exitcode.Usage || !strings.Contains(stderr.String(), "--new-session") || !strings.Contains(stderr.String(), "--session-id") {
		t.Fatalf("code=%d stderr=%s", code, stderr.String())
	}
}

func TestSnapshotSessionResetAndContinuation(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	fixed := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	profile := []byte(`{"status":"ok","data":{"Currency":{"Diamonds":{"id":"Diamonds","_am":5,"displayName":"Diamonds"}}}}`)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(inv)
		case "/v1/account/profile":
			_, _ = w.Write(profile)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(ext)
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	plant := func(t *testing.T, dir string) {
		t.Helper()
		prior := clan.Summary{
			SchemaVersion: clan.SchemaVersion,
			ExportedAt:    fixed.Add(-5 * time.Minute).Format(time.RFC3339),
			SessionID:     "kept-session",
			Profile:       "main",
			Current: clan.Current{
				CurrenciesKnown: true,
				Currencies:      map[string]clan.CurrencyAmount{"Diamonds": {Amount: "1", DisplayName: "Diamonds"}},
			},
		}
		raw, _ := json.Marshal(prior)
		if err := os.WriteFile(filepath.Join(dir, "2026-11-18-155500-ps99-summary.json"), raw, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	run := func(t *testing.T, dir string, extra ...string) clan.Summary {
		t.Helper()
		args := []string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"}
		args = append(args, extra...)
		var stdout, stderr bytes.Buffer
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed},
		}, args)
		if code != 0 {
			t.Fatalf("exit %d stderr=%s", code, stderr.String())
		}
		return readSummary(t, dir, fixed)
	}

	continued := t.TempDir()
	plant(t, continued)
	sum := run(t, continued)
	if sum.SessionID != "kept-session" || !sum.Windows.Five.Valid || sum.Windows.Five.BaselineFile != "2026-11-18-155500-ps99-summary.json" {
		t.Fatalf("restart did not continue the session: %+v", sum.Windows.Five)
	}
	if sum.Windows.Five.ChangedCurrencies["Diamonds"].Delta != "4" {
		t.Fatalf("continued delta: %+v", sum.Windows.Five.ChangedCurrencies["Diamonds"])
	}

	reset := t.TempDir()
	plant(t, reset)
	sum = run(t, reset, "--new-session")
	if sum.SessionID == "kept-session" || sum.Windows.Five.Valid || sum.Windows.Five.ChangedCurrencies != nil {
		t.Fatalf("new session reused the old baseline: session=%s window=%+v", sum.SessionID, sum.Windows.Five)
	}
	if !strings.Contains(sum.Windows.Five.Reason, "no earlier snapshot") {
		t.Fatalf("reason=%s", sum.Windows.Five.Reason)
	}
	if _, err := os.Stat(filepath.Join(reset, "2026-11-18-155500-ps99-summary.json")); err != nil {
		t.Fatal("old summary was removed")
	}

	explicit := t.TempDir()
	plant(t, explicit)
	sum = run(t, explicit, "--session-id", "scripted-1")
	if sum.SessionID != "scripted-1" || sum.Windows.Five.Valid {
		t.Fatalf("explicit session inherited history: %+v", sum.Windows.Five)
	}
}

func TestSnapshotClanReadsBypassPublicCache(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	fixed := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	profile := []byte(`{"status":"ok","data":{"Clan":"SYN","Currency":{"Diamonds":{"id":"Diamonds","_am":1}}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Members":[{}]}}`)
	var clanHits, battleHits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(inv)
		case "/v1/account/profile":
			_, _ = w.Write(profile)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(ext)
		case "/api/clan/SYN":
			clanHits++
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write(clanBody)
		case "/api/activeClanBattle":
			battleHits++
			w.Header().Set("Cache-Control", "public, max-age=3600")
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	dir := t.TempDir()
	for i := 0; i < 2; i++ {
		var stdout, stderr bytes.Buffer
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed},
		}, []string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"})
		if code != 0 {
			t.Fatalf("run %d exit %d stderr=%s", i, code, stderr.String())
		}
	}
	if clanHits != 2 || battleHits != 2 {
		t.Fatalf("upstream reads clan=%d battle=%d", clanHits, battleHits)
	}
}

func TestUnconfiguredUserDoesNotReceiveAnotherContribution(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	fixed := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	start := strconv.FormatInt(fixed.Add(-time.Hour).Unix(), 10)
	finish := strconv.FormatInt(fixed.Add(time.Hour).Unix(), 10)
	profile := []byte(`{"status":"ok","data":{"UserID":555,"Clan":"SYN","Currency":{"Diamonds":{"id":"Diamonds","_am":1}}}}`)
	battle := []byte(`{"status":"ok","data":{"configName":"ActiveBattle","configData":{"Title":"Live","StartTime":` + start + `,"FinishTime":` + finish + `}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Members":[{}],"Battles":{"ActiveBattle":{"BattleID":"ActiveBattle","Points":2000,"Place":4,"PointContributions":[{"UserID":555,"Points":999},{"UserID":1001,"Points":1}]}}}}`)
	srv := accountAndClanServer(t, inv, profile, ext, clanBody, battle, 200, 200)
	defer srv.Close()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
	}, []string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	sum := readSummary(t, dir, fixed)
	if sum.AccountID != "555" || sum.UserID != "" {
		t.Fatalf("account=%s user=%s", sum.AccountID, sum.UserID)
	}
	b := sum.Current.Clan.ActiveBattle
	if b == nil || b.Points != "2000" || b.UserContribution != "" || b.UserContributorRank != nil {
		t.Fatalf("contribution leaked: %+v", b)
	}
	if b == nil || !strings.Contains(b.Reason, "not configured") {
		t.Fatalf("reason=%+v", b)
	}
}

func TestSnapshotClanTelemetryIsNonFatal(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	fixed := time.Date(2026, 11, 18, 16, 0, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	profile := []byte(`{"status":"ok","data":{"Clan":"SYN","Currency":{"Diamonds":{"id":"Diamonds","_am":1}}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic"}}`)

	run := func(t *testing.T, clanStatus, battleStatus int, args ...string) (int, clan.Summary, snapshot.Manifest) {
		t.Helper()
		srv := accountAndClanServer(t, inv, profile, ext, clanBody, inactiveClanBattleBody, clanStatus, battleStatus)
		defer srv.Close()
		dir := t.TempDir()
		var stdout, stderr bytes.Buffer
		cmd := append([]string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"}, args...)
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed},
		}, cmd)
		sum := readSummary(t, dir, fixed)
		man := readManifest(t, dir, fixed)
		if _, err := os.Stat(filepath.Join(dir, stampName(fixed)+"-ps99-inv.json")); err != nil {
			t.Fatal("inventory file missing after clan telemetry failure")
		}
		return code, sum, man
	}

	code, sum, man := run(t, 500, 200)
	if code != 0 || !man.OK || !warningHas(sum.Warnings, "clan") || !failedKind(man, "clan-json") {
		t.Fatalf("clan failure code=%d ok=%v warnings=%v", code, man.OK, sum.Warnings)
	}
	code, sum, man = run(t, 200, 500)
	if code != 0 || !man.OK || !warningHas(sum.Warnings, "active battle") {
		t.Fatalf("battle failure code=%d ok=%v warnings=%v", code, man.OK, sum.Warnings)
	}
	code, sum, _ = run(t, 500, 200, "--require-clan")
	if code != exitcode.Upstream {
		t.Fatalf("require-clan exit=%d warnings=%v", code, sum.Warnings)
	}
	if sum.SchemaVersion == "" {
		t.Fatal("require-clan skipped the summary")
	}
}

func accountFixtureBytes(t *testing.T) (inv, ext []byte) {
	t.Helper()
	var err error
	inv, err = os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	ext, err = os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	return inv, ext
}

func accountAndClanServer(t *testing.T, inv, profile, ext, clanBody, battle []byte, clanStatus, battleStatus int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write(inv)
		case "/v1/account/profile":
			_, _ = w.Write(profile)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(ext)
		case "/api/clan/SYN":
			w.WriteHeader(clanStatus)
			if clanStatus == 200 {
				_, _ = w.Write(clanBody)
			}
		case "/api/activeClanBattle":
			w.WriteHeader(battleStatus)
			if battleStatus == 200 {
				_, _ = w.Write(battle)
			}
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func stampName(fixed time.Time) string {
	return fixed.In(time.Local).Format("2006-01-02-150405")
}

func readSummary(t *testing.T, dir string, fixed time.Time) clan.Summary {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, stampName(fixed)+"-ps99-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	var sum clan.Summary
	if err := json.Unmarshal(body, &sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

func readManifest(t *testing.T, dir string, fixed time.Time) snapshot.Manifest {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(dir, stampName(fixed)+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man snapshot.Manifest
	if err := json.Unmarshal(body, &man); err != nil {
		t.Fatal(err)
	}
	return man
}

func warningHas(warnings []string, sub string) bool {
	for _, w := range warnings {
		if strings.Contains(w, sub) {
			return true
		}
	}
	return false
}

func failedKind(man snapshot.Manifest, kind string) bool {
	for _, f := range man.Files {
		if f.Kind == kind && !f.OK && f.Error != "" {
			return true
		}
	}
	return false
}

func TestProfilePS99UserIDSuppliesContribution(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["main"]
	p.PS99UserID = "1001"
	cfg.Profiles["main"] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 18, 17, 0, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	start := strconv.FormatInt(fixed.Add(-time.Hour).Unix(), 10)
	finish := strconv.FormatInt(fixed.Add(time.Hour).Unix(), 10)
	profile := []byte(`{"status":"ok","data":{"Clan":"SYN","Currency":{"Diamonds":{"id":"Diamonds","_am":1}}}}`)
	battle := []byte(`{"status":"ok","data":{"configName":"ActiveBattle","configData":{"Title":"Live","StartTime":` + start + `,"FinishTime":` + finish + `}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Battles":{"ActiveBattle":{"BattleID":"ActiveBattle","Points":80,"PointContributions":[{"UserID":1001,"Points":12}]}}}}`)
	srv := accountAndClanServer(t, inv, profile, ext, clanBody, battle, 200, 200)
	defer srv.Close()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
	}, []string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	sum := readSummary(t, dir, fixed)
	if sum.UserID != "1001" || sum.Current.Clan.ActiveBattle == nil || sum.Current.Clan.ActiveBattle.UserContribution != "12" {
		t.Fatalf("profile user id was not used: %+v", sum.Current.Clan.ActiveBattle)
	}
}

func TestConfigPS99ClanIDSuppliesClan(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	p := cfg.Profiles["main"]
	p.PS99ClanID = "SYN"
	cfg.Profiles["main"] = p
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 18, 17, 30, 0, 0, time.UTC)
	inv, ext := accountFixtureBytes(t)
	profile := []byte(`{"status":"ok","data":{"Currency":{"Diamonds":{"id":"Diamonds","_am":1}}}}`)
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Members":[{}]}}`)
	srv := accountAndClanServer(t, inv, profile, ext, clanBody, inactiveClanBattleBody, 200, 200)
	defer srv.Close()
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
	}, []string{"account", "snapshot", "--profile", "main", "--dir", dir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d stderr=%s", code, stderr.String())
	}
	if strings.Contains(stderr.String(), "clan.json omitted") {
		t.Fatalf("config clan id was ignored:\n%s", stderr.String())
	}
	sum := readSummary(t, dir, fixed)
	if sum.Current.Clan.Tag != "SYN" || sum.Current.Clan.Name != "Synthetic" {
		t.Fatalf("clan %+v", sum.Current.Clan)
	}
}
