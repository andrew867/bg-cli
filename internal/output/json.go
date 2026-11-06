package output

import (
	"encoding/json"
	"io"
	"time"
)

// Clock abstracts time for deterministic tests.
type Clock interface {
	Now() time.Time
}

// SystemClock uses time.Now UTC.
type SystemClock struct{}

func (SystemClock) Now() time.Time { return time.Now().UTC() }

// FixedClock returns a constant time.
type FixedClock struct{ T time.Time }

func (c FixedClock) Now() time.Time { return c.T.UTC() }

// Writer writes envelopes to stdout-like destinations.
type Writer struct {
	Out    io.Writer
	Pretty bool
	Clock  Clock
}

func (w Writer) clock() Clock {
	if w.Clock != nil {
		return w.Clock
	}
	return SystemClock{}
}

// Write encodes one JSON object to Out. Never writes diagnostics.
func (w Writer) Write(env Envelope) error {
	enc := json.NewEncoder(w.Out)
	if w.Pretty {
		enc.SetIndent("", "  ")
	}
	enc.SetEscapeHTML(false)
	return enc.Encode(env)
}

// StampRetrievedAt sets source.RetrievedAt from the clock when source is non-nil
// and RetrievedAt is zero.
func (w Writer) StampRetrievedAt(source *Source) {
	if source == nil {
		return
	}
	if source.RetrievedAt.IsZero() {
		source.RetrievedAt = w.clock().Now()
	}
}
