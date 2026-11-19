package clan

import (
	"math/big"
	"regexp"
	"strconv"
	"strings"
)

// amountRE matches a base-10 integer, decimal, or scientific literal.
// Parsing uses integer arithmetic so values never pass through float64.
var amountRE = regexp.MustCompile(`^([+-]?)(\d+)(?:\.(\d+))?(?:[eE]([+-]?\d+))?$`)

// ParseExactInt reads an integer literal, including a decimal or scientific
// token that is an exact integer. It returns false for fractions and junk.
func ParseExactInt(s string) (*big.Int, bool) {
	s = strings.TrimSpace(s)
	if s == "" || s == "null" {
		return nil, false
	}
	if z, ok := new(big.Int).SetString(s, 10); ok {
		return z, true
	}
	m := amountRE.FindStringSubmatch(s)
	if m == nil {
		return nil, false
	}
	digits := m[2] + m[3]
	n := new(big.Int)
	if _, ok := n.SetString(digits, 10); !ok {
		return nil, false
	}
	if m[1] == "-" {
		n.Neg(n)
	}
	exp := 0
	if m[4] != "" {
		e, err := strconv.Atoi(m[4])
		if err != nil {
			return nil, false
		}
		exp = e
	}
	scale := exp - len(m[3])
	if scale > 0 {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(scale)), nil)
		n.Mul(n, pow)
		return n, true
	}
	if scale < 0 {
		pow := new(big.Int).Exp(big.NewInt(10), big.NewInt(int64(-scale)), nil)
		if pow.Sign() == 0 {
			return nil, false
		}
		quot, rem := new(big.Int).QuoRem(n, pow, new(big.Int))
		if rem.Sign() != 0 {
			return nil, false
		}
		return quot, true
	}
	return n, true
}

// CanonicalAmount returns the exact integer text when the literal is an integer.
// Otherwise it returns the trimmed original text.
func CanonicalAmount(s string) string {
	if n, ok := ParseExactInt(s); ok {
		return n.String()
	}
	return strings.TrimSpace(s)
}

// SameAmount reports whether two stored amounts are the same exact integer,
// or the same raw text when either side is not an integer.
func SameAmount(a, b string) bool {
	x, ok1 := ParseExactInt(a)
	y, ok2 := ParseExactInt(b)
	if ok1 && ok2 {
		return x.Cmp(y) == 0
	}
	return strings.TrimSpace(a) == strings.TrimSpace(b)
}

// Sub returns current-baseline as an exact integer string.
func Sub(current, baseline string) (string, bool) {
	a, ok1 := ParseExactInt(current)
	b, ok2 := ParseExactInt(baseline)
	if !ok1 || !ok2 {
		return "", false
	}
	return new(big.Int).Sub(a, b).String(), true
}

// NetPerHour is delta / elapsedSeconds * 3600 using exact integer arithmetic.
// Integral results are decimal integers. Other results are a reduced fraction.
func NetPerHour(delta string, elapsedSeconds int64) (string, bool) {
	exact, _, ok := NetPerHourParts(delta, elapsedSeconds)
	return exact, ok
}

// NetPerHourParts returns the exact rate and a decimal approximation.
// The exact form is an integer or a reduced fraction. The approximation is
// decimal text and is not used for stored balances or deltas.
func NetPerHourParts(delta string, elapsedSeconds int64) (exact, approx string, ok bool) {
	return netPerPeriodParts(delta, elapsedSeconds, 3600)
}

// NetPerMinuteParts is delta / elapsedSeconds * 60, with the same exact
// fraction and decimal approximation as NetPerHourParts.
func NetPerMinuteParts(delta string, elapsedSeconds int64) (exact, approx string, ok bool) {
	return netPerPeriodParts(delta, elapsedSeconds, 60)
}

func netPerPeriodParts(delta string, elapsedSeconds, periodSeconds int64) (exact, approx string, ok bool) {
	if elapsedSeconds <= 0 || periodSeconds <= 0 {
		return "", "", false
	}
	d, ok := ParseExactInt(delta)
	if !ok {
		return "", "", false
	}
	num := new(big.Int).Mul(d, big.NewInt(periodSeconds))
	den := big.NewInt(elapsedSeconds)
	g := new(big.Int).GCD(nil, nil, new(big.Int).Abs(num), den)
	if g.Sign() == 0 {
		return "", "", false
	}
	num.Quo(num, g)
	den.Quo(den, g)
	approx = decimalApprox(num, den)
	if den.Cmp(big.NewInt(1)) == 0 {
		return num.String(), approx, true
	}
	return num.String() + "/" + den.String(), approx, true
}

func decimalApprox(num, den *big.Int) string {
	if den.Cmp(big.NewInt(1)) == 0 {
		return num.String()
	}
	return trimDecimal(new(big.Rat).SetFrac(num, den).FloatString(6))
}

func trimDecimal(s string) string {
	if !strings.Contains(s, ".") {
		return s
	}
	s = strings.TrimRight(s, "0")
	s = strings.TrimRight(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}
