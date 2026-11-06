package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPublicCheckRedirectSameHostHTTPLoopbackAllowed(t *testing.T) {
	final := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","data":[]}`))
	}))
	defer final.Close()

	// Same-host redirect on the final server.
	mux := http.NewServeMux()
	mux.HandleFunc("/api/collections", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/api/collections/final", http.StatusFound)
	})
	mux.HandleFunc("/api/collections/final", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","data":["Pets"]}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := &Client{BaseURL: srv.URL, HTTPClient: srv.Client()}
	res, err := c.GetPublic(context.Background(), "/api/collections", nil, BodyLimitDefault)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(res.Data), "Pets") {
		t.Fatalf("body %s", res.Body)
	}
}

func TestPublicCheckRedirectCrossHostRejected(t *testing.T) {
	evil := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"status":"ok","data":["evil"]}`))
	}))
	defer evil.Close()
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, evil.URL+"/api/collections", http.StatusFound)
	}))
	defer origin.Close()

	c := &Client{BaseURL: origin.URL, HTTPClient: origin.Client()}
	_, err := c.GetPublic(context.Background(), "/api/collections", nil, BodyLimitDefault)
	if err == nil {
		t.Fatal("expected cross-host redirect error")
	}
	if !strings.Contains(err.Error(), "same host") && !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("err=%v", err)
	}
}
