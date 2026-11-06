package snapshot

import (
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
)

// ExactInt holds an integer parsed without float64 rounding when possible.
type ExactInt struct {
	Int         *big.Int
	Text        string // canonical decimal text when exact
	Approximate bool   // true if source used scientific notation / float form
	SourceText  string // original token
	ParseOK     bool
}

func (e ExactInt) String() string {
	if e.Text != "" {
		return e.Text
	}
	if e.Int != nil {
		return e.Int.String()
	}
	return e.SourceText
}

func (e ExactInt) IsZero() bool {
	return e.ParseOK && e.Int != nil && e.Int.Sign() == 0
}

// ParseExactInt parses JSON number tokens, decimal strings, or scientific notation.
func ParseExactInt(raw json.RawMessage) ExactInt {
	if len(raw) == 0 || string(raw) == "null" {
		return ExactInt{ParseOK: false, SourceText: string(raw)}
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s = strings.TrimSpace(str)
		}
	}
	return ParseExactIntString(s)
}

func ParseExactIntString(s string) ExactInt {
	out := ExactInt{SourceText: s}
	s = strings.TrimSpace(s)
	if s == "" {
		return out
	}
	if n, ok := new(big.Int).SetString(s, 10); ok {
		out.Int = n
		out.Text = n.String()
		out.ParseOK = true
		return out
	}
	// Scientific / float form — parse via big.Float then require integer value.
	out.Approximate = true
	f, _, err := big.ParseFloat(s, 10, 256, big.ToNearestEven)
	if err != nil {
		return out
	}
	n, acc := f.Int(nil)
	if acc != big.Exact {
		// Still return nearest int but mark approximate.
		out.Int = n
		out.Text = n.String()
		out.ParseOK = true
		out.Approximate = true
		return out
	}
	out.Int = n
	out.Text = n.String()
	out.ParseOK = true
	return out
}

// SubExact returns b - a as ExactInt (new - old).
func SubExact(neu, old ExactInt) (ExactInt, error) {
	if !neu.ParseOK || !old.ParseOK || neu.Int == nil || old.Int == nil {
		return ExactInt{}, fmt.Errorf("cannot subtract non-exact integers")
	}
	d := new(big.Int).Sub(neu.Int, old.Int)
	return ExactInt{
		Int:         d,
		Text:        d.String(),
		ParseOK:     true,
		Approximate: neu.Approximate || old.Approximate,
		SourceText:  d.String(),
	}, nil
}

func AddExact(a, b ExactInt) ExactInt {
	if !a.ParseOK || !b.ParseOK || a.Int == nil || b.Int == nil {
		return ExactInt{}
	}
	s := new(big.Int).Add(a.Int, b.Int)
	return ExactInt{Int: s, Text: s.String(), ParseOK: true, Approximate: a.Approximate || b.Approximate, SourceText: s.String()}
}

func MulInt64(a ExactInt, n int64) ExactInt {
	if !a.ParseOK || a.Int == nil {
		return ExactInt{}
	}
	s := new(big.Int).Mul(a.Int, big.NewInt(n))
	return ExactInt{Int: s, Text: s.String(), ParseOK: true, Approximate: a.Approximate, SourceText: s.String()}
}
