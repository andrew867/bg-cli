// Package snapview renders a snapshot for a person. JSON and CSV stay canonical.
package snapview

import (
	"math/big"
	"strconv"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/clan"
)

const unavailable = "n/a"

// FormatLocalClock prints t in loc as 2006-01-02 15:04:05 MST.
// A nil location uses the local computer timezone.
func FormatLocalClock(t time.Time, loc *time.Location) string {
	if loc == nil {
		loc = time.Local
	}
	return t.In(loc).Format("2006-01-02 15:04:05 MST")
}

// FormatCount prints an exact integer with thousands separators.
// Values of at least one billion may use K, M, B, T, or Qa.
// The result is display text. It is not a stored amount.
func FormatCount(s string) string {
	n, ok := clan.ParseExactInt(s)
	if !ok {
		if strings.TrimSpace(s) == "" {
			return unavailable
		}
		return strings.TrimSpace(s)
	}
	return formatInt(n, false)
}

// FormatSigned prints a delta with a leading + or -.
// An empty or non-integer value is n/a, never zero.
func FormatSigned(s string) string {
	n, ok := clan.ParseExactInt(s)
	if !ok {
		return unavailable
	}
	if n.Sign() == 0 {
		return "0"
	}
	sign := "+"
	if n.Sign() < 0 {
		sign = "-"
		n = new(big.Int).Abs(n)
	}
	return sign + formatInt(n, false)
}

// FormatRank prints a 1-based place as #1,343.
// A missing, zero, or negative rank is n/a.
func FormatRank(n *int64) string {
	if n == nil || *n <= 0 {
		return unavailable
	}
	return "#" + withCommas(new(big.Int).SetInt64(*n).String())
}

// FormatDuration prints seconds as 5h 23m 46s. Zero parts are omitted.
// A negative or missing duration is n/a, never zero.
func FormatDuration(seconds *int64) string {
	if seconds == nil || *seconds < 0 {
		return unavailable
	}
	s := *seconds
	h := s / 3600
	m := (s % 3600) / 60
	sec := s % 60
	var parts []string
	if h > 0 {
		parts = append(parts, itoa(h)+"h")
	}
	if m > 0 {
		parts = append(parts, itoa(m)+"m")
	}
	if sec > 0 || len(parts) == 0 {
		parts = append(parts, itoa(sec)+"s")
	}
	return strings.Join(parts, " ")
}

// FormatRatePerHour prints an approximate eggs-per-hour rate such as ~47.5k/h.
// elapsed must be positive. The tilde marks the figure as approximate.
func FormatRatePerHour(delta string, elapsed int64) string {
	return formatRate(delta, elapsed, 3600, "/h")
}

// FormatRatePerMinute prints an approximate per-minute rate such as ~791/m.
// elapsed must be the actual elapsed seconds. The tilde marks the figure as approximate.
func FormatRatePerMinute(delta string, elapsed int64) string {
	return formatRate(delta, elapsed, 60, "/m")
}

func formatRate(delta string, elapsed, period int64, suffix string) string {
	text, ok := ratePer(delta, elapsed, period)
	if !ok {
		return unavailable
	}
	return "~" + text + suffix
}

func ratePer(delta string, elapsed, period int64) (string, bool) {
	if elapsed <= 0 || period <= 0 {
		return "", false
	}
	n, ok := clan.ParseExactInt(delta)
	if !ok {
		return "", false
	}
	if n.Sign() == 0 {
		return "0", true
	}
	num := new(big.Int).Mul(n, big.NewInt(period))
	return formatRationalRate(num, big.NewInt(elapsed)), true
}

// formatRationalRate prints about two or three significant digits.
// Rounding is display-only. A nonzero rate is not shown as zero.
func formatRationalRate(num, den *big.Int) string {
	if den.Sign() == 0 {
		return "0"
	}
	neg := num.Sign() < 0
	absNum := new(big.Int).Abs(num)
	absDen := new(big.Int).Abs(den)
	pow, suf := chooseScale(absNum, absDen)
	scaledDen := new(big.Int).Mul(absDen, pow10(pow))
	decimals := decimalsFor(absNum, scaledDen)
	text := "0"
	for {
		rounded, zero := roundDecimal(absNum, scaledDen, decimals)
		text = rounded
		if !zero {
			break
		}
		if decimals >= 12 {
			break
		}
		decimals++
	}
	if text == "0" {
		return "0"
	}
	if neg {
		return "-" + text + suf
	}
	return text + suf
}

func chooseScale(num, den *big.Int) (int, string) {
	for _, g := range []struct {
		pow int
		suf string
	}{
		{15, "qa"},
		{12, "t"},
		{9, "b"},
		{6, "m"},
		{3, "k"},
	} {
		if num.Cmp(new(big.Int).Mul(den, pow10(g.pow))) >= 0 {
			return g.pow, g.suf
		}
	}
	return 0, ""
}

func decimalsFor(num, den *big.Int) int {
	if cmpWhole(num, den, 1) < 0 || cmpWhole(num, den, 10) < 0 {
		return 2
	}
	if cmpWhole(num, den, 100) < 0 {
		return 1
	}
	return 0
}

func cmpWhole(num, den *big.Int, whole int64) int {
	return num.Cmp(new(big.Int).Mul(den, big.NewInt(whole)))
}

func roundDecimal(num, den *big.Int, decimals int) (string, bool) {
	if decimals < 0 {
		decimals = 0
	}
	n := new(big.Int).Mul(num, pow10(decimals))
	q, r := new(big.Int).QuoRem(n, den, new(big.Int))
	if new(big.Int).Lsh(r, 1).Cmp(den) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if q.Sign() == 0 {
		return "0", true
	}
	digits := q.String()
	if decimals == 0 {
		return withCommas(digits), false
	}
	if len(digits) <= decimals {
		digits = strings.Repeat("0", decimals-len(digits)+1) + digits
	}
	whole := digits[:len(digits)-decimals]
	frac := strings.TrimRight(digits[len(digits)-decimals:], "0")
	if frac == "" {
		return withCommas(whole), false
	}
	return withCommas(whole) + "." + frac, false
}

func pow10(n int) *big.Int {
	if n <= 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(n)), nil)
}

// FormatSpan prints a compact duration with days, such as 1d 9h 12m.
// Seconds are omitted once the span is at least a day.
func FormatSpan(seconds int64) string {
	if seconds < 0 {
		return unavailable
	}
	if seconds == 0 {
		return "0s"
	}
	d := seconds / 86400
	h := (seconds % 86400) / 3600
	m := (seconds % 3600) / 60
	s := seconds % 60
	var parts []string
	if d > 0 {
		parts = append(parts, strconv.FormatInt(d, 10)+"d")
	}
	if h > 0 {
		parts = append(parts, strconv.FormatInt(h, 10)+"h")
	}
	if m > 0 {
		parts = append(parts, strconv.FormatInt(m, 10)+"m")
	}
	if s > 0 && d == 0 {
		parts = append(parts, strconv.FormatInt(s, 10)+"s")
	}
	if len(parts) == 0 {
		return "0s"
	}
	return strings.Join(parts, " ")
}

// FormatShare prints user contribution as a percent of clan-point gain.
// It is presentation-only. A non-positive clan gain is n/a, not a division.
func FormatShare(userDelta, clanDelta string) string {
	user, uOK := clan.ParseExactInt(userDelta)
	clanGain, cOK := clan.ParseExactInt(clanDelta)
	if !uOK || !cOK || clanGain.Sign() <= 0 || user.Sign() < 0 {
		return unavailable
	}
	num := new(big.Int).Mul(user, big.NewInt(1000))
	q, r := new(big.Int).QuoRem(num, clanGain, new(big.Int))
	if new(big.Int).Lsh(r, 1).Cmp(clanGain) >= 0 {
		q.Add(q, big.NewInt(1))
	}
	if q.Sign() < 0 {
		q.Abs(q)
	}
	digits := q.String()
	if len(digits) < 2 {
		digits = strings.Repeat("0", 2-len(digits)) + digits
	}
	whole := digits[:len(digits)-1]
	frac := digits[len(digits)-1:]
	if whole == "" {
		whole = "0"
	}
	if frac == "0" {
		return whole + "%"
	}
	return whole + "." + frac + "%"
}

func formatInt(n *big.Int, rate bool) string {
	neg := n.Sign() < 0
	if neg {
		n = new(big.Int).Abs(n)
	}
	digits := n.String()
	text := digits
	if rate {
		text = scaleRate(digits)
	} else if len(digits) > 9 {
		text = scaleAmount(digits)
	} else {
		text = withCommas(digits)
	}
	if neg {
		return "-" + text
	}
	return text
}

func scaleAmount(digits string) string {
	return scale(digits, []group{
		{15, "Qa"},
		{12, "T"},
		{9, "B"},
		{6, "M"},
		{3, "K"},
	}, 3)
}

func scaleRate(digits string) string {
	if len(digits) <= 3 {
		return withCommas(digits)
	}
	return scale(digits, []group{
		{15, "qa"},
		{12, "t"},
		{9, "b"},
		{6, "m"},
		{3, "k"},
	}, 1)
}

type group struct {
	pow int
	suf string
}

func scale(digits string, groups []group, decimals int) string {
	for _, g := range groups {
		if len(digits) > g.pow {
			return scaled(digits, g.pow, g.suf, decimals)
		}
	}
	return withCommas(digits)
}

// scaled shows whole.frac with at most decimals places, rounded half up
// from the remaining integer digits. Trailing zeros in the fraction are dropped.
func scaled(digits string, pow int, suf string, decimals int) string {
	if pow <= 0 || pow >= len(digits) {
		return withCommas(digits)
	}
	whole := digits[:len(digits)-pow]
	frac := digits[len(digits)-pow:]
	if decimals < 1 {
		decimals = 1
	}
	if len(frac) > decimals {
		keep := frac[:decimals]
		if frac[decimals] >= '5' {
			whole, keep = roundFrac(whole, keep)
		}
		frac = keep
	} else {
		frac = frac[:min(decimals, len(frac))]
	}
	frac = strings.TrimRight(frac, "0")
	if frac == "" {
		return whole + suf
	}
	return whole + "." + frac + suf
}

func roundFrac(whole, frac string) (string, string) {
	i := len(frac) - 1
	buf := []byte(frac)
	for i >= 0 {
		if buf[i] < '9' {
			buf[i]++
			return whole, string(buf)
		}
		buf[i] = '0'
		i--
	}
	n, ok := new(big.Int).SetString(whole, 10)
	if !ok {
		return whole, string(buf)
	}
	n.Add(n, big.NewInt(1))
	return n.String(), string(buf)
}

func withCommas(digits string) string {
	if digits == "" {
		return "0"
	}
	n := len(digits)
	if n <= 3 {
		return digits
	}
	lead := n % 3
	if lead == 0 {
		lead = 3
	}
	var b strings.Builder
	b.WriteString(digits[:lead])
	for i := lead; i < n; i += 3 {
		b.WriteByte(',')
		b.WriteString(digits[i : i+3])
	}
	return b.String()
}

func itoa(n int64) string {
	return new(big.Int).SetInt64(n).String()
}
