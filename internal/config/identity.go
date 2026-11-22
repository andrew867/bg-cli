package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"math/big"
	"strings"
	"unicode"
)

// GrantIdentity is what an OAuth-accessible account document positively says.
// Empty fields mean the provider did not identify that value. AccountID from
// a profile body is not copied into LinkedAccountID or PS99UserID.
type GrantIdentity struct {
	LinkedAccountID string
	PS99UserID      string
	Username        string
	DisplayName     string
	Warning         string
}

// DiscoverProfile reads identity from an account profile document.
// The official /v1/account/profile whitelist does not include a Roblox user
// id or a BIG Games linked-account id. When those keys are absent the result
// is empty and Warning may explain an ignored legacy field. Nested objects
// are ignored. This does not guess from list order or from a currency balance.
func DiscoverProfile(body json.RawMessage) GrantIdentity {
	if len(body) == 0 || string(body) == "null" {
		return GrantIdentity{}
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(body, &top) != nil {
		return GrantIdentity{}
	}
	id := GrantIdentity{
		LinkedAccountID: rawString(top, "linkedAccountId", "LinkedAccountId", "LinkedAccountID"),
		PS99UserID:      positiveID(top, "UserID", "UserId", "RobloxUserID", "RobloxUserId"),
		Username:        rawString(top, "Username", "username"),
		DisplayName:     rawString(top, "DisplayName", "displayName"),
	}
	if legacy := rawString(top, "AccountID", "AccountId", "accountId"); legacy != "" {
		id.Warning = "profile AccountID is not used as linkedAccountId or ps99UserId"
	}
	return id
}

// IdentityError is a hard failure: the account document and the local profile
// name different accounts. The message does not include ids or tokens.
type IdentityError struct {
	msg string
}

func (e *IdentityError) Error() string {
	if e == nil {
		return "account identity does not match"
	}
	return e.msg
}

// UpstreamConflict reports a hard failure when the account document identifies
// a different linked account or PS99 user than the local profile. A document
// that does not identify the player is not a disagreement.
func UpstreamConflict(local Profile, body json.RawMessage) error {
	found := DiscoverProfile(body)
	linked := strings.TrimSpace(local.LinkedAccountID)
	ps99 := strings.TrimSpace(local.PS99UserID)
	alias := local.Alias
	if alias == "" {
		alias = "the requested profile"
	}
	if found.LinkedAccountID != "" && linked != "" && found.LinkedAccountID != linked {
		return &IdentityError{msg: fmt.Sprintf("upstream linked account does not match profile %q; account data was not saved", alias)}
	}
	if found.PS99UserID != "" && ps99 != "" && found.PS99UserID != ps99 {
		return &IdentityError{msg: fmt.Sprintf("upstream PS99 user does not match profile %q; account data was not saved", alias)}
	}
	return nil
}

// LegacyIdentityWarnings describes local fields that are not a linked-account id.
func LegacyIdentityWarnings(p Profile) []string {
	var out []string
	accountID := strings.TrimSpace(p.AccountID)
	linked := strings.TrimSpace(p.LinkedAccountID)
	ps99 := strings.TrimSpace(p.PS99UserID)
	if accountID != "" && linked == "" {
		out = append(out, "accountId is a legacy field and is not the BIG Games linked-account id")
	}
	if accountID != "" && ps99 != "" && accountID != ps99 {
		out = append(out, "accountId and ps99UserId differ; accountId is not used as linkedAccountId")
	}
	return out
}

// ReconcileGrant chooses which existing alias receives a new OAuth grant.
// It does not rename an alias, delete a missing account, or copy accountId
// into linkedAccountId. A conflicting alias is left unchanged and the token
// must not be stored. An empty discovery returns the requested alias.
func ReconcileGrant(cfg *File, requested string, id GrantIdentity) (alias, notice string, err error) {
	requested = strings.TrimSpace(requested)
	if cfg == nil {
		return "", "", fmt.Errorf("config is missing")
	}
	if cfg.Profiles == nil {
		cfg.Profiles = map[string]Profile{}
	}
	linked := strings.TrimSpace(id.LinkedAccountID)
	ps99 := strings.TrimSpace(id.PS99UserID)
	req, reqOK := cfg.Profiles[requested]
	if reqOK && profileConflictsGrant(req, linked, ps99) {
		return "", "", fmt.Errorf("this grant is a different Roblox account than profile %q; that alias was not changed and the token was not stored", requested)
	}
	if linked == "" && ps99 == "" {
		return requested, "the account profile did not include a linked-account id or a PS99 user id; account selection is trusted to the consent screen and the token stays on the profile you named", nil
	}
	matches := matchingProfiles(cfg, linked, ps99)
	if len(matches) > 1 {
		return "", "", fmt.Errorf("this grant matches more than one local profile; the token was not stored")
	}
	if len(matches) == 1 && matches[0] != requested {
		if reqOK && !profileIdentityEmpty(req) {
			return "", "", fmt.Errorf("this grant matches profile %q and profile %q already has an identity; the token was not stored", matches[0], requested)
		}
		return matches[0], fmt.Sprintf("this grant matches profile %s; that alias was kept and was not renamed", matches[0]), nil
	}
	return requested, "", nil
}

// ApplyDiscovered fills empty identity fields from a positive discovery and
// updates username and display name. It does not change the alias and does
// not replace an id that already points at a different account.
func ApplyDiscovered(p Profile, id GrantIdentity) (Profile, error) {
	if profileConflictsGrant(p, id.LinkedAccountID, id.PS99UserID) {
		return p, fmt.Errorf("discovered account does not match profile %q", p.Alias)
	}
	if strings.TrimSpace(p.LinkedAccountID) == "" && strings.TrimSpace(id.LinkedAccountID) != "" {
		p.LinkedAccountID = strings.TrimSpace(id.LinkedAccountID)
	}
	if strings.TrimSpace(p.PS99UserID) == "" && strings.TrimSpace(id.PS99UserID) != "" {
		p.PS99UserID = strings.TrimSpace(id.PS99UserID)
	}
	if strings.TrimSpace(id.Username) != "" {
		p.Username = strings.TrimSpace(id.Username)
	}
	if strings.TrimSpace(id.DisplayName) != "" {
		p.DisplayName = strings.TrimSpace(id.DisplayName)
	}
	return p, nil
}

func matchingProfiles(cfg *File, linked, ps99 string) []string {
	var names []string
	for key, p := range cfg.Profiles {
		alias := strings.TrimSpace(p.Alias)
		if alias == "" {
			alias = key
		}
		if profileMatchesGrant(p, linked, ps99) {
			names = append(names, alias)
		}
	}
	return names
}

func profileIdentityEmpty(p Profile) bool {
	return strings.TrimSpace(p.LinkedAccountID) == "" && strings.TrimSpace(p.PS99UserID) == ""
}

func profileMatchesGrant(p Profile, linked, ps99 string) bool {
	if profileConflictsGrant(p, linked, ps99) {
		return false
	}
	pl := strings.TrimSpace(p.LinkedAccountID)
	pu := strings.TrimSpace(p.PS99UserID)
	linked = strings.TrimSpace(linked)
	ps99 = strings.TrimSpace(ps99)
	if linked != "" && pl == linked {
		return ps99 == "" || pu == "" || ps99 == pu
	}
	if linked == "" && ps99 != "" && pu == ps99 && pl == "" {
		return true
	}
	if linked != "" && pl == "" && ps99 != "" && pu == ps99 {
		return true
	}
	return false
}

func profileConflictsGrant(p Profile, linked, ps99 string) bool {
	pl := strings.TrimSpace(p.LinkedAccountID)
	pu := strings.TrimSpace(p.PS99UserID)
	linked = strings.TrimSpace(linked)
	ps99 = strings.TrimSpace(ps99)
	if linked != "" && pl != "" && linked != pl {
		return true
	}
	if ps99 != "" && pu != "" && ps99 != pu {
		return true
	}
	return false
}

// rawString returns a JSON string. Objects, arrays, booleans, numbers, null,
// and a string whose value is itself JSON are not identity.
func rawString(top map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := top[key]
		if !ok {
			continue
		}
		s, ok := jsonStringValue(raw)
		if !ok {
			if presentJSON(raw) {
				return ""
			}
			continue
		}
		return s
	}
	return ""
}

func positiveID(top map[string]json.RawMessage, keys ...string) string {
	for _, key := range keys {
		raw, ok := top[key]
		if !ok {
			continue
		}
		if !presentJSON(raw) {
			continue
		}
		id, ok := positiveJSONID(raw)
		if !ok {
			return ""
		}
		return id
	}
	return ""
}

func presentJSON(raw json.RawMessage) bool {
	raw = bytes.TrimSpace(raw)
	return len(raw) > 0 && string(raw) != "null"
}

func jsonStringValue(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '"' {
		return "", false
	}
	var s string
	if json.Unmarshal(raw, &s) != nil {
		return "", false
	}
	s = strings.TrimSpace(s)
	if s == "" || nestedJSONText(s) {
		return "", false
	}
	return s, true
}

func nestedJSONText(s string) bool {
	t := strings.TrimSpace(s)
	if t == "" {
		return false
	}
	switch t[0] {
	case '{', '[':
	default:
		return false
	}
	var v any
	return json.Unmarshal([]byte(t), &v) == nil
}

func positiveJSONID(raw json.RawMessage) (string, bool) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || raw[0] == '{' || raw[0] == '[' || raw[0] == 't' || raw[0] == 'f' {
		return "", false
	}
	text := string(raw)
	if raw[0] == '"' {
		s, ok := jsonStringValue(raw)
		if !ok {
			return "", false
		}
		text = s
	}
	text = strings.TrimSpace(text)
	if text == "" || text[0] < '1' || text[0] > '9' {
		return "", false
	}
	for _, r := range text[1:] {
		if !unicode.IsDigit(r) {
			return "", false
		}
	}
	n, ok := new(big.Int).SetString(text, 10)
	if !ok || n.Sign() <= 0 {
		return "", false
	}
	return n.String(), true
}
