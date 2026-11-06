package auth

import (
	"testing"
)

func TestDefaultScopesIncludeAllKnown(t *testing.T) {
	if len(DefaultScopes) != len(KnownScopes) {
		t.Fatalf("DefaultScopes=%d KnownScopes=%d", len(DefaultScopes), len(KnownScopes))
	}
	for _, s := range KnownScopes {
		found := false
		for _, d := range DefaultScopes {
			if d == s {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("default missing %s", s)
		}
	}
}

func TestParseScopeListEmptyUsesAllDefaults(t *testing.T) {
	got, err := ParseScopeList("")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(KnownScopes) {
		t.Fatalf("got %d scopes", len(got))
	}
}

func TestParseScopeListOptOutSubset(t *testing.T) {
	got, err := ParseScopeList("inventory:read,profile:read")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got %#v", got)
	}
}
