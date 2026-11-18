package clan

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
)

// sessionIDRE keeps a session id safe to store and to pass on a command line.
var sessionIDRE = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,80}$`)

// ValidateSessionID rejects an empty or unsafe --session-id value.
func ValidateSessionID(id string) error {
	if !sessionIDRE.MatchString(id) {
		return fmt.Errorf("invalid session id %q\n  bg-cli account snapshot --session-id <id>\n  use 1-81 chars of [A-Za-z0-9._-], starting with a letter or digit", id)
	}
	return nil
}

// ExtractAccountID reads a stable account id from the authenticated profile
// document. The profile alias is not an account id. Nested objects are ignored.
func ExtractAccountID(profile json.RawMessage) string {
	if len(profile) == 0 || string(profile) == "null" {
		return ""
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(profile, &top) != nil {
		return ""
	}
	for _, key := range []string{"AccountID", "AccountId", "UserID", "UserId", "RobloxUserID", "RobloxUserId"} {
		raw, ok := top[key]
		if !ok || len(raw) == 0 || string(raw) == "null" {
			continue
		}
		if id := canonicalID(raw); id != "" {
			return id
		}
	}
	return ""
}

func canonicalID(raw json.RawMessage) string {
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var str string
		if json.Unmarshal(raw, &str) == nil {
			s = strings.TrimSpace(str)
		}
	}
	n, ok := ParseExactInt(s)
	if !ok || n.Sign() < 0 {
		return ""
	}
	return n.String()
}

// sameAccount reports whether a prior summary belongs to this snapshot's account.
// A stored account id wins. The profile alias is used only when neither side
// has an account id. Records with a different identity stay in the directory
// and are not baselines.
func sameAccount(accountID, profile, priorAccount, priorProfile string) bool {
	if accountID != "" || priorAccount != "" {
		return accountID != "" && accountID == priorAccount
	}
	return profile == priorProfile
}

// MatchingSession is the latest prior session for this account. A newer
// summary from another account does not replace it.
func MatchingSession(priors []Prior, accountID, profile string) string {
	var best Prior
	found := false
	for _, p := range priors {
		if !sameAccount(accountID, profile, p.AccountID, p.Profile) {
			continue
		}
		if !found || p.ExportedAt.After(best.ExportedAt) || (p.ExportedAt.Equal(best.ExportedAt) && p.File > best.File) {
			best = p
			found = true
		}
	}
	if !found {
		return ""
	}
	return best.SessionID
}
