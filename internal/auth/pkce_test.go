package auth

import (
	"crypto/sha256"
	"encoding/base64"
	"testing"
)

func TestGeneratePKCELengthAndS256(t *testing.T) {
	p, err := GeneratePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Verifier) < 43 {
		t.Fatalf("verifier length %d < 43", len(p.Verifier))
	}
	if p.Method != "S256" {
		t.Fatalf("method %q", p.Method)
	}
	sum := sha256.Sum256([]byte(p.Verifier))
	want := base64.RawURLEncoding.EncodeToString(sum[:])
	if p.Challenge != want {
		t.Fatalf("challenge mismatch")
	}
}

func TestGenerateStateUnpredictable(t *testing.T) {
	a, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	b, err := GenerateState()
	if err != nil {
		t.Fatal(err)
	}
	if a == b {
		t.Fatal("state values collided")
	}
	if len(a) < 32 {
		t.Fatalf("state too short: %d", len(a))
	}
}
