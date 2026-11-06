// Package redact removes secret material from error and panic strings.
package redact

import (
	"encoding/base64"
	"strings"
)

// Secrets holds values that must never appear in output.
type Secrets struct {
	values []string
}

// Add registers a secret value and common encodings derived from it.
func (s *Secrets) Add(raw string) {
	if raw == "" {
		return
	}
	s.values = append(s.values, raw)
	s.values = append(s.values, base64.StdEncoding.EncodeToString([]byte(raw)))
	s.values = append(s.values, base64.RawStdEncoding.EncodeToString([]byte(raw)))
	s.values = append(s.values, "Bearer "+raw)
	s.values = append(s.values, "bearer "+raw)
}

// AddBasic registers HTTP Basic credentials clientID:clientSecret.
func (s *Secrets) AddBasic(clientID, clientSecret string) {
	if clientID == "" || clientSecret == "" {
		return
	}
	pair := clientID + ":" + clientSecret
	s.Add(clientSecret)
	encoded := base64.StdEncoding.EncodeToString([]byte(pair))
	s.values = append(s.values, encoded)
	s.values = append(s.values, "Basic "+encoded)
}

// Redact replaces every registered secret substring with [REDACTED].
func (s *Secrets) Redact(in string) string {
	out := in
	for _, v := range s.values {
		if v == "" {
			continue
		}
		out = strings.ReplaceAll(out, v, "[REDACTED]")
	}
	return out
}

// ContainsSecret reports whether in contains any registered secret.
func (s *Secrets) ContainsSecret(in string) bool {
	for _, v := range s.values {
		if v != "" && strings.Contains(in, v) {
			return true
		}
	}
	return false
}
