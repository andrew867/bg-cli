// Package config stores non-secret profile metadata on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"time"
)

const (
	AppName        = "bg-cli"
	ConfigFile     = "config.json"
	DefaultProfile = "main"
)

var profileAliasRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Profile is non-secret metadata for one authorized account alias.
type Profile struct {
	Alias       string    `json:"alias"`
	ClientID    string    `json:"clientId"`
	Scopes      []string  `json:"scopes"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	AccountID   string    `json:"accountId,omitempty"`
	RedirectURI string    `json:"redirectUri,omitempty"`
}

// File is the on-disk configuration document. It must never contain secrets.
type File struct {
	DefaultProfile string             `json:"defaultProfile"`
	Profiles       map[string]Profile `json:"profiles"`
}

// Dir returns the configuration directory.
func Dir() (string, error) {
	if override := os.Getenv("BG_CLI_CONFIG_DIR"); override != "" {
		return override, nil
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, AppName), nil
}

// Path returns the config file path.
func Path() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, ConfigFile), nil
}

// ValidateAlias ensures a profile alias cannot escape keyring namespaces.
func ValidateAlias(alias string) error {
	if !profileAliasRE.MatchString(alias) {
		return fmt.Errorf("invalid profile alias %q: use 1-64 chars of [A-Za-z0-9_-], starting alphanumeric", alias)
	}
	return nil
}

// Load reads the config file, returning an empty file if missing.
func Load() (*File, error) {
	path, err := Path()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &File{Profiles: map[string]Profile{}}, nil
		}
		return nil, err
	}
	var f File
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
	}
	return &f, nil
}

// Save writes the config file atomically.
func Save(f *File) error {
	dir, err := Dir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	path := filepath.Join(dir, ConfigFile)
	tmp := path + ".tmp"
	data, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// UpsertProfile stores or replaces a profile and optionally sets default.
func UpsertProfile(f *File, p Profile, makeDefault bool) error {
	if err := ValidateAlias(p.Alias); err != nil {
		return err
	}
	f.Profiles[p.Alias] = p
	if makeDefault || f.DefaultProfile == "" {
		f.DefaultProfile = p.Alias
	}
	return nil
}

// DeleteProfile removes a profile alias.
func DeleteProfile(f *File, alias string) {
	delete(f.Profiles, alias)
	if f.DefaultProfile == alias {
		f.DefaultProfile = ""
		for k := range f.Profiles {
			f.DefaultProfile = k
			break
		}
	}
}

// Get returns a profile by alias.
func Get(f *File, alias string) (Profile, bool) {
	p, ok := f.Profiles[alias]
	return p, ok
}
