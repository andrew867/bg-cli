package publicintel

import (
	"encoding/json"
	"fmt"
	"strconv"
)

// ExactInt preserves integer counts without float rounding.
type ExactInt struct {
	Value   *int64 `json:"value,omitempty"`
	Text    string `json:"text"`
	ParseOK bool   `json:"parseOk"`
}

// ParseExactIntJSON parses a JSON number or decimal string token.
func ParseExactIntJSON(raw json.RawMessage) ExactInt {
	if len(raw) == 0 || string(raw) == "null" {
		return ExactInt{Text: "null"}
	}
	var num json.Number
	if err := json.Unmarshal(raw, &num); err == nil {
		return ParseExactIntString(num.String())
	}
	var s string
	if err := json.Unmarshal(raw, &s); err == nil {
		return ParseExactIntString(s)
	}
	return ExactInt{Text: string(raw)}
}

// ParseExactIntString parses a base-10 integer string.
func ParseExactIntString(s string) ExactInt {
	out := ExactInt{Text: s}
	if s == "" {
		return out
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		// json.Number may be float without fractional part
		if f, err2 := strconv.ParseFloat(s, 64); err2 == nil && f == float64(int64(f)) {
			iv := int64(f)
			out.Value = &iv
			out.Text = strconv.FormatInt(iv, 10)
			out.ParseOK = true
			return out
		}
		return out
	}
	out.Value = &v
	out.Text = strconv.FormatInt(v, 10)
	out.ParseOK = true
	return out
}

func (e ExactInt) Int64() (int64, error) {
	if !e.ParseOK || e.Value == nil {
		return 0, fmt.Errorf("inexact integer %q", e.Text)
	}
	return *e.Value, nil
}
