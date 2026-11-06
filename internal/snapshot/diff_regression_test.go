package snapshot

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMasqueradeButlerRegressionFixture(t *testing.T) {
	dir := t.TempDir()
	fromMan := writeFixtureSnap(t, dir, "old", fixtureSnap{
		EggsOpened: "3231890",
		Purchased:  map[string]string{"Masquerade Egg": "1325055"},
		Hatches: map[string]string{
			"Butler Penguin":     "429",
			"Neon Skeleton Cat":  "994475",
			"Porcelain Doll Cat": "330153",
		},
		Currency: "1080312800",
		Butler:   map[string]int64{"Regular": 79, "Golden": 71, "Rainbow": 18, "Shiny": 2, "Golden Shiny": 1},
		SnapAt:   "2026-11-06T12:00:00.000Z",
	})
	toMan := writeFixtureSnap(t, dir, "new", fixtureSnap{
		EggsOpened: "3260946",
		Purchased:  map[string]string{"Masquerade Egg": "1354111"},
		Hatches: map[string]string{
			"Butler Penguin":     "443",
			"Neon Skeleton Cat":  "1015458",
			"Porcelain Doll Cat": "338212",
		},
		Currency: "9354975",
		Butler:   map[string]int64{"Regular": 88, "Golden": 74, "Rainbow": 20, "Shiny": 2, "Golden Shiny": 1},
		SnapAt:   "2026-11-06T18:00:00.000Z",
	})

	fromB, err := LoadBundle(fromMan)
	if err != nil {
		t.Fatal(err)
	}
	toB, err := LoadBundle(toMan)
	if err != nil {
		t.Fatal(err)
	}
	diff, err := DiffBundles(fromB, toB, DiffOptions{
		EggID: "Masquerade Egg", PetID: "Butler Penguin",
		EggPrice: "50000", CurrencyID: "HalloweenGamesCoins",
		FutureEggs: 10000, BatchSize: 64,
	})
	if err != nil {
		t.Fatal(err)
	}
	if diff.Reconciliation.Status != Reconciled {
		t.Fatalf("status=%s notes=%v residual=%v", diff.Reconciliation.Status, diff.Reconciliation.Notes, diff.Reconciliation.ResidualEggsVsHatches)
	}
	if got := *diff.Reconciliation.EggsOpenedDelta; got != "29056" {
		t.Fatalf("eggs opened delta=%s", got)
	}
	if got := *diff.Reconciliation.SelectedEggPurchaseDelta; got != "29056" {
		t.Fatalf("egg purchase delta=%s", got)
	}
	if got := *diff.Reconciliation.PetHatchPositiveDeltaSum; got != "29056" {
		t.Fatalf("pet hatch sum=%s", got)
	}
	if got := *diff.Reconciliation.SelectedPetHatchDelta; got != "14" {
		t.Fatalf("butler hatch delta=%s", got)
	}
	if got := *diff.Reconciliation.InventoryNetSelectedPet; got != "14" {
		t.Fatalf("butler inventory net=%s", got)
	}
	if diff.HatchRate == nil || diff.HatchRate.ObservedPercent == nil {
		t.Fatal("missing hatch rate")
	}
	pct := *diff.HatchRate.ObservedPercent
	if math.Abs(pct-0.0481835) > 0.0001 {
		t.Fatalf("observed percent=%v", pct)
	}
	if diff.HatchRate.AvgEggsPerSuccess == nil || math.Abs(*diff.HatchRate.AvgEggsPerSuccess-2075.42857) > 0.01 {
		t.Fatalf("avg eggs/success=%v", diff.HatchRate.AvgEggsPerSuccess)
	}
	if diff.Cost == nil {
		t.Fatal("missing cost")
	}
	if diff.Cost.GrossEggSpend != "1452800000" {
		t.Fatalf("gross=%s", diff.Cost.GrossEggSpend)
	}
	if diff.Cost.CurrencyDelta != "-1070957825" {
		t.Fatalf("currency delta=%s", diff.Cost.CurrencyDelta)
	}
	if diff.Cost.ImpliedOtherNetFlows != "381842175" {
		t.Fatalf("implied=%s", diff.Cost.ImpliedOtherNetFlows)
	}
}

type fixtureSnap struct {
	EggsOpened string
	Purchased  map[string]string
	Hatches    map[string]string
	Currency   string
	Butler     map[string]int64
	SnapAt     string
}

func writeFixtureSnap(t *testing.T, dir, tag string, f fixtureSnap) string {
	t.Helper()
	items := []map[string]any{}
	var petSum int64
	for variant, count := range f.Butler {
		petSum += count
		raw := map[string]any{"id": "Butler Penguin", "_am": count}
		switch variant {
		case "Golden":
			raw["pt"] = 1
		case "Rainbow":
			raw["pt"] = 2
		case "Shiny":
			raw["sh"] = true
		case "Golden Shiny":
			raw["pt"] = 1
			raw["sh"] = true
		case "Rainbow Shiny":
			raw["pt"] = 2
			raw["sh"] = true
		}
		items = append(items, map[string]any{
			"class": "Pet", "id": "Butler Penguin", "count": count,
			"displayName": "Butler Penguin", "rawData": raw,
		})
	}

	purchJSON := map[string]json.RawMessage{}
	for k, v := range f.Purchased {
		purchJSON[k] = json.RawMessage(v)
	}
	hatchJSON := map[string]json.RawMessage{}
	for k, v := range f.Hatches {
		hatchJSON[k] = json.RawMessage(v)
	}

	invEnv := map[string]any{
		"schemaVersion": "1.0", "ok": true, "command": "account inventory",
		"source": map[string]any{
			"provider": "BIG Games", "classification": "official", "baseUrl": "https://ps99.biggamesapi.io",
			"endpoint": "/v1/account/inventory", "httpStatus": 200, "retrievedAt": f.SnapAt, "fromLocalCache": false,
		},
		"freshness": map[string]any{"snapshotAt": f.SnapAt, "meaning": "cached player snapshot, not real time"},
		"data": map[string]any{
			"inventory": map[string]any{"items": items, "equipped": map[string]any{}, "fetchedAt": f.SnapAt, "cached": false},
			"rowCount":  len(items), "petQuantitySum": petSum,
		},
	}
	profEnv := map[string]any{
		"schemaVersion": "1.0", "ok": true, "command": "account profile",
		"source": map[string]any{
			"provider": "BIG Games", "classification": "official", "baseUrl": "https://ps99.biggamesapi.io",
			"endpoint": "/v1/account/profile", "httpStatus": 200, "retrievedAt": f.SnapAt, "fromLocalCache": false,
		},
		"freshness": map[string]any{"meaning": "cached player snapshot, not real time"},
		"data": map[string]any{
			"profile": map[string]any{
				"Statistics":    map[string]json.RawMessage{"Eggs Opened": json.RawMessage(f.EggsOpened)},
				"PurchasedEggs": purchJSON,
				"PetHatchCount": hatchJSON,
				"Currency": map[string]any{
					"HalloweenGamesCoins": map[string]any{"id": "HalloweenGamesCoins", "_am": json.RawMessage(f.Currency)},
				},
			},
		},
	}

	invName := tag + "-ps99-inv.json"
	profName := tag + "-ps99-prof.json"
	manName := tag + "-ps99-manifest.json"
	invPath := filepath.Join(dir, invName)
	profPath := filepath.Join(dir, profName)
	writeJSONFile(t, invPath, invEnv)
	writeJSONFile(t, profPath, profEnv)
	invHash, invSize := shaFile(t, invPath)
	profHash, profSize := shaFile(t, profPath)
	snapAt := f.SnapAt
	man := Manifest{
		SchemaVersion: ManifestSchemaVersion,
		Command:       "account snapshot",
		OK:            true,
		LocalStamp:    tag,
		ExportedAt:    time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC).Format(time.RFC3339),
		OutputDir:     dir,
		Prefix:        "ps99",
		ManifestFile:  manName,
		Files: []FileRecord{
			{Name: invName, RelPath: invName, Kind: "inventory-json", OK: true, Bytes: invSize, SHA256: invHash, Endpoint: "/v1/account/inventory", HTTPStatus: 200, SnapshotAt: &snapAt, RetrievedAt: f.SnapAt},
			{Name: profName, RelPath: profName, Kind: "profile-json", OK: true, Bytes: profSize, SHA256: profHash, Endpoint: "/v1/account/profile", HTTPStatus: 200, RetrievedAt: f.SnapAt},
		},
	}
	manPath := filepath.Join(dir, manName)
	writeJSONFile(t, manPath, man)
	return manPath
}

func writeJSONFile(t *testing.T, path string, v any) {
	t.Helper()
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
}

func shaFile(t *testing.T, path string) (string, int64) {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), int64(len(b))
}
