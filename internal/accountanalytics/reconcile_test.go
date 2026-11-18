package accountanalytics

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/analysis"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func TestReconcileHollowEggPass(t *testing.T) {
	fromBytes, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "analytics", "hollow-egg-from-profile.json"))
	toBytes, _ := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", "analytics", "hollow-egg-to-profile.json"))
	from := &snapshot.Bundle{ProfileData: fromBytes}
	to := &snapshot.Bundle{ProfileData: toBytes}
	results := RunReconciliation(from, to, ReconcileOptions{EggID: "Hollow Egg"})
	var hatchResult *analysis.CheckResult
	for i := range results {
		if results[i].ID == "egg.hatch.species_sum" {
			hatchResult = &results[i]
			break
		}
	}
	if hatchResult == nil || hatchResult.Status != analysis.CheckPass {
		t.Fatalf("results: %+v", results)
	}
}
