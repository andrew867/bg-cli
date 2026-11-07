package craft

import (
	"testing"
)

func TestGargantuanBOMRegression(t *testing.T) {
	bom, err := ExpandBOM(DefaultMasqueradeRecipes(), "gargantuan-butler-penguin", 1)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]int{}
	for _, l := range bom.Lines {
		got[l.ItemID] = l.Quantity
	}
	want := map[string]int{
		"Masquerade Agony":         480,
		"Spirit Shards":            3420,
		"Huge Bat Hoodie Bunny":    48,
		"Huge Witch Fox":           12,
		"Huge Thorn Dragon":        4,
		"Titanic Masquerade Agony": 2,
	}
	for k, w := range want {
		if got[k] != w {
			t.Fatalf("%s: got %d want %d (all=%v)", k, got[k], w, got)
		}
	}
	if bom.SumDurationHours != 136 {
		// 480*1 + 48*2 + 12*4 + 4*12 + 2*24? No — sum is over craft ops:
		// 48 bunny crafts *1h + 12 fox *2h + 4 dragon *4h + 2 titanic *12h + 1 garg *24h
		// = 48 + 24 + 16 + 24 + 24 = 136
		t.Fatalf("sumDuration=%v want 136", bom.SumDurationHours)
	}
	if bom.TheoreticalParallelH == nil || *bom.TheoreticalParallelH != 43 {
		t.Fatalf("parallel=%v", bom.TheoreticalParallelH)
	}
	if bom.ParallelNote == "" {
		t.Fatal("parallel must be labeled theoretical")
	}
}

func TestBunnyNormalOnly(t *testing.T) {
	r, ok := LookupRecipe(DefaultMasqueradeRecipes(), "huge-bat-hoodie-bunny")
	if !ok {
		t.Fatal("missing")
	}
	ing := r.Ingredients[0]
	if AllowsVariant(ing, VariantGolden) || AllowsVariant(ing, VariantRainbow) {
		t.Fatal("golden/rainbow must not be allowed for agony ingredient")
	}
	if !AllowsVariant(ing, VariantNormal) {
		t.Fatal("normal required")
	}
}

func TestOptimizeNeverClaimsAutoCraft(t *testing.T) {
	rep, err := OptimizePlan(OptimizeOptions{
		Objective: ObjEarliestFirstHuge,
		Stock:     InventoryStock{"Masquerade Agony": 10, "Spirit Shards": 30},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.Assumptions) == 0 {
		t.Fatal("need advisory assumptions")
	}
	found := false
	for _, a := range rep.Assumptions {
		if contains(a, "never submits") || contains(a, "advisory") {
			found = true
		}
	}
	if !found {
		t.Fatalf("%v", rep.Assumptions)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
