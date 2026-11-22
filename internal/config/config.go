// Package config stores non-secret profile metadata on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	AppName        = "bg-cli"
	ConfigFile     = "config.json"
	DefaultProfile = "main"
)

var profileAliasRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Profile is non-secret metadata for one authorized account alias.
// AccountID, PS99UserID, and PS99ClanTag are not part of the OAuth response.
// They are stored on the profile so a later auth login can fill them once
// and a reauthorization does not erase a value already written here.
type Profile struct {
	Alias       string    `json:"alias"`
	ClientID    string    `json:"clientId"`
	Scopes      []string  `json:"scopes"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	RedirectURI string    `json:"redirectUri,omitempty"`
	AccountID   string    `json:"accountId,omitempty"`
	PS99UserID  string    `json:"ps99UserId,omitempty"`
	// PS99ClanTag is the clan tag used by GET /api/clan/{tag} when the account
	// profile and extended profile do not include one.
	PS99ClanTag string `json:"ps99ClanTag,omitempty"`
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
	normalizeAliases(&f)
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

// PreserveProfileIdentity copies non-secret account metadata that OAuth does
// not return. Reauthorization replaces the token fields and keeps accountId,
// ps99UserId, and ps99ClanTag instead of erasing them.
func PreserveProfileIdentity(next, previous Profile) Profile {
	next.AccountID = previous.AccountID
	next.PS99UserID = previous.PS99UserID
	next.PS99ClanTag = previous.PS99ClanTag
	return next
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

// BindAccount resolves alias for authenticated use.
//
// The OS credential store keys the access token by the profile that ran
// auth login. That credential belongs to the OAuth client (clientId), not
// to the Roblox id. A hand-added alias with the same clientId is the same
// API account even when accountId and ps99UserId differ. BindAccount then
// returns the logged-in alias as a later credential candidate and copies
// missing non-secret OAuth fields (scopes, expiry, redirect URI) onto the
// result. It does not read the token and does not write config. Roblox
// identity fields on the requested alias are left as written.
func BindAccount(f *File, alias string) (Profile, []string, bool) {
	if f == nil {
		return Profile{}, nil, false
	}
	normalizeAliases(f)
	p, ok := f.Profiles[alias]
	if !ok {
		return Profile{}, nil, false
	}
	if strings.TrimSpace(p.Alias) == "" {
		p.Alias = alias
	}
	peers := sameClientPeers(f, p)
	p = fillSharedAuth(p, peers)
	aliases := make([]string, 0, 1+len(peers))
	seen := map[string]struct{}{}
	for _, name := range append([]string{p.Alias}, peerAliases(peers)...) {
		if name == "" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		aliases = append(aliases, name)
	}
	if len(aliases) == 0 {
		aliases = []string{alias}
	}
	return p, aliases, true
}

func normalizeAliases(f *File) {
	if f.Profiles == nil {
		f.Profiles = map[string]Profile{}
		return
	}
	for key, p := range f.Profiles {
		if strings.TrimSpace(p.Alias) == "" {
			p.Alias = key
			f.Profiles[key] = p
		}
	}
}

func sameClientPeers(f *File, p Profile) []Profile {
	id := strings.TrimSpace(p.ClientID)
	if id == "" {
		return nil
	}
	var peers []Profile
	for key, other := range f.Profiles {
		if strings.TrimSpace(other.Alias) == "" {
			other.Alias = key
		}
		if other.Alias == p.Alias {
			continue
		}
		if strings.TrimSpace(other.ClientID) != id {
			continue
		}
		peers = append(peers, other)
	}
	sort.Slice(peers, func(i, j int) bool {
		return peerLess(f, peers[i], peers[j])
	})
	return peers
}

func peerAliases(peers []Profile) []string {
	out := make([]string, 0, len(peers))
	for _, p := range peers {
		out = append(out, p.Alias)
	}
	return out
}

func peerLess(f *File, a, b Profile) bool {
	ra, rb := peerRank(f, a.Alias), peerRank(f, b.Alias)
	if ra != rb {
		return ra < rb
	}
	return a.Alias < b.Alias
}

func peerRank(f *File, alias string) int {
	if f != nil && alias != "" && alias == f.DefaultProfile {
		return 0
	}
	if alias == DefaultProfile {
		return 1
	}
	return 2
}

func fillSharedAuth(p Profile, peers []Profile) Profile {
	for _, src := range peers {
		if len(p.Scopes) == 0 && len(src.Scopes) > 0 {
			p.Scopes = append([]string(nil), src.Scopes...)
		}
		if p.RedirectURI == "" {
			p.RedirectURI = src.RedirectURI
		}
		if p.IssuedAt.IsZero() {
			p.IssuedAt = src.IssuedAt
		}
		if p.ExpiresAt.IsZero() {
			p.ExpiresAt = src.ExpiresAt
		}
	}
	return p
}
