// Package jsonutil helpers preserve exact JSON number text.
package jsonutil

import (
	"bytes"
	"encoding/json"
)

// Decode unmarshals JSON using json.Number for all numeric values.
func Decode(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// CloneRaw returns a copy of b suitable for embedding in another document.
func CloneRaw(b json.RawMessage) json.RawMessage {
	if b == nil {
		return nil
	}
	out := make(json.RawMessage, len(b))
	copy(out, b)
	return out
}
