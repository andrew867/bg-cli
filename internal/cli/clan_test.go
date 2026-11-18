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
	clanBody := []byte(`{"status":"ok","data":{"Name":"Synthetic","Members":[{},{}],"Battles":{"Christmas2023":{"BattleID":"Christmas2023","Points":10},"ActiveBattle":{"BattleID":"ActiveBattle","Points":2000,"Place":4,"PointContributions":[{"UserID":111,"Points":500},{"UserID":4092648666,"Points":428}]}}}}`)

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
		Current: clan.Current{
			Currencies: map[string]clan.CurrencyAmount{
				"Diamonds":           {Amount: "10", DisplayName: "Diamonds"},
				"PumpkinTokens":      {Amount: "4500", DisplayName: "Pumpkin Tokens"},
				"PumpkinTycoonCoins": {Amount: "970000000000", DisplayName: "Pumpkin Tycoon Coins"},
			},
			Clan: clan.ClanCurrent{ActiveBattle: &clan.BattleCurrent{ID: "ActiveBattle", Points: "1000", UserContribution: "400"}},
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
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
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
	if w.BaselineFile != "2026-11-18-155500-ps99-summary.json" || sum.Windows.Session.BaselineFile != w.BaselineFile {
		t.Fatalf("baselines diverged 5m=%s session=%s", w.BaselineFile, sum.Windows.Session.BaselineFile)
	}
	if sum.Current.Clan.ActiveBattle == nil || sum.Current.Clan.ActiveBattle.UserContributorRank == nil || *sum.Current.Clan.ActiveBattle.UserContributorRank != 2 {
		t.Fatalf("rank=%+v", sum.Current.Clan.ActiveBattle)
	}
	if sum.UserID != clan.DefaultUserID {
		t.Fatalf("user=%s", sum.UserID)
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
