package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBearerIsNotSentToDashboardRoutes(t *testing.T) {
	var called bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.Header.Get("Authorization") != "" {
			t.Error("bearer was sent")
		}
	}))
	defer srv.Close()
	client := &Client{BaseURL: "https://db.biggames.io", HTTPClient: srv.Client()}
	for _, path := range []string{"/account.data", "/account/inventory.data"} {
		_, err := client.GetAccount(context.Background(), path, nil, "synthetic_bearer_do_not_send", 1<<20)
		if err == nil {
			t.Fatalf("path %s was allowed", path)
		}
		if strings.Contains(err.Error(), "synthetic_bearer_do_not_send") {
			t.Fatalf("error leaked the bearer: %v", err)
		}
	}
	if called {
		t.Fatal("dashboard route was requested")
	}
	for _, raw := range []string{
		"https://db.biggames.io/account.data",
		"https://db.biggames.io/account/inventory.data?account=all",
		"https://db.biggames.io/account/inventory.data?account=synthetic-linked",
		"https://ps99.biggamesapi.io/account.data",
	} {
		if err := ValidateBearerTarget(raw); err == nil {
			t.Fatalf("allowlisted %s", raw)
		}
	}
}
