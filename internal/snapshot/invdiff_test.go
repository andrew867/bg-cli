package snapshot

import (
	"bytes"
	"encoding/json"
	"math/big"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func itemsJSON(rows ...string) json.RawMessage {
	if len(rows) == 0 {
		return json.RawMessage(`{"items":[]}`)
	}
	return json.RawMessage(`{"items":[` + strings.Join(rows, ",") + `]}`)
}

func findItem(d InventoryDiff, id string) (ItemChange, bool) {
	for _, ch := range d.ItemChanges {
		if ch.ID == id {
			return ch, true
		}
	}
	return ItemChange{}, false
}

func TestOrchardPonyStackIsOneIdentity(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Orchard Pony","count":100,"collection":"Pets","displayName":"Orchard Pony","stackKey":"{\"id\":\"Orchard Pony\",\"_am\":100}","rawData":{"_am":100,"id":"Orchard Pony","pt":1}}`)
	cur := itemsJSON(`{"class":"Pet","id":"Orchard Pony","count":125,"collection":"Pets","displayName":"Orchard Pony","stackKey":"{\"id\":\"Orchard Pony\",\"_am\":125}","rap":50,"exists":9,"icon":"rbx","rawData":{"pt":1,"id":"Orchard Pony","_am":125}}`)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	if d.Status != DiffStatusOK {
		t.Fatalf("status %s %s", d.Status, d.Reason)
	}
	if len(d.ItemChanges) != 1 {
		t.Fatalf("changes=%d %+v", len(d.ItemChanges), d.ItemChanges)
	}
	ch := d.ItemChanges[0]
	if ch.Previous != "100" || ch.Current != "125" || ch.Delta != "25" {
		t.Fatalf("qty %+v", ch)
	}
	if ch.Classification != ClassNetInventoryChange {
		t.Fatalf("class %s", ch.Classification)
	}
	if strings.Contains(ch.Identity, "_am") || strings.Contains(ch.Identity, "stackKey") {
		t.Fatalf("identity kept quantity: %s", ch.Identity)
	}
	if !strings.Contains(ch.Identity, `"pt":1`) || !ch.Golden || ch.Variant != "Golden" {
		t.Fatalf("variant identity %s %+v", ch.Identity, ch)
	}
	same := itemsJSON(`{"class":"Pet","id":"Orchard Pony","count":125,"collection":"Pets","rawData":{"id":"Orchard Pony","pt":1,"_am":125}}`)
	again := DiffInventories(cur, same, InventoryDiffOptions{})
	if len(again.ItemChanges) != 0 || again.Aggregates.PetQuantity.Delta != "0" {
		t.Fatalf("field order or stack key created a change: %+v", again.ItemChanges)
	}
}

func TestVariantOverlapIsNotExclusive(t *testing.T) {
	cur := itemsJSON(
		`{"class":"Pet","id":"Foo","count":3,"displayName":"Foo","collection":"Pets","rawData":{"id":"Foo"}}`,
		`{"class":"Pet","id":"Foo","count":2,"displayName":"Foo","collection":"Pets","rawData":{"id":"Foo","pt":1}}`,
		`{"class":"Pet","id":"Foo","count":1,"displayName":"Foo","collection":"Pets","rawData":{"id":"Foo","pt":2}}`,
		`{"class":"Pet","id":"Foo","count":4,"displayName":"Foo","collection":"Pets","rawData":{"id":"Foo","sh":true}}`,
		`{"class":"Pet","id":"Foo","count":5,"displayName":"Foo","collection":"Pets","rawData":{"id":"Foo","pt":1,"sh":true}}`,
	)
	d := DiffInventories(itemsJSON(), cur, InventoryDiffOptions{})
	if d.Aggregates.PetQuantity.Delta != "15" {
		t.Fatalf("pets %s", d.Aggregates.PetQuantity.Delta)
	}
	if d.Aggregates.Golden.Delta != "7" || d.Aggregates.Rainbow.Delta != "1" || d.Aggregates.Shiny.Delta != "9" {
		t.Fatalf("traits g=%s r=%s s=%s", d.Aggregates.Golden.Delta, d.Aggregates.Rainbow.Delta, d.Aggregates.Shiny.Delta)
	}
	traitSum := new(big.Int).Add(ParseExactIntString(d.Aggregates.Golden.Delta).Int, ParseExactIntString(d.Aggregates.Rainbow.Delta).Int)
	traitSum.Add(traitSum, ParseExactIntString(d.Aggregates.Shiny.Delta).Int)
	if traitSum.Cmp(ParseExactIntString(d.Aggregates.PetQuantity.Delta).Int) == 0 {
		t.Fatal("trait totals were exclusive")
	}
	if len(d.PetSpeciesChanges) != 1 || d.PetSpeciesChanges[0].Delta != "15" || d.PetSpeciesChanges[0].ID != "Foo" {
		t.Fatalf("species %+v", d.PetSpeciesChanges)
	}
	if len(d.ItemChanges) != 5 {
		t.Fatalf("identities %d", len(d.ItemChanges))
	}
	var goldenShiny bool
	for _, ch := range d.ItemChanges {
		if ch.Golden && ch.Shiny && ch.Variant == "Golden Shiny" && ch.Delta == "5" {
			goldenShiny = true
		}
	}
	if !goldenShiny {
		t.Fatalf("missing golden shiny identity: %+v", d.ItemChanges)
	}
}

func TestUniquePetsStayDistinctAndHugeUsesQuantity(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Huge Cat","count":1,"displayName":"Huge Cat","rawData":{"id":"Huge Cat","createdAt":"A","huge":true}}`)
	cur := itemsJSON(
		`{"class":"Pet","id":"Huge Cat","count":3,"displayName":"Huge Cat","rawData":{"createdAt":"A","huge":true,"id":"Huge Cat"}}`,
		`{"class":"Pet","id":"Huge Cat","count":2,"displayName":"Huge Cat","rawData":{"id":"Huge Cat","createdAt":"B","huge":true}}`,
	)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	if len(d.ItemChanges) != 2 {
		t.Fatalf("identities %d %+v", len(d.ItemChanges), d.ItemChanges)
	}
	if len(d.PetSpeciesChanges) != 1 || d.PetSpeciesChanges[0].Delta != "4" {
		t.Fatalf("species %+v", d.PetSpeciesChanges)
	}
	if d.Aggregates.Huge.Delta != "4" || d.Aggregates.Huge.Status != "" {
		t.Fatalf("huge %+v", d.Aggregates.Huge)
	}
	for _, ch := range d.ItemChanges {
		if !strings.Contains(ch.Identity, "createdAt") {
			t.Fatalf("dropped createdAt %s", ch.Identity)
		}
		if ch.Huge == nil || !*ch.Huge {
			t.Fatalf("huge flag %+v", ch)
		}
	}
	if d.ItemChanges[0].Identity == d.ItemChanges[1].Identity {
		t.Fatal("createdAt collapsed")
	}
}

func TestHugeNameIsNotClassification(t *testing.T) {
	cur := itemsJSON(`{"class":"Pet","id":"Huge Not Huge","count":4,"displayName":"Huge Not Huge","rawData":{"id":"Huge Not Huge"}}`)
	d := DiffInventories(itemsJSON(), cur, InventoryDiffOptions{})
	if d.Aggregates.PetQuantity.Delta != "4" {
		t.Fatalf("pets %s", d.Aggregates.PetQuantity.Delta)
	}
	if d.Aggregates.Huge.Status != DiffStatusUnavailable || d.Aggregates.Huge.Current != "" || d.Aggregates.Huge.Delta != "" || d.Aggregates.Huge.Previous != "" {
		t.Fatalf("name inferred huge: %+v", d.Aggregates.Huge)
	}
	prev := itemsJSON(`{"class":"Pet","id":"Plain","count":1,"rawData":{"id":"Plain","huge":false}}`)
	flagged := itemsJSON(
		`{"class":"Pet","id":"Huge Not Huge","count":4,"rawData":{"id":"Huge Not Huge"}}`,
		`{"class":"Pet","id":"Real","count":3,"rawData":{"id":"Real","huge":true}}`,
	)
	both := DiffInventories(prev, flagged, InventoryDiffOptions{})
	if both.Aggregates.Huge.Previous != "0" || both.Aggregates.Huge.Current != "3" || both.Aggregates.Huge.Delta != "3" {
		t.Fatalf("huge quantity %+v", both.Aggregates.Huge)
	}
}

func TestMarketMetadataIsNotAnAcquisition(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Foo","count":8,"displayName":"Foo","rap":1,"exists":2,"icon":"a","rarity":"Basic","collection":"Pets","stackKey":"old","rawData":{"id":"Foo","_am":8}}`)
	cur := itemsJSON(`{"class":"Pet","id":"Foo","count":8,"displayName":"Foo Deluxe","rap":99,"exists":100,"icon":"b","rarity":"Legendary","rarityNumber":5,"goldenIcon":"g","collection":"Pets","stackKey":"new","rawData":{"_am":8,"id":"Foo"}}`)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	if len(d.ItemChanges) != 0 || len(d.PetSpeciesChanges) != 0 {
		t.Fatalf("metadata change leaked: %+v", d.ItemChanges)
	}
	if d.Aggregates.PetQuantity.Delta != "0" {
		t.Fatalf("pet delta %s", d.Aggregates.PetQuantity.Delta)
	}
}

func TestDuplicateIdentitiesSumBeforeCompare(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Foo","count":20,"rawData":{"id":"Foo","pt":1}}`)
	cur := itemsJSON(
		`{"class":"Pet","id":"Foo","count":15,"rawData":{"pt":1,"id":"Foo"}}`,
		`{"class":"Pet","id":"Foo","count":10,"rawData":{"id":"Foo","pt":1}}`,
	)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	if len(d.ItemChanges) != 1 || d.ItemChanges[0].Previous != "20" || d.ItemChanges[0].Current != "25" || d.ItemChanges[0].Delta != "5" {
		t.Fatalf("%+v", d.ItemChanges)
	}
	swapped := itemsJSON(
		`{"class":"Pet","id":"Foo","count":10,"rawData":{"id":"Foo","pt":1}}`,
		`{"class":"Pet","id":"Foo","count":15,"rawData":{"pt":1,"id":"Foo"}}`,
	)
	again := DiffInventories(prev, swapped, InventoryDiffOptions{})
	if again.ItemChanges[0].Delta != "5" || again.ItemChanges[0].Identity != d.ItemChanges[0].Identity {
		t.Fatalf("row order changed the diff: %+v", again.ItemChanges)
	}
}

func TestAddedAndRemovedIdentities(t *testing.T) {
	prev := itemsJSON(
		`{"class":"Pet","id":"Old","count":4,"displayName":"Old","rawData":{"id":"Old"}}`,
		`{"class":"Fruit","id":"Apple","count":2,"displayName":"Apple","rawData":{"id":"Apple"}}`,
		`{"class":"Currency","id":"TestCoins","count":5,"displayName":"Test Coins","rawData":{"id":"TestCoins"}}`,
	)
	cur := itemsJSON(
		`{"class":"Pet","id":"New","count":1,"displayName":"New","rawData":{"id":"New"}}`,
		`{"class":"Currency","id":"TestCoins","count":9,"displayName":"Test Coins","rawData":{"id":"TestCoins"}}`,
	)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	old, ok := findItem(d, "Old")
	if !ok || old.Classification != ClassRemovedFromInventory || old.Previous != "4" || old.Current != "0" || old.Delta != "-4" {
		t.Fatalf("removed %+v", old)
	}
	neu, ok := findItem(d, "New")
	if !ok || neu.Classification != ClassAddedToInventory || neu.Previous != "0" || neu.Current != "1" || neu.Delta != "1" {
		t.Fatalf("added %+v", neu)
	}
	apple, ok := findItem(d, "Apple")
	if !ok || apple.Classification != ClassRemovedFromInventory || apple.Delta != "-2" {
		t.Fatalf("fruit %+v", apple)
	}
	coins, ok := findItem(d, "TestCoins")
	if !ok || coins.Class != "Currency" || coins.Delta != "4" {
		t.Fatalf("currency %+v", coins)
	}
	if d.Counts == nil || d.Counts.Added != 1 || d.Counts.Removed != 2 || d.Counts.Changed != 4 {
		t.Fatalf("counts %+v", d.Counts)
	}
}

func TestMissingPreviousKeepsAggregatesWithoutZeroDeltas(t *testing.T) {
	cur := itemsJSON(`{"class":"Pet","id":"Foo","count":6,"rawData":{"id":"Foo","pt":1}}`)
	d := DiffInventories(nil, cur, InventoryDiffOptions{UnavailableReason: "no previous snapshot"})
	if d.Status != DiffStatusUnavailable || d.Reason != "no previous snapshot" {
		t.Fatalf("status %s %s", d.Status, d.Reason)
	}
	if d.Aggregates.PetQuantity.Current != "6" || d.Aggregates.PetQuantity.Delta != "" || d.Aggregates.PetQuantity.Previous != "" {
		t.Fatalf("invented delta %+v", d.Aggregates.PetQuantity)
	}
	if d.Aggregates.Golden.Current != "6" || d.Aggregates.Golden.Delta != "" {
		t.Fatalf("golden %+v", d.Aggregates.Golden)
	}
	if len(d.ItemChanges) != 0 || len(d.PetSpeciesChanges) != 0 || d.Counts != nil {
		t.Fatalf("invented changes %+v", d)
	}
	raw, err := json.Marshal(d.Aggregates.PetQuantity)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), `"delta"`) || strings.Contains(string(raw), `"previous"`) {
		t.Fatalf("zero stored in JSON %s", raw)
	}
}

func TestOldInventoryPayloadStillDiffs(t *testing.T) {
	dir := t.TempDir()
	const stamp = "2026-11-01-000000"
	summary := `{
	  "schemaVersion": "1.0",
	  "localStamp": "2026-11-01-000000",
	  "exportedAt": "2026-11-01T00:00:00Z",
	  "sessionId": "ps99-old",
	  "prefix": "ps99",
	  "profile": "main",
	  "current": {"currenciesKnown": false, "currencies": {}, "statisticsKnown": true, "statistics": {"Eggs Opened": "10"}, "clan": {}}
	}`
	manifest := `{
	  "schemaVersion": "1.0",
	  "command": "account snapshot",
	  "ok": true,
	  "localStamp": "2026-11-01-000000",
	  "exportedAt": "2026-11-01T00:00:00Z",
	  "profile": "main",
	  "prefix": "ps99",
	  "files": [{"name": "2026-11-01-000000-ps99-inv.json", "relPath": "2026-11-01-000000-ps99-inv.json", "kind": "inventory-json", "ok": true}],
	  "manifestFile": "2026-11-01-000000-ps99-manifest.json"
	}`
	inv := `{"status":"ok","data":{"inventory":{"items":[{"class":"Pet","id":"Foo","count":2,"rawData":{"id":"Foo","_am":2}}]}}}`
	for name, body := range map[string]string{
		stamp + "-ps99-summary.json":  summary,
		stamp + "-ps99-manifest.json": manifest,
		stamp + "-ps99-inv.json":      inv,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	got := LoadPreviousInventory(PreviousRequest{
		Dir: dir, SummaryFile: stamp + "-ps99-summary.json", Prefix: "ps99",
		SessionID: "ps99-old", Profile: "main",
	})
	if got.Reason != "" {
		t.Fatal(got.Reason)
	}
	cur := itemsJSON(`{"class":"Pet","id":"Foo","count":5,"rawData":{"id":"Foo"}}`)
	d := DiffInventories(got.Items, cur, InventoryDiffOptions{
		PrevStats: got.Statistics, PrevStatsKnown: got.StatisticsKnown,
		CurStats: map[string]string{"Eggs Opened": "14"}, CurStatsKnown: true,
	})
	if d.Status != DiffStatusOK || len(d.ItemChanges) != 1 || d.ItemChanges[0].Delta != "3" {
		t.Fatalf("%s %+v", d.Reason, d.ItemChanges)
	}
	if d.Diagnostics == nil || d.Diagnostics.PetQuantityDelta != "3" || d.Diagnostics.EggsOpenedDelta != "4" || d.Diagnostics.PetMinusEggs != "-1" {
		t.Fatalf("diagnostic %+v", d.Diagnostics)
	}
}

func TestSpeciesDeltasSumToPetDelta(t *testing.T) {
	prev := itemsJSON(
		`{"class":"Pet","id":"A","count":5,"rawData":{"id":"A"}}`,
		`{"class":"Pet","id":"B","count":2,"rawData":{"id":"B","pt":1}}`,
	)
	cur := itemsJSON(
		`{"class":"Pet","id":"A","count":1,"rawData":{"id":"A"}}`,
		`{"class":"Pet","id":"A","count":2,"rawData":{"id":"A","sh":true}}`,
		`{"class":"Pet","id":"C","count":4,"rawData":{"id":"C","pt":1,"sh":true}}`,
	)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	sum := 0
	for _, s := range d.PetSpeciesChanges {
		n := ParseExactIntString(s.Delta)
		if !n.ParseOK || n.Int == nil {
			t.Fatalf("species %s", s.Delta)
		}
		sum += int(n.Int.Int64())
	}
	pet := ParseExactIntString(d.Aggregates.PetQuantity.Delta)
	if int(pet.Int.Int64()) != sum {
		t.Fatalf("species %d pet %s", sum, d.Aggregates.PetQuantity.Delta)
	}
	if d.Aggregates.Golden.Delta != "2" || d.Aggregates.Shiny.Delta != "6" {
		t.Fatalf("overlap g=%s s=%s pet=%s", d.Aggregates.Golden.Delta, d.Aggregates.Shiny.Delta, d.Aggregates.PetQuantity.Delta)
	}
}

func TestSnapshotDiffUsesTheSameInventoryEngine(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Foo","count":1,"rawData":{"id":"Foo"}}`)
	cur := itemsJSON(`{"class":"Pet","id":"Foo","count":4,"rawData":{"id":"Foo","pt":2,"sh":true}}`)
	from := &Bundle{Manifest: &Manifest{ExportedAt: "2026-11-01T00:00:00Z", Files: []FileRecord{{Name: "old-inv.json", Kind: "inventory-json", OK: true}}}, Inventory: prev}
	to := &Bundle{Manifest: &Manifest{ExportedAt: "2026-11-06T00:00:00Z", Files: []FileRecord{{Name: "new-inv.json", Kind: "inventory-json", OK: true}}}, Inventory: cur}
	res, err := DiffBundles(from, to, DiffOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if res.InventoryDiff == nil {
		t.Fatal("missing inventory diff")
	}
	direct := DiffInventories(prev, cur, InventoryDiffOptions{
		BaselineFile: "old-inv.json", CurrentFile: "new-inv.json",
		BaselineAt: "2026-11-01T00:00:00Z", CurrentAt: "2026-11-06T00:00:00Z",
	})
	got, _ := json.Marshal(res.InventoryDiff)
	want, _ := json.Marshal(direct)
	if string(got) != string(want) {
		t.Fatalf("offline diff diverged\n%s\n%s", got, want)
	}
}

func TestInventoryDiffRoundTrip(t *testing.T) {
	const huge = "123456789012345678901234567890"
	prev := itemsJSON(`{"class":"Pet","id":"Big","count":1,"rawData":{"id":"Big","createdAt":"u","huge":false,"tn":"x"}}`)
	cur := itemsJSON(`{"class":"Pet","id":"Big","count":` + huge + `,"displayName":"Big","rawData":{"tn":"x","createdAt":"u","id":"Big","huge":false}}`)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	var buf bytes.Buffer
	if err := WriteInventoryDiff(&buf, d); err != nil {
		t.Fatal(err)
	}
	text := buf.String()
	if strings.Contains(text, "NaN") || strings.Contains(text, "Infinity") || strings.Contains(text, "_am") {
		t.Fatalf("bad encoding %s", text)
	}
	if !strings.Contains(text, `"`+huge+`"`) || !strings.Contains(text, "createdAt") {
		t.Fatalf("lost exact fields %s", text)
	}
	var again InventoryDiff
	if err := json.Unmarshal(buf.Bytes(), &again); err != nil {
		t.Fatal(err)
	}
	if len(again.ItemChanges) != 1 {
		t.Fatalf("%+v", again.ItemChanges)
	}
	ch := again.ItemChanges[0]
	if ch.Previous != "1" || ch.Current != huge || ch.Delta != subHuge(huge) {
		t.Fatalf("rounded %+v", ch)
	}
	if ch.Huge == nil || *ch.Huge {
		t.Fatalf("huge flag %+v", ch.Huge)
	}
	if !strings.Contains(ch.Identity, `"createdAt":"u"`) || !strings.Contains(ch.Identity, `"tn":"x"`) {
		t.Fatalf("identity %s", ch.Identity)
	}
	second, _ := json.Marshal(again.ItemChanges)
	first, _ := json.Marshal(d.ItemChanges)
	if string(first) != string(second) {
		t.Fatalf("order or fields drifted\n%s\n%s", first, second)
	}
}

func subHuge(cur string) string {
	return new(big.Int).Sub(ParseExactIntString(cur).Int, big.NewInt(1)).String()
}

func TestInventoryDiffIsLinear(t *testing.T) {
	const n = 4000
	prev := generatedPets(n, 0)
	cur := generatedPets(n, 1)
	t0 := time.Now()
	pi, reason := indexInventory(prev, "previous")
	if reason != "" {
		t.Fatal(reason)
	}
	parsePrev := time.Since(t0)
	t0 = time.Now()
	ci, reason := indexInventory(cur, "current")
	if reason != "" {
		t.Fatal(reason)
	}
	parseCur := time.Since(t0)
	t0 = time.Now()
	d := compareIndexes(newInventoryDiff(), pi, ci, InventoryDiffOptions{})
	diffed := time.Since(t0)
	t0 = time.Now()
	var buf bytes.Buffer
	if err := WriteInventoryDiff(&buf, d); err != nil {
		t.Fatal(err)
	}
	serialized := time.Since(t0)
	t.Logf("parsePrev=%s parseCur=%s diff=%s serialize=%s bytes=%d", parsePrev, parseCur, diffed, serialized, buf.Len())
	if len(d.ItemChanges) != n/2 {
		t.Fatalf("changes %d", len(d.ItemChanges))
	}
	total := parsePrev + parseCur + diffed + serialized
	if total > 5*time.Second {
		t.Fatalf("diff too slow: %s", total)
	}
}

func generatedPets(n, bump int) json.RawMessage {
	var b strings.Builder
	b.WriteString(`{"items":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteByte(',')
		}
		qty := 10
		if i%2 == 0 {
			qty += bump
		}
		b.WriteString(`{"class":"Pet","id":"P`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`","count":`)
		b.WriteString(strconv.Itoa(qty))
		b.WriteString(`,"rawData":{"id":"P`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`","createdAt":"`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`"}}`)
	}
	b.WriteString(`]}`)
	return json.RawMessage(b.String())
}

func TestPreviousInventoryRejectsSymlinkAndTraversal(t *testing.T) {
	dir := t.TempDir()
	outside := t.TempDir()
	secret := filepath.Join(outside, "secret.json")
	if err := os.WriteFile(secret, []byte(`{"items":[{"class":"Pet","id":"Secret","count":1}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	const stamp = "2026-11-01-000000"
	summary := `{"schemaVersion":"1.0","localStamp":"2026-11-01-000000","exportedAt":"2026-11-01T00:00:00Z","sessionId":"s","prefix":"ps99","profile":"main","current":{"currenciesKnown":false,"currencies":{},"clan":{}}}`
	if err := os.WriteFile(filepath.Join(dir, stamp+"-ps99-summary.json"), []byte(summary), 0o644); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, stamp+"-ps99-inv.json")
	if err := os.Symlink(secret, link); err != nil {
		t.Fatal(err)
	}
	manifest := `{"schemaVersion":"1.0","ok":true,"localStamp":"2026-11-01-000000","prefix":"ps99","profile":"main","files":[{"name":"` + stamp + `-ps99-inv.json","kind":"inventory-json","ok":true}],"manifestFile":"m.json"}`
	if err := os.WriteFile(filepath.Join(dir, stamp+"-ps99-manifest.json"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	got := LoadPreviousInventory(PreviousRequest{Dir: dir, SummaryFile: stamp + "-ps99-summary.json", Prefix: "ps99", SessionID: "s", Profile: "main"})
	if got.Items != nil || !strings.Contains(got.Reason, "symlink") {
		t.Fatalf("symlink followed: %+v", got)
	}

	badMan := `{"schemaVersion":"1.0","ok":true,"localStamp":"2026-11-01-000000","prefix":"ps99","profile":"main","files":[{"name":"../secret.json","kind":"inventory-json","ok":true}],"manifestFile":"m.json"}`
	if err := os.WriteFile(filepath.Join(dir, stamp+"-ps99-manifest.json"), []byte(badMan), 0o644); err != nil {
		t.Fatal(err)
	}
	got = LoadPreviousInventory(PreviousRequest{Dir: dir, SummaryFile: stamp + "-ps99-summary.json", Prefix: "ps99", SessionID: "s", Profile: "main"})
	if got.Items != nil || !strings.Contains(got.Reason, "refusing") {
		t.Fatalf("traversal accepted: %+v", got)
	}
}

func TestAllocateStampReservesInventoryDiff(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "2026-01-01-000000-ps99-inventory-diff.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, coll, err := AllocateStamp(dir, "2026-01-01-000000", "ps99")
	if err != nil {
		t.Fatal(err)
	}
	if coll != "0001" {
		t.Fatalf("collision %q", coll)
	}
}

func TestTopLevelQuantityIsNotIdentity(t *testing.T) {
	prev := itemsJSON(`{"class":"Pet","id":"Foo","count":1,"pt":1,"_am":1}`)
	cur := itemsJSON(`{"class":"Pet","id":"Foo","pt":1,"_am":4,"count":4}`)
	d := DiffInventories(prev, cur, InventoryDiffOptions{})
	if len(d.ItemChanges) != 1 || d.ItemChanges[0].Delta != "3" || strings.Contains(d.ItemChanges[0].Identity, "_am") {
		t.Fatalf("%+v", d.ItemChanges)
	}
}
