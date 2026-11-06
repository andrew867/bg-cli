package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func TestAuthStatusLocalNoNetwork(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {
				Alias:     "main",
				ClientID:  "cid",
				Scopes:    auth.DefaultScopes,
				IssuedAt:  time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				ExpiresAt: time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", "synthetic_test_access_token_do_not_use")

	var stdout, stderr bytes.Buffer
	code := Execute(Deps{
		Stdout:     &stdout,
		Stderr:     &stderr,
		TokenStore: store,
		Clock:      output.FixedClock{T: time.Date(2026, 11, 6, 12, 0, 0, 0, time.UTC)},
	}, []string{"auth", "status", "--profile", "main"})
	if code != exitcode.OK {
		t.Fatalf("exit %d stderr=%s stdout=%s", code, stderr.String(), stdout.String())
	}
	if strings.Contains(stdout.String(), "synthetic_test_access_token") {
		t.Fatal("token leaked to stdout")
	}
	var env output.Envelope
	if err := json.Unmarshal(stdout.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if !env.OK || env.Command != "auth status" {
		t.Fatalf("%+v", env)
	}
	data, _ := env.Data.(map[string]any)
	if data["tokenPresent"] != true {
		t.Fatalf("data %#v", data)
	}
}

func TestAuthLogout(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{
		DefaultProfile: "main",
		Profiles: map[string]config.Profile{
			"main": {Alias: "main", ClientID: "cid", Scopes: auth.DefaultScopes, ExpiresAt: time.Now().Add(time.Hour)},
		},
	}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	store := auth.NewMemoryStore()
	_ = store.Set("main", "tok")
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: store}, []string{"auth", "logout", "--profile", "main"})
	if code != 0 {
		t.Fatalf("exit %d %s", code, stderr.String())
	}
	if _, err := store.Get("main"); err == nil {
		t.Fatal("token still present")
	}
	loaded, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := loaded.Profiles["main"]; ok {
		t.Fatal("profile metadata remain")
	}
}

func TestNoClientSecretFlag(t *testing.T) {
	root := NewRoot(Deps{Stdout: ioDiscard{}, Stderr: ioDiscard{}})
	for _, c := range root.Commands() {
		if c.Name() != "auth" {
			continue
		}
		for _, sub := range c.Commands() {
			if sub.Name() != "login" {
				continue
			}
			if sub.Flags().Lookup("client-secret") != nil || sub.Flags().Lookup("secret") != nil {
				t.Fatal("client secret must not be a flag")
			}
			if sub.PersistentFlags().Lookup("client-secret") != nil {
				t.Fatal("client secret must not be a persistent flag")
			}
		}
	}
}

func TestAuthRawRejected(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr, TokenStore: auth.NewMemoryStore()},
		[]string{"--output", "raw", "auth", "profiles"})
	if code != exitcode.Usage {
		t.Fatalf("exit %d", code)
	}
}

func TestVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := Execute(Deps{Stdout: &stdout, Stderr: &stderr}, []string{"version"})
	if code != 0 {
		t.Fatal(code)
	}
	if !strings.Contains(stdout.String(), "0.1.0") {
		t.Fatalf("stdout %q", stdout.String())
	}
}

func TestConfigNeverContainsTokenAfterManualWriteScan(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("BG_CLI_CONFIG_DIR", dir)
	cfg := &config.File{Profiles: map[string]config.Profile{"main": {Alias: "main", ClientID: "cid"}}}
	if err := config.Save(cfg); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(dir, config.ConfigFile))
	if err != nil {
		t.Fatal(err)
	}
	forbidden := []string{"access_token", "client_secret", "ClientSecret", "Bearer "}
	for _, f := range forbidden {
		if strings.Contains(string(raw), f) {
			t.Fatalf("config contains %q", f)
		}
	}
}

type ioDiscard struct{}

func (ioDiscard) Write(p []byte) (int, error) { return len(p), nil }
