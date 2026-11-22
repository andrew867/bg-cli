// Package config stores non-secret profile metadata on disk.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

const (
	AppName        = "bg-cli"
	ConfigFile     = "config.json"
	DefaultProfile = "main"
)

var profileAliasRE = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_-]{0,63}$`)

// Profile is non-secret metadata for one local alias.
//
// These are different things and are not inferred from each other:
// the alias, the OAuth credential, the developer clientId, the BIG Games
// linked-account id, and the Roblox/PS99 user id. clientId is not an
// account. accountId is legacy config and is not linkedAccountId.
// AccountID, PS99UserID, PS99ClanTag, LinkedAccountID, Username, and
// DisplayName are not part of the OAuth token response. A later login
// keeps values already written here.
type Profile struct {
	Alias       string    `json:"alias"`
	ClientID    string    `json:"clientId"`
	Scopes      []string  `json:"scopes"`
	IssuedAt    time.Time `json:"issuedAt"`
	ExpiresAt   time.Time `json:"expiresAt"`
	RedirectURI string    `json:"redirectUri,omitempty"`
	// AccountID is a legacy label. It is not the BIG Games linked-account id.
	AccountID  string `json:"accountId,omitempty"`
	PS99UserID string `json:"ps99UserId,omitempty"`
	// LinkedAccountID is the BIG Games linked-account id when a provider
	// response positively includes one. It is never copied from accountId.
	LinkedAccountID string `json:"linkedAccountId,omitempty"`
	Username        string `json:"username,omitempty"`
	DisplayName     string `json:"displayName,omitempty"`
	// CredentialEpoch identifies one installed OAuth grant. It is not a
	// secret and it is not derived from the access token. A later snapshot
	// uses it when the provider did not verify the linked account.
	CredentialEpoch string `json:"credentialEpoch,omitempty"`
	// PS99ClanTag is the clan tag used by GET /api/clan/{tag} when the account
	// profile and extended profile do not include one. It stays on this alias.
	PS99ClanTag string `json:"ps99ClanTag,omitempty"`
}

// OAuthApp is the developer application shared by local aliases.
// ClientID and RedirectURI are not secrets and they are not a player.
type OAuthApp struct {
	ClientID    string `json:"clientId,omitempty"`
	RedirectURI string `json:"redirectUri,omitempty"`
}

// File is the on-disk configuration document. It must never contain secrets.
// An old credentialProfile value is ignored. The next save omits it.
type File struct {
	DefaultProfile string             `json:"defaultProfile"`
	OAuth          OAuthApp           `json:"oauth,omitempty"`
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
// not return. Reauthorization replaces the token fields and keeps the
// existing identity. It does not delete a linked account or point the alias
// at a different one.
func PreserveProfileIdentity(next, previous Profile) Profile {
	next.AccountID = previous.AccountID
	next.PS99UserID = previous.PS99UserID
	next.PS99ClanTag = previous.PS99ClanTag
	next.LinkedAccountID = previous.LinkedAccountID
	next.Username = previous.Username
	next.DisplayName = previous.DisplayName
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

// BindAccount loads the requested alias and nothing else.
//
// The returned alias list contains only that profile. A same-client peer is
// not a credential. Scopes, expiry, and redirect URI are not copied from
// another profile. The access token, when one exists, is the keyring entry
// for this alias. Login resolves the developer application separately.
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
	return p, []string{p.Alias}, true
}

// ResolveDeveloperApp fills the non-secret client id and redirect URI for a
// login. Explicit flags win, then the named profile, then the shared oauth
// app, then the default profile. It does not return an access token.
func ResolveDeveloperApp(cfg *File, profile, clientID, redirectURI string) (string, string) {
	clientID = strings.TrimSpace(clientID)
	redirectURI = strings.TrimSpace(redirectURI)
	if cfg == nil {
		return clientID, redirectURI
	}
	fill := func(id, uri string) {
		if clientID == "" {
			clientID = strings.TrimSpace(id)
		}
		if redirectURI == "" {
			redirectURI = strings.TrimSpace(uri)
		}
	}
	if p, ok := cfg.Profiles[profile]; ok {
		fill(p.ClientID, p.RedirectURI)
	}
	fill(cfg.OAuth.ClientID, cfg.OAuth.RedirectURI)
	def := strings.TrimSpace(cfg.DefaultProfile)
	if def != "" && def != profile {
		if p, ok := cfg.Profiles[def]; ok {
			fill(p.ClientID, p.RedirectURI)
		}
	}
	return clientID, redirectURI
}

// RememberDeveloperApp stores the application used by a successful login.
// The access token stays on the profile that received the grant.
func RememberDeveloperApp(cfg *File, clientID, redirectURI string) {
	if cfg == nil {
		return
	}
	if id := strings.TrimSpace(clientID); id != "" {
		cfg.OAuth.ClientID = id
	}
	if uri := strings.TrimSpace(redirectURI); uri != "" {
		cfg.OAuth.RedirectURI = uri
	}
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
