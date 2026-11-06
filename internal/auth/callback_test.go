package auth

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestCallbackStateMatchAndMismatch(t *testing.T) {
	ln, err := StartLoopback("http://127.0.0.1:0/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ln.Timeout = 3 * time.Second
	uri := ln.RedirectURI("/callback")

	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = http.Get(uri + "?code=abc&state=good-state")
	}()
	res, err := ln.Wait(context.Background(), "good-state")
	if err != nil {
		t.Fatal(err)
	}
	if res.Code != "abc" {
		t.Fatalf("code %q", res.Code)
	}

	ln2, err := StartLoopback("http://127.0.0.1:0/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer ln2.Close()
	ln2.Timeout = 3 * time.Second
	uri2 := ln2.RedirectURI("/callback")
	go func() {
		time.Sleep(50 * time.Millisecond)
		_, _ = http.Get(uri2 + "?code=abc&state=wrong")
	}()
	_, err = ln2.Wait(context.Background(), "good-state")
	if err == nil || err.Error() != "oauth state mismatch" {
		t.Fatalf("expected state mismatch, got %v", err)
	}
}

func TestCallbackRejectsWrongPathAndError(t *testing.T) {
	ln, err := StartLoopback("http://127.0.0.1:0/callback")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	ln.Timeout = 2 * time.Second
	base := fmt.Sprintf("http://%s", ln.Listener.Addr().String())
	go func() {
		time.Sleep(30 * time.Millisecond)
		_, _ = http.Get(base + "/other?code=x&state=s")
	}()
	_, err = ln.Wait(context.Background(), "s")
	if err == nil {
		t.Fatal("expected path error")
	}
}

func TestCallbackRejectsNonLoopback(t *testing.T) {
	_, err := StartLoopback("https://example.com/callback")
	if err == nil {
		t.Fatal("expected error")
	}
}
