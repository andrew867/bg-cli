package loadout

import (
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestHugeXPFixtures(t *testing.T) {
	for level, want := range FixtureDisplayedHugeXPBonus {
		got, id, conf, _ := DisplayedHugeXPBonusPercent(level)
		if id != HugeXPCurveCommunityV1 {
			t.Fatal(id)
		}
		if conf != ConfidenceMeasured {
			t.Fatalf("level %d conf=%s", level, conf)
		}
		if math.Abs(got-want) > 1e-9 {
			t.Fatalf("level %d got %v want %v", level, got, want)
		}
	}
	p1, _, _, _ := DisplayedHugeXPBonusPercent(1)
	if p1 != 0 {
		t.Fatal(p1)
	}
	_, _, conf99, note := DisplayedHugeXPBonusPercent(99)
	if conf99 != ConfidenceAssumed || note == "" {
		t.Fatalf("%s %s", conf99, note)
	}
}

func TestHalloweenModelBHypotheticals(t *testing.T) {
	ex := ExampleModelBNoCharm()
	if math.Abs(ex["Huge Ghost"]-0.31*1.593) > 1e-9 {
		t.Fatal(ex["Huge Ghost"])
	}
	if math.Abs(ex["Huge Hippokin"]-0.31*1.569) > 1e-9 {
		t.Fatal(ex["Huge Hippokin"])
	}
	if math.Abs(ex["Huge Froggy"]-0.31*1.421) > 1e-9 {
		t.Fatal(ex["Huge Froggy"])
	}
}

func TestHalloweenModelsWithholdUnknowns(t *testing.T) {
	preds := PredictHalloweenModels(HalloweenModelInput{
		BestEventStatRelative: 1,
		LevelBonusPercent:     59.3,
		VariantFactorKnown:    false,
		CharmFactorKnown:      false,
	})
	if preds[0].RelativeIndex == nil || math.Abs(*preds[0].RelativeIndex-0.31) > 1e-12 {
		t.Fatalf("%+v", preds[0])
	}
	if preds[1].RelativeIndex != nil {
		t.Fatal("Model B must not default unknown factors to 1")
	}
}

func TestPublicConfigHalloweenPresentationOnly(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "collection-pets-power-sample.json"))
	if err != nil {
		t.Fatal(err)
	}
	pets, meta, err := ParsePetsCollection(raw)
	if err != nil {
		t.Fatal(err)
	}
	if meta.BodySHA256 == "" || meta.ItemCount == 0 {
		t.Fatal(meta)
	}
	idx := BuildPublicConfigIndex(pets, meta, nil, CollectionCacheMeta{})
	bp, ok := idx.LookupPet("Butler Penguin")
	if !ok || bp.HalloweenExtraDamagePresentation == nil || *bp.HalloweenExtraDamagePresentation != 885735 {
		t.Fatalf("%+v", bp)
	}
	hg, ok := idx.LookupPet("Huge Ghost")
	if !ok || hg.HalloweenExtraDamagePresentation != nil {
		t.Fatal("Huge Ghost must not invent structured halloween extra from missing indexDesc pattern")
	}
}

func TestCharmFamiliesNotAllDamage(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "collection-charms.json"))
	if err != nil {
		t.Fatal(err)
	}
	charms, _, err := ParseCharmsCollection(raw)
	if err != nil {
		t.Fatal(err)
	}
	idx := BuildPublicConfigIndex(nil, CollectionCacheMeta{}, charms, CollectionCacheMeta{})
	bonus, _ := idx.LookupCharm("Bonus")
	if bonus.Family != "reward_proc" {
		t.Fatalf("%+v", bonus)
	}
	light, _ := idx.LookupCharm("Lightning")
	if light.Family != "triggered" {
		t.Fatalf("%+v", light)
	}
	roy, _ := idx.LookupCharm("Royalty")
	if roy.Family != "mixed" || roy.DamagePercentClaim == nil || *roy.DamagePercentClaim != 40 {
		t.Fatalf("%+v", roy)
	}
	coins, _ := idx.LookupCharm("Coins")
	if coins.Family != "currency_coins" {
		t.Fatalf("%+v", coins)
	}
}

func TestInspectPowerMissingLevelsAndCurrencySeparationNote(t *testing.T) {
	pl, err := ParseInventoryLoadout(fixtureInventory(t))
	if err != nil {
		t.Fatal(err)
	}
	// Add a Huge Ghost equipped without annotation
	pl.Pets.List = append(pl.Pets.List, EquippedSlot{ID: "Huge Ghost", UID: "g1", IsHugeCandidate: true, TierStatus: "absent"})
	rawP, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "collection-pets-power-sample.json"))
	rawC, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "collection-charms.json"))
	pets, pm, _ := ParsePetsCollection(rawP)
	charms, cm, _ := ParseCharmsCollection(rawC)
	pub := BuildPublicConfigIndex(pets, pm, charms, cm)
	ann, err := LoadAnnotations(filepath.Join("..", "..", "testdata", "fixtures", "loadout-annotations-halloween.json"))
	if err != nil {
		t.Fatal(err)
	}
	// Match ghost by uid override in list
	pl.Pets.List[len(pl.Pets.List)-1].UID = "pet-ghost-1"
	rep := InspectPower(pl, InspectOptions{WorldID: "halloween-games", Annotations: ann, Public: pub, BestEventRel: 1})
	if rep.CurrenciesNote == "" {
		t.Fatal("missing currency separation note")
	}
	var ghost *PetPowerBreakdown
	for i := range rep.Pets {
		if rep.Pets[i].PetID == "Huge Ghost" {
			ghost = &rep.Pets[i]
		}
	}
	if ghost == nil {
		t.Fatal("missing ghost")
	}
	if ghost.LevelBonus.Value == nil || *ghost.LevelBonus.Value != 59.3 {
		t.Fatalf("%+v", ghost.LevelBonus)
	}
	if ghost.ModeledHitDamage.Unknown == false {
		t.Fatal("must not invent modeled hit damage")
	}
	if len(ghost.HalloweenModels) != 4 {
		t.Fatal(ghost.HalloweenModels)
	}
}

func TestDuplicateHugeAnnotationsAmbiguousWithoutUID(t *testing.T) {
	ann := AnnotationFile{SchemaVersion: "1.0", Pets: []PetAnnotation{
		{ID: "Huge Guard Elephant", Level: intPtr(10)},
		{ID: "Huge Guard Elephant", Level: intPtr(20)},
	}}
	_, ok := ann.MatchAnnotation(EquippedSlot{ID: "Huge Guard Elephant"})
	if ok {
		t.Fatal("expected ambiguous")
	}
}

func TestRecommendCurrencyNotMixed(t *testing.T) {
	pl, _ := ParseInventoryLoadout(fixtureInventory(t))
	obj := Objective{ID: "currency-rate"}
	res, err := Recommend(DefaultRegistry(), pl, RecommendOptions{
		OptimizeOptions: OptimizeOptions{WorldID: "halloween-games", Objective: obj, CurrencyID: "HalloweenGamesCoins", MaxCandidates: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, a := range res.Assumptions {
		if contains(a, "Coins enchant") || contains(a, "HalloweenGamesCoins") {
			found = true
		}
	}
	if !found {
		t.Fatalf("%v", res.Assumptions)
	}
	if res.Objective.CurrencyID != "HalloweenGamesCoins" && res.Objective.ID != ObjHalloweenCoinsPerMin {
		t.Fatalf("%+v", res.Objective)
	}
}

func intPtr(v int) *int { return &v }
