package observatory

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveCoinsTierIII(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "enchants-sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ParseTieredCollection("Enchants", b)
	if err != nil {
		t.Fatal(err)
	}
	tn := 3
	dn, cfg, te, ok, reason := ResolveTierDisplay(cat, "Coins", &tn)
	if !ok {
		t.Fatalf("resolve: %s", reason)
	}
	if dn != "Coins III" {
		t.Fatalf("display=%q", dn)
	}
	if cfg.ConfigName != "Enchant | Coins" || te.TierNumber != 3 {
		t.Fatalf("cfg=%+v te=%+v", cfg.ConfigName, te)
	}
	zero := 0
	if _, _, _, ok, _ = ResolveTierDisplay(cat, "Coins", &zero); ok {
		t.Fatal("tn=0 must not resolve as tier I")
	}
	if _, _, _, ok, _ = ResolveTierDisplay(cat, "Coins", nil); ok {
		t.Fatal("nil tn must not invent a tier")
	}
}

func TestResolveInventoryTiersFixture(t *testing.T) {
	enc, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "enchants-sample.json"))
	pot, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "potions-sample.json"))
	ch, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "charms-sample.json"))
	ec, _ := ParseTieredCollection("Enchants", enc)
	pc, _ := ParseTieredCollection("Potions", pot)
	cc, _ := ParseTieredCollection("Charms", ch)
	cat := NewTierCatalogSet(ec, pc, cc)

	inv, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "observatory", "inventory-tiers.json"))
	if err != nil {
		t.Fatal(err)
	}
	rep, err := ResolveInventoryTiers(inv, cat, "testdata/fixtures/observatory/inventory-tiers.json")
	if err != nil {
		t.Fatal(err)
	}
	owned := map[string]TieredItemRow{}
	for _, r := range rep.Owned {
		owned[r.ID+"/"+itoaPtr(r.TN)] = r
	}
	if r := owned["Coins/3"]; !r.ResolvedOK || r.ResolvedDisplayName != "Coins III" {
		t.Fatalf("owned Coins: %+v", r)
	}
	if r := owned["Damage/2"]; !r.ResolvedOK || r.ResolvedDisplayName != "Damage Potion II" {
		t.Fatalf("owned Damage potion: %+v", r)
	}
	if r := owned["Royalty/1"]; !r.ResolvedOK || r.ResolvedDisplayName != "Royalty Charm" {
		t.Fatalf("owned Royalty: %+v", r)
	}

	var coinsEq, luckyEq *TieredItemRow
	for i := range rep.Equipped {
		r := &rep.Equipped[i]
		switch r.ID {
		case "Coins":
			coinsEq = r
		case "Lucky Eggs":
			luckyEq = r
		}
	}
	if coinsEq == nil || !coinsEq.ResolvedOK || coinsEq.ResolvedDisplayName != "Coins III" {
		t.Fatalf("equipped Coins: %+v", coinsEq)
	}
	if luckyEq == nil || luckyEq.ResolvedOK || luckyEq.TN == nil || *luckyEq.TN != 0 {
		t.Fatalf("equipped Lucky Eggs level 0 must stay unresolved: %+v", luckyEq)
	}
}

func itoaPtr(p *int) string {
	if p == nil {
		return "nil"
	}
	return itoa(*p)
}
