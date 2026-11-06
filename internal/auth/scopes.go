package auth

import (
	"fmt"
	"strings"
)

const scopePrefix = "player-data:pet-simulator-99:"

// KnownScopes are the seven documented account scopes.
var KnownScopes = []string{
	"player-data:pet-simulator-99:inventory:read",
	"player-data:pet-simulator-99:profile:read",
	"player-data:pet-simulator-99:extendedProfile:read",
	"player-data:pet-simulator-99:itemIndex:read",
	"player-data:pet-simulator-99:trades:read",
	"player-data:pet-simulator-99:booth:read",
	"player-data:pet-simulator-99:mail:read",
}

// DefaultScopes are requested by auth login unless the operator opts out
// with an explicit --scopes list. Policy is all documented account views.
var DefaultScopes = append([]string{}, KnownScopes...)

var knownSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(KnownScopes))
	for _, s := range KnownScopes {
		m[s] = struct{}{}
	}
	return m
}()

// ValidateScopes ensures every scope is a documented value.
func ValidateScopes(scopes []string) error {
	if len(scopes) == 0 {
		return fmt.Errorf("at least one scope is required")
	}
	for _, s := range scopes {
		if _, ok := knownSet[s]; !ok {
			return fmt.Errorf("unknown scope %q; must be one of the seven documented player-data scopes", s)
		}
	}
	return nil
}

// ParseScopeList splits a comma- or space-separated scope list.
func ParseScopeList(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return append([]string{}, DefaultScopes...), nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == ',' || r == ' ' || r == '+'
	})
	out := make([]string, 0, len(fields))
	seen := map[string]struct{}{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.HasPrefix(f, scopePrefix) && !strings.Contains(f, ":") {
			f = scopePrefix + f
		}
		if _, ok := seen[f]; ok {
			continue
		}
		seen[f] = struct{}{}
		out = append(out, f)
	}
	if err := ValidateScopes(out); err != nil {
		return nil, err
	}
	return out, nil
}

// ScopeString joins scopes with a single space for OAuth.
func ScopeString(scopes []string) string {
	return strings.Join(scopes, " ")
}

// HasAllScopes reports whether granted contains every requested scope.
func HasAllScopes(granted string, requested []string) bool {
	set := map[string]struct{}{}
	for _, s := range strings.Fields(granted) {
		set[s] = struct{}{}
	}
	for _, r := range requested {
		if _, ok := set[r]; !ok {
			return false
		}
	}
	return true
}

// MissingScopes returns requested scopes absent from granted.
func MissingScopes(granted string, requested []string) []string {
	set := map[string]struct{}{}
	for _, s := range strings.Fields(granted) {
		set[s] = struct{}{}
	}
	var missing []string
	for _, r := range requested {
		if _, ok := set[r]; !ok {
			missing = append(missing, r)
		}
	}
	return missing
}
