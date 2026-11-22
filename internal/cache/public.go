// Package cache provides an optional on-disk cache for public API responses only.
package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Store is a simple public response cache. It never stores Authorization headers.
type Store struct {
	Dir string
}

type entry struct {
	URL         string    `json:"url"`
	Status      int       `json:"status"`
	Body        []byte    `json:"body"`
	ContentType string    `json:"contentType"`
	StoredAt    time.Time `json:"storedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
}

// DirPath returns the cache directory.
func DirPath() (string, error) {
	if v := os.Getenv("BG_CLI_CACHE_DIR"); v != "" {
		return v, nil
	}
	base, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "bg-cli", "public"), nil
}

func (s Store) pathFor(rawURL string) (string, error) {
	dir := s.Dir
	if dir == "" {
		var err error
		dir, err = DirPath()
		if err != nil {
			return "", err
		}
	}
	sum := sha256.Sum256([]byte(rawURL))
	name := hex.EncodeToString(sum[:]) + ".json"
	return filepath.Join(dir, name), nil
}

// Get returns a cached body when present and unexpired.
func (s Store) Get(rawURL string, now time.Time) (status int, body []byte, ok bool, err error) {
	if Refuses(rawURL, "") {
		return 0, nil, false, nil
	}
	path, err := s.pathFor(rawURL)
	if err != nil {
		return 0, nil, false, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return 0, nil, false, nil
		}
		return 0, nil, false, err
	}
	var e entry
	if err := json.Unmarshal(data, &e); err != nil {
		_ = os.Remove(path)
		return 0, nil, false, nil
	}
	if now.After(e.ExpiresAt) {
		_ = os.Remove(path)
		return 0, nil, false, nil
	}
	return e.Status, e.Body, true, nil
}

// Refuses reports a response that must not be stored. Account payloads,
// db.biggames.io responses, and private or no-cache documents stay out of
// the public cache so one account cannot be read back as another.
func Refuses(rawURL, cacheControl string) bool {
	cc := strings.ToLower(cacheControl)
	if strings.Contains(cc, "private") || strings.Contains(cc, "no-cache") {
		return true
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return true
	}
	host := strings.ToLower(u.Hostname())
	if host == "db.biggames.io" {
		return true
	}
	if strings.Contains(u.Path, "/v1/account") {
		return true
	}
	return false
}

// Put stores a public response atomically. maxAge <= 0 skips caching.
// A private or account URL is refused and nothing is written.
func (s Store) Put(rawURL string, status int, body []byte, contentType string, maxAge time.Duration, now time.Time) error {
	if Refuses(rawURL, "") {
		return fmt.Errorf("refusing to cache a private response")
	}
	if maxAge <= 0 || status < 200 || status >= 300 {
		return nil
	}
	path, err := s.pathFor(rawURL)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	e := entry{
		URL:         rawURL,
		Status:      status,
		Body:        body,
		ContentType: contentType,
		StoredAt:    now.UTC(),
		ExpiresAt:   now.UTC().Add(maxAge),
	}
	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// ParseMaxAge extracts max-age from a Cache-Control header.
func ParseMaxAge(cacheControl string) time.Duration {
	parts := strings.Split(cacheControl, ",")
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if strings.HasPrefix(p, "max-age=") {
			n, err := strconv.Atoi(strings.TrimPrefix(p, "max-age="))
			if err != nil || n <= 0 {
				return 0
			}
			return time.Duration(n) * time.Second
		}
	}
	return 0
}
