package accountanalytics

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

func loadViewEnvelope(t *testing.T, name string) json.RawMessage {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "testdata", "fixtures", name))
	if err != nil {
		t.Fatal(err)
	}
	var env map[string]json.RawMessage
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatal(err)
	}
	return env["data"]
}

func TestParseMovementsFromBundle(t *testing.T) {
	b := &snapshot.Bundle{
		ViewRaw: map[string]json.RawMessage{
			"trades": loadViewEnvelope(t, "account-trades-ok.json"),
			"booth":  loadViewEnvelope(t, "account-booth-ok.json"),
			"mail":   loadViewEnvelope(t, "account-mail-ok.json"),
		},
	}
	mov, err := ParseMovements(b)
	if err != nil {
		t.Fatal(err)
	}
	if len(mov) < 3 {
		t.Fatalf("movements=%d", len(mov))
	}
	sum := SummarizeMovements(mov, true)
	if sum.MovementCount != len(mov) {
		t.Fatalf("%+v", sum)
	}
	for _, m := range sum.LargestTransfers {
		if m.CounterpartyRedacted && (m.CounterpartyUID != "" || m.CounterpartyName != "") {
			t.Fatal("counterparty should be redacted")
		}
	}
	// Trade given = out
	foundOut := false
	for _, m := range mov {
		if m.Source == "trades" && m.Direction == DirectionOut && m.ItemID == "Test Cat" {
			foundOut = true
		}
	}
	if !foundOut {
		t.Fatal("expected trade out movement")
	}
}
