package accountanalytics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestPortfolioVariantRAPMatching(t *testing.T) {
	invPath := filepath.Join("..", "..", "testdata", "fixtures", "account-inventory-ok.json")
	rapPath := filepath.Join("..", "..", "testdata", "fixtures", "rap-ok.json")
	invBytes, err := os.ReadFile(invPath)
	if err != nil {
		t.Fatal(err)
	}
	var invEnv map[string]json.RawMessage
	if err := json.Unmarshal(invBytes, &invEnv); err != nil {
		t.Fatal(err)
	}
	rapBytes, err := os.ReadFile(rapPath)
	if err != nil {
		t.Fatal(err)
	}
	var rapEnv map[string]json.RawMessage
	_ = json.Unmarshal(rapBytes, &rapEnv)
	rep, err := BuildPortfolioReport(invEnv["data"], rapEnv["data"])
	if err != nil {
		t.Fatal(err)
	}
	if rep.UnmatchedLineCount == 0 {
		t.Fatal("expected unmatched lines without full variant rap coverage")
	}
	if rep.MatchedRAPSum == "0" && rep.MatchedLineCount > 0 {
		t.Fatalf("matched rap should not be zero when lines matched: %+v", rep)
	}
	for _, line := range rep.Lines {
		if !line.RAPMatched && line.RAPLineTotal != "" {
			t.Fatalf("unmatched line must not have rap total: %+v", line)
		}
	}
}
