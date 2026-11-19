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
	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestSnapshotInventoryDiffMatchesOfflineDiff(t *testing.T) {
	const oldStamp = "2026-11-01-000000"
	prevItems := `{"items":[
		{"class":"Pet","id":"Orchard Pony","count":100,"collection":"Pets","displayName":"Orchard Pony","category":"Uncategorized","stackKey":"{\"id\":\"Orchard Pony\",\"_am\":100}","rawData":{"_am":100,"id":"Orchard Pony","pt":1}},
		{"class":"Pet","id":"Huge Not Huge","count":1,"displayName":"Huge Not Huge","category":"Uncategorized","collection":"Pets","rawData":{"id":"Huge Not Huge"}},
		{"class":"Pet","id":"Huge Cat","count":1,"displayName":"Huge Cat","collection":"Pets","rawData":{"id":"Huge Cat","createdAt":"A","huge":true}},
		{"class":"Fruit","id":"Apple","count":2,"displayName":"Apple","rawData":{"id":"Apple"}},
		{"class":"Currency","id":"TestCoins","count":5,"displayName":"Test Coins","rawData":{"id":"TestCoins"}}
	]}`
	curAPI := `{"status":"ok","data":{"fetchedAt":"2026-11-06T15:45:30Z","items":[
		{"class":"Pet","id":"Orchard Pony","count":125,"collection":"Pets","displayName":"Orchard Pony","category":"Uncategorized","rap":40,"stackKey":"{\"id\":\"Orchard Pony\",\"_am\":125}","rawData":{"pt":1,"id":"Orchard Pony","_am":125}},
		{"class":"Pet","id":"Huge Not Huge","count":1,"displayName":"Huge Not Huge","category":"Uncategorized","collection":"Pets","rawData":{"id":"Huge Not Huge"}},
		{"class":"Pet","id":"Huge Cat","count":1,"displayName":"Huge Cat","collection":"Pets","rawData":{"id":"Huge Cat","createdAt":"A","huge":true}},
		{"class":"Pet","id":"Huge Kitten","count":1,"displayName":"Huge Kitten","collection":"Pets","rawData":{"createdAt":"B","huge":true,"id":"Huge Kitten"}},
		{"class":"Currency","id":"TestCoins","count":9,"displayName":"Test Coins","rawData":{"id":"TestCoins"}}
	]}}`
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
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
			_, _ = w.Write([]byte(curAPI))
		case "/v1/account/profile":
			_, _ = w.Write(profBody)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(extBody)
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	outDir := t.TempDir()
	oldSummary := `{
		"schemaVersion":"1.0","localStamp":"` + oldStamp + `","exportedAt":"2026-11-01T00:00:00Z",
		"sessionId":"ps99-old","prefix":"ps99","profile":"main",
		"current":{"currenciesKnown":false,"currencies":{},"statisticsKnown":true,"statistics":{"Eggs Opened":"10"},"clan":{}}
	}`
	if strings.Contains(oldSummary, "petQuantity") {
		t.Fatal("fixture summary already has inventory aggregates")
	}
	oldManifest := `{
		"schemaVersion":"1.0","command":"account snapshot","ok":true,"localStamp":"` + oldStamp + `",
		"exportedAt":"2026-11-01T00:00:00Z","profile":"main","prefix":"ps99",
		"files":[
			{"name":"` + oldStamp + `-ps99-inv.json","relPath":"` + oldStamp + `-ps99-inv.json","kind":"inventory-json","ok":true},
			{"name":"` + oldStamp + `-ps99-prof.json","relPath":"` + oldStamp + `-ps99-prof.json","kind":"profile-json","ok":true}
		],
		"manifestFile":"` + oldStamp + `-ps99-manifest.json"
	}`
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-summary.json"), oldSummary)
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-manifest.json"), oldManifest)
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-inv.json"), `{"status":"ok","data":{"inventory":`+prevItems+`}}`)
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-prof.json"), `{"status":"ok","data":{"profile":{"Statistics":{"Eggs Opened":10}}}}`)

	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
		IsTTY: func() bool { return false },
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d\nstderr=%s\nstdout=%s", code, stderr.String(), stdout.String())
	}
	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	diffPath := filepath.Join(outDir, stamp+"-ps99-inventory-diff.json")
	diffBytes, err := os.ReadFile(diffPath)
	if err != nil {
		t.Fatal(err)
	}
	var written snapshot.InventoryDiff
	if err := json.Unmarshal(diffBytes, &written); err != nil {
		t.Fatal(err)
	}
	if written.Status != snapshot.DiffStatusOK {
		t.Fatalf("status %s %s", written.Status, written.Reason)
	}
	var orchard int
	for _, ch := range written.ItemChanges {
		if ch.ID != "Orchard Pony" {
			continue
		}
		orchard++
		if ch.Previous != "100" || ch.Current != "125" || ch.Delta != "25" || ch.Classification != snapshot.ClassNetInventoryChange {
			t.Fatalf("orchard %+v", ch)
		}
		if strings.Contains(ch.Identity, "_am") || strings.Contains(ch.Identity, "stackKey") {
			t.Fatalf("quantity leaked into identity %s", ch.Identity)
		}
	}
	if orchard != 1 {
		t.Fatalf("orchard identities %d", orchard)
	}
	if written.Aggregates.Huge.Previous != "1" || written.Aggregates.Huge.Current != "2" || written.Aggregates.Huge.Delta != "1" {
		t.Fatalf("huge %+v", written.Aggregates.Huge)
	}
	foundAdded, foundRemoved := false, false
	for _, ch := range written.ItemChanges {
		if ch.ID == "Huge Kitten" && strings.Contains(ch.Identity, `"createdAt":"B"`) && ch.Classification == snapshot.ClassAddedToInventory && ch.Delta == "1" {
			foundAdded = true
		}
		if ch.ID == "Apple" && ch.Classification == snapshot.ClassRemovedFromInventory && ch.Delta == "-2" {
			foundRemoved = true
		}
	}
	if !foundAdded || !foundRemoved {
		t.Fatalf("added=%v removed=%v %+v", foundAdded, foundRemoved, written.ItemChanges)
	}

	text := stdout.String()
	invAt := strings.Index(text, "\nInventory\n")
	dataAt := strings.Index(text, "\nData\n")
	if invAt < 0 || dataAt < invAt {
		t.Fatalf("inventory section missing\n%s", text)
	}
	inv := text[invAt:dataAt]
	for _, want := range []string{"+25", "net inventory change", "added to inventory", "removed from inventory", "Apple"} {
		if !strings.Contains(inv, want) {
			t.Fatalf("missing %q\n%s", want, inv)
		}
	}
	if strings.Contains(inv, "Test Coins") || strings.Contains(inv, "sold") || strings.Contains(inv, "hatched") || strings.Contains(inv, "Pet quantity and eggs") {
		t.Fatalf("wording or currency leak\n%s", inv)
	}

	zipPath := filepath.Join(outDir, stamp+"-ps99.zip")
	zr, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	zipped := false
	for _, f := range zr.File {
		if f.Name == stamp+"-ps99-inventory-diff.json" {
			zipped = true
		}
	}
	if !zipped {
		t.Fatal("zip is missing inventory-diff.json")
	}

	from, err := snapshot.LoadBundle(filepath.Join(outDir, oldStamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	to, err := snapshot.LoadBundle(filepath.Join(outDir, stamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	offline, err := snapshot.DiffBundles(from, to, snapshot.DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if offline.InventoryDiff == nil {
		t.Fatal("offline diff missing")
	}
	got, _ := json.Marshal(written)
	want, _ := json.Marshal(offline.InventoryDiff)
	if string(got) != string(want) {
		t.Fatalf("snapshot diff != offline diff\nfile %s\noffline %s", got, want)
	}

	var sum clan.Summary
	sumBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-summary.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(sumBytes, &sum); err != nil {
		t.Fatal(err)
	}
	if sum.Current.Inventory == nil || sum.Current.Inventory.PetQuantity == "" {
		t.Fatal("current aggregates were not stored")
	}
	if sum.Windows.Previous.Inventory == nil || sum.Windows.Previous.Inventory.PetQuantity == nil {
		t.Fatal("previous window omitted the aggregate comparison")
	}
	if sum.Windows.Previous.Inventory.PetQuantity.Delta != "" || sum.Windows.Previous.Inventory.PetQuantity.Status != "unavailable" {
		t.Fatalf("old summary invented an aggregate delta: %+v", sum.Windows.Previous.Inventory.PetQuantity)
	}
}

func TestSnapshotVerboseHelpMentionsInventoryDetail(t *testing.T) {
	for _, args := range [][]string{
		{"account", "snapshot", "--help"},
		{"account", "snapshot", "export", "--help"},
	} {
		var stdout, stderr bytes.Buffer
		code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, args)
		if code != 0 {
			t.Fatalf("%v exit %d %s", args, code, stderr.String())
		}
		if !strings.Contains(stdout.String(), "show full inventory changes, diagnostics, and absolute paths") {
			t.Fatalf("help missing verbose wording for %v:\n%s", args, stdout.String())
		}
	}
}

func TestSnapshotDerivedDiffDoesNotMarkInventorySource(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	profBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	extBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	serve := func(inventory int) (*httptest.Server, *bool) {
		called := false
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/v1/account/inventory":
				called = true
				w.WriteHeader(inventory)
				_, _ = w.Write([]byte(`{"status":"error"}`))
			case "/v1/account/profile":
				_, _ = w.Write(profBody)
			case "/v1/account/extendedProfile":
				_, _ = w.Write(extBody)
			case "/api/activeClanBattle":
				_, _ = w.Write(inactiveClanBattleBody)
			default:
				http.NotFound(w, r)
			}
		}))
		return srv, &called
	}

	t.Run("skipped", func(t *testing.T) {
		srv, called := serve(http.StatusOK)
		defer srv.Close()
		outDir := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed},
			IsTTY: func() bool { return false },
		}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99", "--include", "profile,extendedProfile"})
		if code != 0 {
			t.Fatalf("exit %d\nstderr=%s\nstdout=%s", code, stderr.String(), stdout.String())
		}
		if *called {
			t.Fatal("inventory was requested")
		}
		status := dataInventoryStatus(t, stdout.String())
		if status != "skipped" {
			t.Fatalf("Inventory %s\n%s", status, stdout.String())
		}
		stamp := fixed.In(time.Local).Format("2006-01-02-150405")
		assertDerivedDiffOnly(t, outDir, stamp, false)
	})

	t.Run("failed", func(t *testing.T) {
		srv, called := serve(http.StatusInternalServerError)
		defer srv.Close()
		outDir := t.TempDir()
		var stdout, stderr bytes.Buffer
		code := Execute(Deps{
			Stdout: &stdout, Stderr: &stderr, TokenStore: store,
			BaseURL: srv.URL, HTTPClient: srv.Client(),
			Clock: output.FixedClock{T: fixed},
			IsTTY: func() bool { return false },
		}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
		if code != exitcode.Upstream {
			t.Fatalf("exit %d\nstderr=%s\nstdout=%s", code, stderr.String(), stdout.String())
		}
		if !*called {
			t.Fatal("inventory was not requested")
		}
		status := dataInventoryStatus(t, stdout.String())
		if status != "failed" {
			t.Fatalf("Inventory %s\n%s", status, stdout.String())
		}
		stamp := fixed.In(time.Local).Format("2006-01-02-150405")
		assertDerivedDiffOnly(t, outDir, stamp, true)
	})
}

func TestSnapshotMissingItemsStillSucceeds(t *testing.T) {
	store := auth.NewMemoryStore()
	setupProfile(t, store, auth.DefaultScopes)
	profBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	extBody, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "account-extended-profile-ok.json"))
	if err != nil {
		t.Fatal(err)
	}
	const oldStamp = "2026-11-01-000000"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/account/inventory":
			_, _ = w.Write([]byte(`{"status":"ok","data":{"fetchedAt":"2026-11-06T15:45:30Z"}}`))
		case "/v1/account/profile":
			_, _ = w.Write(profBody)
		case "/v1/account/extendedProfile":
			_, _ = w.Write(extBody)
		case "/api/activeClanBattle":
			_, _ = w.Write(inactiveClanBattleBody)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	outDir := t.TempDir()
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-summary.json"), `{
		"schemaVersion":"1.0","localStamp":"`+oldStamp+`","exportedAt":"2026-11-01T00:00:00Z",
		"sessionId":"ps99-old","prefix":"ps99","profile":"main",
		"current":{"currenciesKnown":false,"currencies":{},"statisticsKnown":false,"statistics":{},"clan":{}}
	}`)
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-manifest.json"), `{
		"schemaVersion":"1.0","command":"account snapshot","ok":true,"localStamp":"`+oldStamp+`",
		"exportedAt":"2026-11-01T00:00:00Z","profile":"main","prefix":"ps99",
		"files":[{"name":"`+oldStamp+`-ps99-inv.json","relPath":"`+oldStamp+`-ps99-inv.json","kind":"inventory-json","view":"inventory","ok":true}],
		"manifestFile":"`+oldStamp+`-ps99-manifest.json"
	}`)
	writeFile(t, filepath.Join(outDir, oldStamp+"-ps99-inv.json"), `{"status":"ok","data":{"inventory":{"items":[{"class":"Pet","id":"Orchard Pony","count":100,"category":"Uncategorized","rawData":{"id":"Orchard Pony"}}]}}}`)
	fixed := time.Date(2026, 11, 6, 15, 45, 30, 0, time.UTC)
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout: &stdout, Stderr: &stderr, TokenStore: store,
		BaseURL: srv.URL, HTTPClient: srv.Client(),
		Clock: output.FixedClock{T: fixed},
		IsTTY: func() bool { return false },
	}, []string{"account", "snapshot", "--profile", "main", "--dir", outDir, "--prefix", "ps99"})
	if code != 0 {
		t.Fatalf("exit %d\nstderr=%s\nstdout=%s", code, stderr.String(), stdout.String())
	}
	stamp := fixed.In(time.Local).Format("2006-01-02-150405")
	diffBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-inventory-diff.json"))
	if err != nil {
		t.Fatal(err)
	}
	var written snapshot.InventoryDiff
	if err := json.Unmarshal(diffBytes, &written); err != nil {
		t.Fatal(err)
	}
	if written.Status != snapshot.DiffStatusUnavailable || written.Reason != "current inventory items are absent" {
		t.Fatalf("status %s %s", written.Status, written.Reason)
	}
	if len(written.ItemChanges) != 0 || written.Aggregates.PetQuantity.Current != "" || written.Aggregates.PetQuantity.Delta != "" {
		t.Fatalf("invented removal %+v pets %+v", written.ItemChanges, written.Aggregates.PetQuantity)
	}
	if strings.Contains(stdout.String(), "removed from inventory") {
		t.Fatalf("human summary invented removals\n%s", stdout.String())
	}
}

func dataInventoryStatus(t *testing.T, text string) string {
	t.Helper()
	i := strings.Index(text, "\nData\n")
	if i < 0 {
		t.Fatalf("no data section\n%s", text)
	}
	for _, line := range strings.Split(text[i:], "\n") {
		fields := strings.Fields(line)
		if len(fields) >= 2 && fields[0] == "Inventory" {
			return fields[1]
		}
	}
	t.Fatalf("no inventory status\n%s", text[i:])
	return ""
}

func assertDerivedDiffOnly(t *testing.T, outDir, stamp string, failedUpstream bool) {
	t.Helper()
	manBytes, err := os.ReadFile(filepath.Join(outDir, stamp+"-ps99-manifest.json"))
	if err != nil {
		t.Fatal(err)
	}
	var man snapshot.Manifest
	if err := json.Unmarshal(manBytes, &man); err != nil {
		t.Fatal(err)
	}
	var sawDiff, sawInv bool
	for _, f := range man.Files {
		switch f.Kind {
		case "inventory-diff":
			sawDiff = true
			if f.View != "" || !f.OK {
				t.Fatalf("diff record %+v", f)
			}
		case "inventory-json", "inventory-csv":
			sawInv = true
			if f.View != "inventory" {
				t.Fatalf("inventory view %+v", f)
			}
			if failedUpstream && f.OK {
				t.Fatalf("failed inventory recorded ok %+v", f)
			}
		}
	}
	if !sawDiff {
		t.Fatal("manifest is missing inventory-diff")
	}
	if failedUpstream && !sawInv {
		t.Fatal("failed inventory files were not recorded")
	}
	if !failedUpstream && sawInv {
		t.Fatal("skipped inventory still wrote an inventory source")
	}
	diffName := stamp + "-ps99-inventory-diff.json"
	if _, err := os.Stat(filepath.Join(outDir, diffName)); err != nil {
		t.Fatal(err)
	}
	zr, err := zip.OpenReader(filepath.Join(outDir, stamp+"-ps99.zip"))
	if err != nil {
		t.Fatal(err)
	}
	defer zr.Close()
	zipped := false
	for _, f := range zr.File {
		if f.Name == diffName {
			zipped = true
		}
	}
	if !zipped {
		t.Fatal("zip is missing inventory-diff.json")
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
