package snapshot

import "testing"

func TestValidatePrefix(t *testing.T) {
	if err := ValidatePrefix("ps99"); err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"", "../x", "a/b", `a\b`, "-bad", "has space"} {
		if err := ValidatePrefix(bad); err == nil {
			t.Fatalf("expected error for %q", bad)
		}
	}
}
