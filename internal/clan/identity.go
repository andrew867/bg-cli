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

// BaselineIdentityRejected is the history reason when a prior snapshot is a
// different account. Rates and inventory diffs must not use that file.
const BaselineIdentityRejected = "previous snapshot was rejected because account identity differs"

// HistoryIdentity is the account a snapshot belongs to.
// LinkedAccountID is provider identity, and only when the provider supplied it.
// PS99UserID is owner metadata. It is not proof of which bearer wrote the file.
// CredentialEpoch is the history lineage when LinkedAccountID is absent. It is
// not a token hash. A matching non-empty epoch is enough; the contribution
// user id is not part of it. A different epoch is a new lineage.
// LegacyAccountID is the older summary accountId field. It is not a
// linked-account id and is used only when both sides lack the newer ids.
// Profile is the local alias and is never identity by itself.
// A summary that has neither PS99UserID nor CredentialEpoch may have stored
// the owner in userId. That fallback is applied by NewHistoryIdentity and is
// not used for a summary that has an epoch.
type HistoryIdentity struct {
	LinkedAccountID string
	PS99UserID      string
	CredentialEpoch string
	LegacyAccountID string
	Profile         string
	Conflict        bool
}

// NewHistoryIdentity builds the one account identity used by history,
// session selection, scan keys, and inventory baselines.
// user is the contribution target. It becomes owner metadata only for a
// summary that predates this model: both ps99 and credentialEpoch are empty.
func NewHistoryIdentity(linked, ps99, user, epoch, legacyAccount, profile string) HistoryIdentity {
	id := HistoryIdentity{
		LinkedAccountID: strings.TrimSpace(linked),
		PS99UserID:      strings.TrimSpace(ps99),
		CredentialEpoch: strings.TrimSpace(epoch),
		LegacyAccountID: strings.TrimSpace(legacyAccount),
		Profile:         strings.TrimSpace(profile),
	}
	if id.PS99UserID == "" && id.CredentialEpoch == "" {
		id.PS99UserID = strings.TrimSpace(user)
	}
	return id
}

// SameHistory reports whether two snapshots are the same account.
// The alias is not compared. A legacy accountId matches only when neither
// side has a linked-account id or a PS99 user id.
func SameHistory(a, b HistoryIdentity) bool {
	ok, _ := CompareHistory(a, b)
	return ok
}

// CompareHistory returns false and BaselineIdentityRejected when the accounts
// differ or cannot be confirmed. Prefer an unavailable baseline over a diff
// across accounts.
func CompareHistory(a, b HistoryIdentity) (bool, string) {
	if a.Conflict || b.Conflict {
		return false, BaselineIdentityRejected
	}
	aL, bL := strings.TrimSpace(a.LinkedAccountID), strings.TrimSpace(b.LinkedAccountID)
	aU, bU := strings.TrimSpace(a.PS99UserID), strings.TrimSpace(b.PS99UserID)
	if aL != "" && bL != "" && aL != bL {
		return false, BaselineIdentityRejected
	}
	if aU != "" && bU != "" && aU != bU {
		return false, BaselineIdentityRejected
	}
	if aL != "" && bL != "" && aL == bL {
		return true, ""
	}
	// No provider-verified linked account. A matching epoch is the lineage.
	// Contribution userId is not compared. One empty epoch does not continue
	// an older summary, including one that only shares a user id.
	aE, bE := strings.TrimSpace(a.CredentialEpoch), strings.TrimSpace(b.CredentialEpoch)
	if aE != "" && bE != "" {
		if aE == bE {
			return true, ""
		}
		return false, BaselineIdentityRejected
	}
	if aE != "" || bE != "" {
		return false, BaselineIdentityRejected
	}
	if aL == "" && bL == "" && aU != "" && bU != "" && aU == bU {
		return true, ""
	}
	if aL == "" && bL == "" && aU == "" && bU == "" {
		aA := strings.TrimSpace(a.LegacyAccountID)
		bA := strings.TrimSpace(b.LegacyAccountID)
		if aA != "" && aA == bA {
			return true, ""
		}
		if aA == "" && bA == "" && strings.TrimSpace(a.Profile) == strings.TrimSpace(b.Profile) {
			// No provider id was stored. Keep one alias's own history, including
			// older summaries that recorded no alias. A different alias is not
			// the same account.
			return true, ""
		}
	}
	return false, BaselineIdentityRejected
}

func (p Prior) historyIdentity() HistoryIdentity {
	return NewHistoryIdentity(p.LinkedAccountID, p.PS99UserID, p.UserID, p.CredentialEpoch, p.AccountID, p.Profile)
}

func (in Input) historyIdentity() HistoryIdentity {
	return NewHistoryIdentity(in.LinkedAccountID, in.PS99UserID, in.UserID, in.CredentialEpoch, in.AccountID, in.Profile)
}

// MatchingSession is the latest prior session for this account. A newer
// summary from another account does not replace it. The alias is not identity.
func MatchingSession(priors []Prior, cur HistoryIdentity) string {
	var best Prior
	found := false
	for _, p := range priors {
		if !SameHistory(cur, p.historyIdentity()) {
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
