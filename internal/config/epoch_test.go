package config

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
	"testing"
)

func TestNewCredentialEpochIsNotATokenHash(t *testing.T) {
	const token = "synthetic_test_access_token_do_not_use"
	sum := sha256.Sum256([]byte(token))
	hashed := hex.EncodeToString(sum[:])
	a, err := NewCredentialEpoch()
	if err != nil {
		t.Fatal(err)
	}
	b, err := NewCredentialEpoch()
	if err != nil {
		t.Fatal(err)
	}
	if a == "" || a == b || len(a) != 32 || strings.Contains(a, token) || a == hashed {
		t.Fatalf("epochs %q %q", a, b)
	}
}
