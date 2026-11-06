package redact

import (
	"encoding/base64"
	"testing"
)

func TestRedactSecrets(t *testing.T) {
	s := &Secrets{}
	s.Add("synthetic_test_access_token_do_not_use")
	s.AddBasic("client-id", "super-secret-value")
	s.Add("pkce-verifier-canary")
	s.Add("auth-code-canary")

	in := "Bearer synthetic_test_access_token_do_not_use Basic " +
		base64.StdEncoding.EncodeToString([]byte("client-id:super-secret-value")) +
		" code=auth-code-canary verifier=pkce-verifier-canary"
	out := s.Redact(in)
	if s.ContainsSecret(out) {
		t.Fatalf("redacted output still contains secret: %s", out)
	}
	if out == in {
		t.Fatal("expected redaction to change string")
	}
}
