package snapshot

import (
	"math/big"
	"testing"
)

func TestParseScientificAndHuge(t *testing.T) {
	e := ParseExactIntString("1.23e+3")
	if !e.ParseOK || !e.Approximate || e.Text != "1230" {
		t.Fatalf("%+v", e)
	}
	huge := "9007199254740993"
	h := ParseExactIntString(huge)
	if !h.ParseOK || h.Approximate || h.Text != huge {
		t.Fatalf("%+v", h)
	}
	d, err := SubExact(h, ParseExactIntString("1"))
	if err != nil || d.Text != "9007199254740992" {
		t.Fatalf("%v %v", d, err)
	}
}

func TestZeroEggDeltaUndefinedRate(t *testing.T) {
	r := ComputeHatchRate(HatchRateInput{
		EggsPurchasedDelta: ParseExactIntString("0"),
		PetHatchDelta:      ParseExactIntString("0"),
	}, nil)
	if r.ObservedRate != nil {
		t.Fatalf("%+v", r)
	}
}

func TestNegativeDeltaFlag(t *testing.T) {
	d := DiffExactMaps(
		map[string]ExactInt{"Eggs Opened": ParseExactIntString("10")},
		map[string]ExactInt{"Eggs Opened": ParseExactIntString("20")},
	)
	negs := FlagNegative(d)
	if len(negs) != 1 {
		t.Fatalf("%v", negs)
	}
}

func TestSumPositive(t *testing.T) {
	s := SumPositiveDeltas(map[string]ExactInt{
		"a": ParseExactIntString("5"),
		"b": ParseExactIntString("-2"),
		"c": ParseExactIntString("7"),
	})
	if s.Text != "12" {
		t.Fatalf("%s", s.Text)
	}
	_ = big.NewInt(0)
}
