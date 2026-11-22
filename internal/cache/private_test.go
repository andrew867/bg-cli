package cache

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPrivateResponsesAreNotCached(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	secret := []byte(`{"access_token":"synthetic_do_not_store","items":["private"]}`)
	cases := []string{
		"https://ps99.biggamesapi.io/v1/account/inventory",
		"https://ps99.biggamesapi.io/v1/account/profile",
		"https://db.biggames.io/account.data",
		"https://db.biggames.io/account/inventory.data?account=all",
		"https://db.biggames.io/account/inventory.data?account=linked-synthetic",
	}
	for _, raw := range cases {
		if err := s.Put(raw, 200, secret, "application/json", time.Hour, now); err == nil {
			t.Fatalf("cached %s", raw)
		}
		if _, _, ok, err := s.Get(raw, now); err != nil || ok {
			t.Fatalf("get %s ok=%v err=%v", raw, ok, err)
		}
	}
	ents, err := os.ReadDir(dir)
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(ents) != 0 {
		t.Fatalf("cache files %d", len(ents))
	}
}

func TestPublicCacheDoesNotMixURLs(t *testing.T) {
	dir := t.TempDir()
	s := Store{Dir: dir}
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	mainURL := "https://ps99.biggamesapi.io/api/clan/MAIN"
	altURL := "https://ps99.biggamesapi.io/api/clan/ALT"
	if err := s.Put(mainURL, 200, []byte("main-body"), "application/json", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	if err := s.Put(altURL, 200, []byte("alt-body"), "application/json", time.Hour, now); err != nil {
		t.Fatal(err)
	}
	_, mainBody, ok, err := s.Get(mainURL, now)
	if err != nil || !ok || string(mainBody) != "main-body" {
		t.Fatalf("main %q ok=%v err=%v", mainBody, ok, err)
	}
	_, altBody, ok, err := s.Get(altURL, now)
	if err != nil || !ok || string(altBody) != "alt-body" {
		t.Fatalf("alt %q ok=%v err=%v", altBody, ok, err)
	}
	ents, _ := os.ReadDir(dir)
	for _, e := range ents {
		b, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(b), "Authorization") || strings.Contains(string(b), "Bearer") {
			t.Fatalf("cache entry stored a credential: %s", e.Name())
		}
	}
}

func TestPrivateCacheControlIsRefused(t *testing.T) {
	if !Refuses("https://ps99.biggamesapi.io/api/clan/TAG", "private, no-cache") {
		t.Fatal("private cache-control was cacheable")
	}
}
