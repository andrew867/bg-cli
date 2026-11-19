package snapview

import (
	"strings"
	"testing"
)

func TestFormatCountAndSigned(t *testing.T) {
	if got := FormatCount("11492509"); got != "11,492,509" {
		t.Fatalf("commas: %s", got)
	}
	if got := FormatCount("1276000000000"); got != "1.276T" {
		t.Fatalf("trillions: %s", got)
	}
	if got := FormatCount("136415000000"); got != "136.415B" {
		t.Fatalf("billions: %s", got)
	}
	if got := FormatSigned("161506"); got != "+161,506" {
		t.Fatalf("plus: %s", got)
	}
	if got := FormatSigned("-8236"); got != "-8,236" {
		t.Fatalf("minus: %s", got)
	}
	if got := FormatCount(""); got != "n/a" {
		t.Fatalf("empty: %s", got)
	}
	if got := FormatSigned(""); got != "n/a" || strings.Contains(got, "0") && got != "n/a" {
		t.Fatalf("missing delta must not be zero: %s", got)
	}
	huge := "123456789012345678901234567890"
	got := FormatCount(huge)
	if strings.Contains(got, "e+") || strings.Contains(got, ".") && strings.Contains(got, "e") {
		t.Fatalf("float scientific: %s", got)
	}
	if !strings.Contains(got, "Qa") && !strings.Contains(got, "T") {
		t.Fatalf("huge display: %s", got)
	}
}

func TestFormatDurationRankAndRate(t *testing.T) {
	s := int64(5*3600 + 23*60 + 46)
	if got := FormatDuration(&s); got != "5h 23m 46s" {
		t.Fatalf("duration: %s", got)
	}
	play := int64(550*3600 + 35*60)
	if got := FormatDuration(&play); got != "550h 35m" {
		t.Fatalf("playtime: %s", got)
	}
	var missing *int64
	if got := FormatDuration(missing); got != "n/a" {
		t.Fatalf("missing duration: %s", got)
	}
	rank := int64(1343)
	one := int64(1)
	zero := int64(0)
	neg := int64(-1)
	if got := FormatRank(&rank); got != "#1,343" {
		t.Fatalf("rank: %s", got)
	}
	if got := FormatRank(&one); got != "#1" {
		t.Fatalf("one: %s", got)
	}
	if got := FormatRank(nil); got != "n/a" {
		t.Fatalf("nil rank: %s", got)
	}
	if got := FormatRank(&zero); got != "n/a" {
		t.Fatalf("zero rank: %s", got)
	}
	if got := FormatRank(&neg); got != "n/a" {
		t.Fatalf("negative rank: %s", got)
	}
	if got := FormatRatePerHour("47500", 3600); got != "~47.5k/h" {
		t.Fatalf("rate: %s", got)
	}
	if got := FormatRatePerHour("10", 0); got != "n/a" {
		t.Fatalf("zero elapsed: %s", got)
	}
	if got := FormatRatePerHour("", 3600); got != "n/a" {
		t.Fatalf("missing eggs: %s", got)
	}
}
