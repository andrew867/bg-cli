package clan

import (
	"fmt"
	"strings"
	"time"
)

// SchemaVersion is the summary document version future snapshots accept.
const SchemaVersion = "1.0"

const (
	Window5m      = "5m"
	Window30m     = "30m"
	Window1h      = "1h"
	WindowSession = "session"
)

// Summary is the lightweight history record written beside each snapshot.
type Summary struct {
	SchemaVersion   string   `json:"schemaVersion"`
	LocalStamp      string   `json:"localStamp"`
	CollisionSuffix string   `json:"collisionSuffix,omitempty"`
	ExportedAt      string   `json:"exportedAt"`
	SessionID       string   `json:"sessionId"`
	Prefix          string   `json:"prefix,omitempty"`
	Profile         string   `json:"profile,omitempty"`
	AccountID       string   `json:"accountId,omitempty"`
	UserID          string   `json:"userId,omitempty"`
	Current         Current  `json:"current"`
	Windows         Windows  `json:"windows"`
	Warnings        []string `json:"warnings,omitempty"`
}

// Current holds the full currency map and the clan row for this snapshot.
type Current struct {
	CurrenciesKnown bool                      `json:"currenciesKnown"`
	Currencies      map[string]CurrencyAmount `json:"currencies"`
	Clan            ClanCurrent               `json:"clan"`
}

// Windows holds one shared baseline per interval.
type Windows struct {
	Five    Window `json:"5m"`
	Thirty  Window `json:"30m"`
	Hour    Window `json:"1h"`
	Session Window `json:"session"`
}

// Window is the shared baseline for every metric in that interval.
type Window struct {
	RequestedSeconds     *int64                   `json:"requestedSeconds"`
	BaselineAt           string                   `json:"baselineAt,omitempty"`
	BaselineFile         string                   `json:"baselineFile,omitempty"`
	CurrentAt            string                   `json:"currentAt"`
	ElapsedSeconds       *int64                   `json:"elapsedSeconds,omitempty"`
	BaselineErrorSeconds *int64                   `json:"baselineErrorSeconds,omitempty"`
	Valid                bool                     `json:"valid"`
	Reason               string                   `json:"reason,omitempty"`
	CurrenciesStatus     string                   `json:"currenciesStatus,omitempty"`
	CurrenciesReason     string                   `json:"currenciesReason,omitempty"`
	ChangedCurrencies    map[string]CurrencyDelta `json:"changedCurrencies,omitempty"`
	Clan                 *ClanWindow              `json:"clan,omitempty"`
}

// CurrencyDelta is one currency whose balance changed, or an unavailable comparison.
type CurrencyDelta struct {
	Status           string `json:"status,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Baseline         string `json:"baseline,omitempty"`
	Current          string `json:"current,omitempty"`
	Delta            string `json:"delta,omitempty"`
	NetPerHour       string `json:"netPerHour,omitempty"`
	NetPerHourApprox string `json:"netPerHourApprox,omitempty"`
}

// ClanWindow is the clan-point and contribution delta for the shared baseline.
type ClanWindow struct {
	BattleID         string       `json:"battleId,omitempty"`
	BaselineBattleID string       `json:"baselineBattleId,omitempty"`
	Status           string       `json:"status,omitempty"`
	Reason           string       `json:"reason,omitempty"`
	Points           *MetricDelta `json:"points,omitempty"`
	UserContribution *MetricDelta `json:"userContribution,omitempty"`
}

// MetricDelta is an exact signed change and net per hour.
type MetricDelta struct {
	Status           string `json:"status,omitempty"`
	Reason           string `json:"reason,omitempty"`
	Baseline         string `json:"baseline,omitempty"`
	Current          string `json:"current,omitempty"`
	Delta            string `json:"delta,omitempty"`
	NetPerHour       string `json:"netPerHour,omitempty"`
	NetPerHourApprox string `json:"netPerHourApprox,omitempty"`
}

// Prior is one earlier summary used as a baseline candidate.
type Prior struct {
	File            string
	ExportedAt      time.Time
	ExportedAtText  string
	SessionID       string
	AccountID       string
	Profile         string
	UserID          string
	ClanTag         string
	BattleID        string
	Points          string
	PointsOK        bool
	UserPoints      string
	UserPointsOK    bool
	CurrenciesKnown bool
	Currencies      map[string]string
}

// Input is the current snapshot plus priors already scanned from summary files.
type Input struct {
	Now             time.Time
	LocalStamp      string
	CollisionSuffix string
	Prefix          string
	Profile         string
	AccountID       string
	UserID          string
	SessionID       string
	Currencies      map[string]CurrencyAmount
	CurrenciesKnown bool
	Clan            ClanCurrent
	Priors          []Prior
	Warnings        []string
}

// NewSessionID starts a session identity that later snapshots copy forward.
func NewSessionID(prefix, stamp string) string {
	if prefix == "" {
		prefix = "ps99"
	}
	return prefix + "-" + stamp
}

// ToleranceSeconds is the largest |elapsed-requested| that still counts as
// that window. It is one quarter of the requested duration, and at least 60s.
func ToleranceSeconds(requested int64) int64 {
	tol := requested / 4
	if tol < 60 {
		return 60
	}
	return tol
}

// Build assembles the summary. Every finite window and the session window
// share one baseline record. Rates use actual elapsed seconds.
func Build(in Input) Summary {
	if in.Currencies == nil {
		in.Currencies = map[string]CurrencyAmount{}
	}
	session := in.SessionID
	if session == "" {
		session = NewSessionID(in.Prefix, in.LocalStamp)
	}
	user := strings.TrimSpace(in.UserID)
	nowText := in.Now.Format(time.RFC3339)
	sum := Summary{
		SchemaVersion:   SchemaVersion,
		LocalStamp:      in.LocalStamp,
		CollisionSuffix: in.CollisionSuffix,
		ExportedAt:      nowText,
		SessionID:       session,
		Prefix:          in.Prefix,
		Profile:         in.Profile,
		AccountID:       strings.TrimSpace(in.AccountID),
		UserID:          user,
		Current: Current{
			CurrenciesKnown: in.CurrenciesKnown,
			Currencies:      in.Currencies,
			Clan:            in.Clan,
		},
		Warnings: append([]string(nil), in.Warnings...),
	}
	five := int64(5 * 60)
	thirty := int64(30 * 60)
	hour := int64(60 * 60)
	sum.Windows.Five = buildWindow(in, session, nowText, &five)
	sum.Windows.Thirty = buildWindow(in, session, nowText, &thirty)
	sum.Windows.Hour = buildWindow(in, session, nowText, &hour)
	sum.Windows.Session = buildWindow(in, session, nowText, nil)
	return sum
}

func buildWindow(in Input, session, nowText string, requested *int64) Window {
	w := Window{CurrentAt: nowText, RequestedSeconds: requested}
	base, ok := selectBaseline(in.Priors, in.Now, session, in.AccountID, in.Profile, requested)
	if !ok {
		w.Reason = "no earlier snapshot in this session"
		return w
	}
	elapsed := in.Now.Unix() - base.ExportedAt.Unix()
	w.BaselineAt = base.ExportedAtText
	if w.BaselineAt == "" {
		w.BaselineAt = base.ExportedAt.Format(time.RFC3339)
	}
	w.BaselineFile = base.File
	w.ElapsedSeconds = &elapsed
	if requested == nil {
		w.Valid = elapsed > 0
		if !w.Valid {
			w.Reason = "elapsed time is not positive"
			return w
		}
	} else {
		errSec := elapsed - *requested
		w.BaselineErrorSeconds = &errSec
		tol := ToleranceSeconds(*requested)
		absErr := errSec
		if absErr < 0 {
			absErr = -absErr
		}
		if absErr > tol {
			w.Reason = fmt.Sprintf("no baseline within tolerance of the requested window: closest is %ds from current, requested %ds, tolerance %ds", elapsed, *requested, tol)
			return w
		}
		w.Valid = true
	}
	switch {
	case in.CurrenciesKnown && base.CurrenciesKnown:
		w.ChangedCurrencies = diffCurrencies(base.Currencies, in.Currencies, elapsed)
	case !in.CurrenciesKnown && !base.CurrenciesKnown:
		w.CurrenciesStatus = "unavailable"
		w.CurrenciesReason = "currency list is unknown on the baseline and the current snapshot"
	case !base.CurrenciesKnown:
		w.CurrenciesStatus = "unavailable"
		w.CurrenciesReason = "baseline currency list is unknown"
	default:
		w.CurrenciesStatus = "unavailable"
		w.CurrenciesReason = "current currency list is unknown"
	}
	w.Clan = diffClan(base, in.Clan, elapsed, strings.TrimSpace(in.UserID))
	return w
}

func selectBaseline(priors []Prior, now time.Time, session, accountID, profile string, requested *int64) (Prior, bool) {
	var best Prior
	found := false
	for _, p := range priors {
		if p.SessionID != session || !sameAccount(accountID, profile, p.AccountID, p.Profile) || !p.ExportedAt.Before(now) {
			continue
		}
		age := now.Unix() - p.ExportedAt.Unix()
		if age <= 0 {
			continue
		}
		if !found {
			best = p
			found = true
			continue
		}
		if requested == nil {
			if p.ExportedAt.Before(best.ExportedAt) || (p.ExportedAt.Equal(best.ExportedAt) && p.File > best.File) {
				best = p
			}
			continue
		}
		if closerBaseline(p, best, *requested, now) {
			best = p
		}
	}
	return best, found
}

func closerBaseline(p, best Prior, requested int64, now time.Time) bool {
	ageP := now.Unix() - p.ExportedAt.Unix()
	ageB := now.Unix() - best.ExportedAt.Unix()
	errP := abs64(ageP - requested)
	errB := abs64(ageB - requested)
	if errP != errB {
		return errP < errB
	}
	if (ageP >= requested) != (ageB >= requested) {
		return ageP >= requested
	}
	return p.File > best.File
}

func abs64(n int64) int64 {
	if n < 0 {
		return -n
	}
	return n
}

func diffCurrencies(base map[string]string, current map[string]CurrencyAmount, elapsed int64) map[string]CurrencyDelta {
	seen := map[string]struct{}{}
	for id := range base {
		seen[id] = struct{}{}
	}
	for id := range current {
		seen[id] = struct{}{}
	}
	out := map[string]CurrencyDelta{}
	for id := range seen {
		bv, bok := base[id]
		cur, cok := current[id]
		cv, cvOK := "", false
		if cok {
			cv = cur.Amount
			cvOK = cv != ""
		}
		if bok && cvOK && SameAmount(bv, cv) {
			continue
		}
		delta := CurrencyDelta{}
		if bok {
			delta.Baseline = bv
		}
		if cvOK {
			delta.Current = cv
		}
		switch {
		case bok && cvOK:
			d, ok := Sub(cv, bv)
			if !ok {
				delta.Status = "unavailable"
				delta.Reason = "amount is not an exact integer"
				break
			}
			delta.Delta = d
			if !setCurrencyRate(&delta, d, elapsed) {
				delta.Status = "unavailable"
				delta.Reason = "elapsed time is not positive"
			}
		case cvOK && !bok:
			delta.Status = "unavailable"
			delta.Reason = "newly observed"
		case bok && !cvOK:
			delta.Status = "unavailable"
			delta.Reason = "no longer returned"
		default:
			continue
		}
		out[id] = delta
	}
	return out
}

func diffClan(base Prior, clan ClanCurrent, elapsed int64, userID string) *ClanWindow {
	curID, curPoints, curPointsOK, curUser, curUserOK := currentBattle(clan)
	out := &ClanWindow{BattleID: curID, BaselineBattleID: base.BattleID}
	if curID == "" || base.BattleID == "" || curID != base.BattleID {
		out.Status = "unavailable"
		switch {
		case curID == "" && base.BattleID == "":
			out.Reason = "no battle is active"
		case curID == "":
			out.Reason = "no battle is active"
		case base.BattleID == "":
			out.Reason = "baseline has no active battle"
		default:
			out.Reason = "active battle id differs from the baseline"
		}
		return out
	}
	if ok, reason := clanTagsCompatible(base.ClanTag, clan.Tag); !ok {
		out.Status = "unavailable"
		out.Reason = reason
		return out
	}
	out.Points = metric(base.Points, base.PointsOK, curPoints, curPointsOK, elapsed, "clan battle points")
	out.UserContribution = userContribution(base, curUser, curUserOK, elapsed, userID)
	return out
}

func clanTagsCompatible(base, current string) (bool, string) {
	if base == current {
		return true, ""
	}
	if base == "" || current == "" {
		return false, "clan tag is absent on one side of the comparison"
	}
	return false, "clan tag differs from the baseline"
}

func userContribution(base Prior, cur string, curOK bool, elapsed int64, userID string) *MetricDelta {
	switch {
	case userID == "" && base.UserID == "":
		return metric(base.UserPoints, base.UserPointsOK, cur, curOK, elapsed, "player contribution")
	case userID == "":
		d := &MetricDelta{Status: "unavailable", Reason: "player user id is not configured"}
		return d
	case base.UserID == "":
		d := &MetricDelta{Status: "unavailable", Reason: "baseline player user id is absent"}
		return d
	case userID != base.UserID:
		d := &MetricDelta{Status: "unavailable", Reason: "player user id differs from the baseline"}
		return d
	default:
		return metric(base.UserPoints, base.UserPointsOK, cur, curOK, elapsed, "player contribution")
	}
}

func currentBattle(clan ClanCurrent) (id, points string, pointsOK bool, user string, userOK bool) {
	if clan.ActiveBattle == nil {
		return "", "", false, "", false
	}
	b := clan.ActiveBattle
	return b.ID, b.Points, b.Points != "", b.UserContribution, b.UserContribution != ""
}

func metric(base string, baseOK bool, cur string, curOK bool, elapsed int64, name string) *MetricDelta {
	d := &MetricDelta{}
	if baseOK {
		d.Baseline = base
	}
	if curOK {
		d.Current = cur
	}
	if !baseOK || !curOK {
		d.Status = "unavailable"
		switch {
		case !baseOK && !curOK:
			d.Reason = name + " are absent"
		case !baseOK:
			d.Reason = "absent prior point"
		default:
			d.Reason = "absent current point"
		}
		return d
	}
	delta, ok := Sub(cur, base)
	if !ok {
		d.Status = "unavailable"
		d.Reason = "amount is not an exact integer"
		return d
	}
	d.Delta = delta
	if !setMetricRate(d, delta, elapsed) {
		d.Status = "unavailable"
		d.Reason = "elapsed time is not positive"
		return d
	}
	return d
}

func setCurrencyRate(d *CurrencyDelta, delta string, elapsed int64) bool {
	exact, approx, ok := NetPerHourParts(delta, elapsed)
	if !ok {
		return false
	}
	d.NetPerHour = exact
	if approx != exact {
		d.NetPerHourApprox = approx
	}
	return true
}

func setMetricRate(d *MetricDelta, delta string, elapsed int64) bool {
	exact, approx, ok := NetPerHourParts(delta, elapsed)
	if !ok {
		return false
	}
	d.NetPerHour = exact
	if approx != exact {
		d.NetPerHourApprox = approx
	}
	return true
}
