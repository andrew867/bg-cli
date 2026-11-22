package clan

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// maxSummaryBytes skips a file that is too large to be a summary.
const maxSummaryBytes = 2 << 20

// ScanResult is the prior history read from summary JSON files only.
type ScanResult struct {
	Priors    []Prior
	SessionID string
	Warnings  []string
	Opened    []string
}

// Scan reads prior *-prefix-summary.json files. It does not open inventory,
// profile, clan, or currency CSV payloads. Malformed files are warnings.
func Scan(dir, prefix string, now time.Time, skipName string) (ScanResult, error) {
	var out ScanResult
	ents, err := os.ReadDir(dir)
	if err != nil {
		return out, err
	}
	suffix := "-" + prefix + "-summary.json"
	// Same-second files collapse only when they are the same account and session.
	// A different account at that second must remain so later filtering can see it.
	byKey := map[summaryKey]Prior{}
	for _, ent := range ents {
		name := ent.Name()
		if ent.IsDir() || name == skipName || !strings.HasSuffix(name, suffix) {
			continue
		}
		path := filepath.Join(dir, name)
		info, err := os.Stat(path)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("summary %s: %v", name, err))
			continue
		}
		if info.Size() > maxSummaryBytes {
			out.Warnings = append(out.Warnings, fmt.Sprintf("summary too large, skipped: %s", name))
			continue
		}
		body, err := os.ReadFile(path)
		out.Opened = append(out.Opened, name)
		if err != nil {
			out.Warnings = append(out.Warnings, fmt.Sprintf("malformed summary %s: %v", name, err))
			continue
		}
		prior, warn, ok := decodePrior(name, body, now)
		if warn != "" {
			out.Warnings = append(out.Warnings, warn)
		}
		if !ok {
			continue
		}
		key := summaryKeyFor(prior)
		if prev, exists := byKey[key]; exists {
			if name > prev.File {
				byKey[key] = prior
			}
			continue
		}
		byKey[key] = prior
	}
	out.Priors = make([]Prior, 0, len(byKey))
	for _, p := range byKey {
		out.Priors = append(out.Priors, p)
	}
	sortPriors(out.Priors)
	if len(out.Priors) > 0 {
		out.SessionID = out.Priors[len(out.Priors)-1].SessionID
	}
	return out, nil
}

// summaryKey is the only collision Scan collapses: one second, one account, one session.
type summaryKey struct {
	unix    int64
	ident   string
	session string
}

func summaryKeyFor(p Prior) summaryKey {
	h := p.historyIdentity()
	ident := "alias:" + p.Profile
	switch {
	case h.LinkedAccountID != "":
		ident = "linked:" + h.LinkedAccountID
	case h.CredentialEpoch != "":
		ident = "epoch:" + h.CredentialEpoch
	case h.PS99UserID != "":
		ident = "ps99:" + h.PS99UserID
	case h.LegacyAccountID != "":
		ident = "legacy:" + h.LegacyAccountID
	}
	return summaryKey{unix: p.ExportedAt.Unix(), ident: ident, session: p.SessionID}
}

func decodePrior(name string, body []byte, now time.Time) (Prior, string, bool) {
	var sum Summary
	if err := json.Unmarshal(body, &sum); err != nil {
		return Prior{}, fmt.Sprintf("malformed summary %s: %v", name, err), false
	}
	if !schemaCompatible(sum.SchemaVersion) {
		return Prior{}, fmt.Sprintf("incompatible summary schema %s: %q", name, sum.SchemaVersion), false
	}
	if strings.TrimSpace(sum.SessionID) == "" {
		return Prior{}, fmt.Sprintf("summary %s missing sessionId", name), false
	}
	exported, err := time.Parse(time.RFC3339, sum.ExportedAt)
	if err != nil {
		return Prior{}, fmt.Sprintf("malformed summary %s: exportedAt %q", name, sum.ExportedAt), false
	}
	if exported.After(now) {
		return Prior{}, fmt.Sprintf("future-dated summary ignored: %s", name), false
	}
	if !exported.Before(now) {
		return Prior{}, "", false
	}
	p := Prior{
		File:            name,
		ExportedAt:      exported,
		ExportedAtText:  sum.ExportedAt,
		SessionID:       sum.SessionID,
		AccountID:       strings.TrimSpace(sum.AccountID),
		LinkedAccountID: strings.TrimSpace(sum.LinkedAccountID),
		Profile:         firstNonEmpty(sum.ProfileAlias, sum.Profile),
		UserID:          strings.TrimSpace(sum.UserID),
		PS99UserID:      strings.TrimSpace(sum.PS99UserID),
		CredentialEpoch: strings.TrimSpace(sum.CredentialEpoch),
		ClanTag:         sum.Current.Clan.Tag,
		CurrenciesKnown: sum.Current.CurrenciesKnown,
		Currencies:      map[string]string{},
		StatsKnown:      sum.Current.StatisticsKnown,
		Stats:           map[string]string{},
	}
	if p.StatsKnown {
		for k, v := range sum.Current.Statistics {
			if strings.TrimSpace(v) == "" {
				continue
			}
			p.Stats[k] = strings.TrimSpace(v)
		}
	}
	if p.CurrenciesKnown {
		for id, cur := range sum.Current.Currencies {
			if cur.Amount == "" {
				continue
			}
			p.Currencies[id] = CanonicalAmount(cur.Amount)
		}
	}
	if sum.Current.Inventory != nil && sum.Current.Inventory.Known {
		cp := *sum.Current.Inventory
		p.Inventory = &cp
	}
	if b := sum.Current.Clan.ActiveBattle; b != nil && b.ID != "" {
		p.BattleID = b.ID
		if b.Points != "" {
			p.Points = CanonicalAmount(b.Points)
			p.PointsOK = true
		}
		if place, ok := placeText(b.Place); ok {
			p.Place = place
			p.PlaceOK = true
		}
		if b.UserContribution != "" {
			p.UserPoints = CanonicalAmount(b.UserContribution)
			p.UserPointsOK = true
		}
	}
	return p, "", true
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

func schemaCompatible(v string) bool {
	return v == "1" || strings.HasPrefix(v, "1.")
}

func sortPriors(priors []Prior) {
	for i := 1; i < len(priors); i++ {
		j := i
		for j > 0 {
			a, b := priors[j-1], priors[j]
			if a.ExportedAt.Before(b.ExportedAt) || (a.ExportedAt.Equal(b.ExportedAt) && a.File <= b.File) {
				break
			}
			priors[j-1], priors[j] = priors[j], priors[j-1]
			j--
		}
	}
}
