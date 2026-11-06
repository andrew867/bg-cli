// Package output defines the stable JSON envelope for bg-cli.
package output

import "time"

const (
	SchemaVersion    = "1.0"
	FreshnessMeaning = "cached player snapshot, not real time"
)

// Envelope is the stable stdout JSON object.
type Envelope struct {
	SchemaVersion string     `json:"schemaVersion"`
	OK            bool       `json:"ok"`
	Command       string     `json:"command"`
	Source        *Source    `json:"source"`
	Freshness     *Freshness `json:"freshness"`
	Data          any        `json:"data,omitempty"`
	Error         *ErrorBody `json:"error,omitempty"`
}

// Source describes where data was retrieved from.
type Source struct {
	Provider       string    `json:"provider"`
	Classification string    `json:"classification"`
	BaseURL        string    `json:"baseUrl"`
	Endpoint       string    `json:"endpoint"`
	HTTPStatus     int       `json:"httpStatus"`
	RetrievedAt    time.Time `json:"retrievedAt"`
	FromLocalCache bool      `json:"fromLocalCache"`
}

// Freshness describes snapshot semantics. Never invent timestamps.
type Freshness struct {
	SnapshotAt *time.Time `json:"snapshotAt"`
	IsStale    *bool      `json:"isStale"`
	Meaning    string     `json:"meaning"`
	Refresh    any        `json:"refresh"`
}

// ErrorBody is the stable error object.
type ErrorBody struct {
	Code              string         `json:"code"`
	Message           string         `json:"message"`
	Retryable         bool           `json:"retryable"`
	UpstreamStatus    *int           `json:"upstreamStatus,omitempty"`
	RetryAfterSeconds *int           `json:"retryAfterSeconds,omitempty"`
	Details           map[string]any `json:"details,omitempty"`
}

// NewSuccess builds a success envelope.
func NewSuccess(command string, source *Source, freshness *Freshness, data any) Envelope {
	if freshness == nil {
		freshness = &Freshness{
			Meaning: FreshnessMeaning,
			Refresh: nil,
		}
	}
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            true,
		Command:       command,
		Source:        source,
		Freshness:     freshness,
		Data:          data,
	}
}

// NewError builds an error envelope.
func NewError(command string, source *Source, freshness *Freshness, err *ErrorBody) Envelope {
	return Envelope{
		SchemaVersion: SchemaVersion,
		OK:            false,
		Command:       command,
		Source:        source,
		Freshness:     freshness,
		Error:         err,
	}
}

// AuthSource returns a null-ish local source for auth commands (no upstream fetch).
func LocalAuthSource() *Source {
	return nil
}

// DefaultFreshnessNull returns freshness with no invented snapshot.
func DefaultFreshnessNull() *Freshness {
	return &Freshness{
		SnapshotAt: nil,
		IsStale:    nil,
		Meaning:    FreshnessMeaning,
		Refresh:    nil,
	}
}
