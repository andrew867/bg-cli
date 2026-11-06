package api

import "testing"

func TestValidateBearerTarget(t *testing.T) {
	ok := "https://ps99.biggamesapi.io/v1/account/inventory"
	if err := ValidateBearerTarget(ok); err != nil {
		t.Fatal(err)
	}
	bads := []string{
		"https://evil.example/v1/account/inventory",
		"https://ps99.biggamesapi.io/v1/players/x",
		"https://ps99.biggamesapi.io/api/rap",
		"http://ps99.biggamesapi.io/v1/account/profile",
		"https://db.biggames.io/oauth/token",
	}
	for _, u := range bads {
		if err := ValidateBearerTarget(u); err == nil {
			t.Fatalf("expected error for %s", u)
		}
	}
}

func TestValidateTokenExchangeURL(t *testing.T) {
	if err := ValidateTokenExchangeURL(ProductionOAuthToken); err != nil {
		t.Fatal(err)
	}
	bads := []string{
		"https://db.biggames.io/oauth/authorize",
		"https://ps99.biggamesapi.io/oauth/token",
		"https://evil.example/oauth/token",
		"http://db.biggames.io/oauth/token",
	}
	for _, u := range bads {
		if err := ValidateTokenExchangeURL(u); err == nil {
			t.Fatalf("expected error for %s", u)
		}
	}
}

func TestIsAccountPath(t *testing.T) {
	if !IsAccountPath("/v1/account/mail") {
		t.Fatal("expected mail path")
	}
	if IsAccountPath("/v1/account/../../etc/passwd") {
		t.Fatal("path traversal must fail allowlist")
	}
}
