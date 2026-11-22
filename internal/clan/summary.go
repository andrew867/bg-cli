package clan

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// SchemaVersion is the summary document version future snapshots accept.
const SchemaVersion = "1.0"

const (
	Window5m          = "5m"
	Window30m         = "30m"
	Window1h          = "1h"
	WindowSession     = "session"
	WindowPrevious    = "previous"
	CurrencyRateBasis = "observation_interval"
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
	ProfileAlias    string   `json:"profileAlias,omitempty"`
	AccountID       string   `json:"accountId,omitempty"`
	LinkedAccountID string   `json:"linkedAccountId,omitempty"`
	UserID          string   `json:"userId,omitempty"`
	PS99UserID      string   `json:"ps99UserId,omitempty"`
	Current         Current  `json:"current"`
	Windows         Windows  `json:"windows"`
	Warnings        []string `json:"warnings,omitempty"`
}

// Current holds the full currency map and the clan row for this snapshot.
type Current struct {
	CurrenciesKnown bool                      `json:"currenciesKnown"`
	Currencies      map[string]CurrencyAmount `json:"currencies"`
	StatisticsKnown bool                      `json:"statisticsKnown,omitempty"`
	Statistics      map[string]string         `json:"statistics,omitempty"`
	Clan            ClanCurrent               `json:"clan"`
	Inventory       *InventoryState           `json:"inventory,omitempty"`
}

// Windows holds one shared baseline per interval.
type Windows struct {
	Five     Window `json:"5m"`
	Thirty   Window `json:"30m"`
	Hour     Window `json:"1h"`
	Session  Window `json:"session"`
	Previous Window `json:"previous"`
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
	Eggs                 *CurrencyDelta           `json:"eggs,omitempty"`
	Clan                 *ClanWindow              `json:"clan,omitempty"`
	Inventory            *InventoryWindow         `json:"inventory,omitempty"`
}

// InventoryState is the current pet and inventory totals stored on a summary.
// Older summaries omit it. Quantities are exact integer text. Huge is omitted
// unless HugeKnown is true; a missing huge classification is not stored as zero.
type InventoryState struct {
	Known         bool   `json:"known"`
	PetQuantity   string `json:"petQuantity,omitempty"`
	PetRows       string `json:"petRows,omitempty"`
	Golden        string `json:"golden,omitempty"`
	Rainbow       string `json:"rainbow,omitempty"`
	Shiny         string `json:"shiny,omitempty"`
	Huge          string `json:"huge,omitempty"`
	HugeKnown     bool   `json:"hugeKnown,omitempty"`
	InventoryRows string `json:"inventoryRows,omitempty"`
	ObservedAt    string `json:"observedAt,omitempty"`
}

// InventoryWindow is the aggregate pet-count delta for one summary window.
// It has no production rate. A decrease is a real negative delta.
type InventoryWindow struct {
	PetQuantity   *InventoryCount `json:"petQuantity,omitempty"`
	PetRows       *InventoryCount `json:"petRows,omitempty"`
	Golden        *InventoryCount `json:"golden,omitempty"`
	Rainbow       *InventoryCount `json:"rainbow,omitempty"`
	Shiny         *InventoryCount `json:"shiny,omitempty"`
	Huge          *InventoryCount `json:"huge,omitempty"`
	InventoryRows *InventoryCount `json:"inventoryRows,omitempty"`
}

// InventoryCount is an exact aggregate comparison without a rate.
type InventoryCount struct {
	Status   string `json:"status,omitempty"`
	Reason   string `json:"reason,omitempty"`
	Baseline string `json:"baseline,omitempty"`
	Current  string `json:"current,omitempty"`
	Delta    string `json:"delta,omitempty"`
}

// CurrencyDelta is one currency whose balance changed, or an unavailable comparison.
// A numeric rate is an observation estimate: profile currency comes from a
// cached player snapshot, not a continuous production counter.
type CurrencyDelta struct {
	Status             string `json:"status,omitempty"`
	Reason             string `json:"reason,omitempty"`
	Baseline           string `json:"baseline,omitempty"`
	Current            string `json:"current,omitempty"`
	Delta              string `json:"delta,omitempty"`
	NetPerHour         string `json:"netPerHour,omitempty"`
	NetPerHourApprox   string `json:"netPerHourApprox,omitempty"`
	NetPerMinute       string `json:"netPerMinute,omitempty"`
	NetPerMinuteApprox string `json:"netPerMinuteApprox,omitempty"`
	RateBasis          string `json:"rateBasis,omitempty"`
	Estimated          bool   `json:"estimated,omitempty"`
	// RateStatus is set only when a production rate is withheld.
	// The raw delta stays populated. "reset" means this snapshot is the new
	// rate baseline. "unavailable" means the selected window crosses a break.
	RateStatus  string `json:"rateStatus,omitempty"`
	RateReason  string `json:"rateReason,omitempty"`
	RateResetAt string `json:"rateResetAt,omitempty"`
}

// ClanWindow is the clan-point, place, and contribution delta for the shared baseline.
type ClanWindow struct {
	BattleID         string       `json:"battleId,omitempty"`
	BaselineBattleID string       `json:"baselineBattleId,omitempty"`
	Status           string       `json:"status,omitempty"`
	Reason           string       `json:"reason,omitempty"`
	Points           *MetricDelta `json:"points,omitempty"`
	Place            *MetricDelta `json:"place,omitempty"`
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
	LinkedAccountID string
	Profile         string
	UserID          string
	PS99UserID      string
	ClanTag         string
	BattleID        string
	Points          string
	PointsOK        bool
	Place           string
	PlaceOK         bool
	UserPoints      string
	UserPointsOK    bool
	CurrenciesKnown bool
	Currencies      map[string]string
	StatsKnown      bool
	Stats           map[string]string
	Inventory       *InventoryState
}

// Input is the current snapshot plus priors already scanned from summary files.
type Input struct {
	Now             time.Time
	LocalStamp      string
	CollisionSuffix string
	Prefix          string
	Profile         string
	AccountID       string
	LinkedAccountID string
	UserID          string
	PS99UserID      string
	SessionID       string
	Currencies      map[string]CurrencyAmount
	CurrenciesKnown bool
	Statistics      map[string]string
	StatisticsKnown bool
	Clan            ClanCurrent
	Inventory       *InventoryState
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
	ps99 := strings.TrimSpace(in.PS99UserID)
	if ps99 == "" {
		ps99 = user
	}
	nowText := in.Now.Format(time.RFC3339)
	sum := Summary{
		SchemaVersion:   SchemaVersion,
		LocalStamp:      in.LocalStamp,
		CollisionSuffix: in.CollisionSuffix,
		ExportedAt:      nowText,
		SessionID:       session,
		Prefix:          in.Prefix,
		Profile:         in.Profile,
		ProfileAlias:    in.Profile,
		AccountID:       strings.TrimSpace(in.AccountID),
		LinkedAccountID: strings.TrimSpace(in.LinkedAccountID),
		UserID:          user,
		PS99UserID:      ps99,
		Current: Current{
			CurrenciesKnown: in.CurrenciesKnown,
			Currencies:      in.Currencies,
			StatisticsKnown: in.StatisticsKnown,
			Statistics:      in.Statistics,
			Clan:            in.Clan,
			Inventory:       in.Inventory,
		},
		Warnings: append([]string(nil), in.Warnings...),
	}
	five := int64(5 * 60)
	thirty := int64(30 * 60)
	hour := int64(60 * 60)
	sum.Windows.Five = buildWindow(in, session, nowText, &five, baselineClosest)
	sum.Windows.Thirty = buildWindow(in, session, nowText, &thirty, baselineClosest)
	sum.Windows.Hour = buildWindow(in, session, nowText, &hour, baselineClosest)
	sum.Windows.Session = buildWindow(in, session, nowText, nil, baselineEarliest)
	sum.Windows.Previous = buildWindow(in, session, nowText, nil, baselineLatest)
	return sum
}

const (
	baselineClosest = iota
	baselineEarliest
	baselineLatest
)

func buildWindow(in Input, session, nowText string, requested *int64, kind int) Window {
	w := Window{CurrentAt: nowText, RequestedSeconds: requested}
	base, ok, reject := selectBaseline(in.Priors, in.Now, session, in.historyIdentity(), requested, kind)
	if !ok {
		if reject != "" {
			w.Reason = reject
		} else {
			w.Reason = "no earlier snapshot in this session"
		}
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
	mids := priorsBetween(in.Priors, base, in.Now, session, in.historyIdentity())
	switch {
	case in.CurrenciesKnown && base.CurrenciesKnown:
		w.ChangedCurrencies = diffCurrencies(base, mids, in.Currencies, elapsed, nowText)
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
	w.Eggs = diffEggs(base, mids, in.Statistics, in.StatisticsKnown, elapsed, nowText)
	w.Inventory = inventoryWindow(base.Inventory, in.Inventory)
	return w
}

func inventoryWindow(base, cur *InventoryState) *InventoryWindow {
	baseKnown := base != nil && base.Known
	curKnown := cur != nil && cur.Known
	if !baseKnown && !curKnown {
		return nil
	}
	baseQty, curQty := "", ""
	baseRows, curRows := "", ""
	baseGold, curGold := "", ""
	baseRain, curRain := "", ""
	baseShiny, curShiny := "", ""
	baseInv, curInv := "", ""
	if baseKnown {
		baseQty, baseRows = base.PetQuantity, base.PetRows
		baseGold, baseRain, baseShiny = base.Golden, base.Rainbow, base.Shiny
		baseInv = base.InventoryRows
	}
	if curKnown {
		curQty, curRows = cur.PetQuantity, cur.PetRows
		curGold, curRain, curShiny = cur.Golden, cur.Rainbow, cur.Shiny
		curInv = cur.InventoryRows
	}
	return &InventoryWindow{
		PetQuantity:   plainInventoryCount(baseQty, curQty, baseKnown, curKnown),
		PetRows:       plainInventoryCount(baseRows, curRows, baseKnown, curKnown),
		Golden:        plainInventoryCount(baseGold, curGold, baseKnown, curKnown),
		Rainbow:       plainInventoryCount(baseRain, curRain, baseKnown, curKnown),
		Shiny:         plainInventoryCount(baseShiny, curShiny, baseKnown, curKnown),
		Huge:          hugeInventoryCount(base, cur),
		InventoryRows: plainInventoryCount(baseInv, curInv, baseKnown, curKnown),
	}
}

func plainInventoryCount(baseline, current string, baseKnown, curKnown bool) *InventoryCount {
	switch {
	case baseKnown && curKnown:
		delta, ok := Sub(current, baseline)
		if !ok {
			return &InventoryCount{Status: "unavailable", Reason: "inventory aggregate is not an exact integer", Baseline: baseline, Current: current}
		}
		return &InventoryCount{Baseline: baseline, Current: current, Delta: delta}
	case !baseKnown:
		return &InventoryCount{Status: "unavailable", Reason: "baseline inventory aggregates are absent", Current: current}
	default:
		return &InventoryCount{Status: "unavailable", Reason: "current inventory aggregates are absent", Baseline: baseline}
	}
}

func hugeInventoryCount(base, cur *InventoryState) *InventoryCount {
	baseKnown := base != nil && base.Known && base.HugeKnown
	curKnown := cur != nil && cur.Known && cur.HugeKnown
	if baseKnown && curKnown {
		return plainInventoryCount(base.Huge, cur.Huge, true, true)
	}
	out := &InventoryCount{Status: "unavailable", Reason: "huge classification is absent"}
	if baseKnown {
		out.Baseline = base.Huge
	}
	if curKnown {
		out.Current = cur.Huge
	}
	return out
}

func selectBaseline(priors []Prior, now time.Time, session string, cur HistoryIdentity, requested *int64, kind int) (Prior, bool, string) {
	var best Prior
	found := false
	sawSame := false
	sawOther := false
	for _, p := range priors {
		if !p.ExportedAt.Before(now) {
			continue
		}
		if SameHistory(cur, p.historyIdentity()) {
			sawSame = true
			if p.SessionID != session {
				continue
			}
		} else {
			sawOther = true
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
		if betterBaseline(p, best, requested, kind, now) {
			best = p
		}
	}
	if !found && !sawSame && sawOther {
		return Prior{}, false, BaselineIdentityRejected
	}
	return best, found, ""
}

func betterBaseline(p, best Prior, requested *int64, kind int, now time.Time) bool {
	switch kind {
	case baselineLatest:
		if !p.ExportedAt.Equal(best.ExportedAt) {
			return p.ExportedAt.After(best.ExportedAt)
		}
		return p.File > best.File
	case baselineEarliest:
		if !p.ExportedAt.Equal(best.ExportedAt) {
			return p.ExportedAt.Before(best.ExportedAt)
		}
		return p.File > best.File
	default:
		if requested == nil {
			return false
		}
		return closerBaseline(p, best, *requested, now)
	}
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

func diffCurrencies(base Prior, mids []Prior, current map[string]CurrencyAmount, elapsed int64, nowText string) map[string]CurrencyDelta {
	seen := map[string]struct{}{}
	for id := range base.Currencies {
		seen[id] = struct{}{}
	}
	for id := range current {
		seen[id] = struct{}{}
	}
	for _, mid := range mids {
		for id := range mid.Currencies {
			seen[id] = struct{}{}
		}
	}
	out := map[string]CurrencyDelta{}
	for id := range seen {
		bv, bok := base.Currencies[id]
		cur, cok := current[id]
		cv, cvOK := "", false
		if cok {
			cv = cur.Amount
			cvOK = cv != ""
		}
		points := currencyPoints(base, mids, current, id, nowText)
		if bok && cvOK && SameAmount(bv, cv) {
			block := continuityBreak(points, false)
			if !block.blocked {
				continue
			}
			delta := CurrencyDelta{Baseline: bv, Current: cv}
			if d, ok := Sub(cv, bv); ok {
				delta.Delta = d
			}
			applyRateBlock(&delta, block)
			out[id] = delta
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
			finishCurrencyRate(&delta, points, elapsed, false)
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
	curID, curPoints, curPointsOK, curPlace, curPlaceOK, curUser, curUserOK := currentBattle(clan)
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
	out.Place = placeMetric(base.Place, base.PlaceOK, curPlace, curPlaceOK, elapsed)
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

func currentBattle(clan ClanCurrent) (id, points string, pointsOK bool, place string, placeOK bool, user string, userOK bool) {
	if clan.ActiveBattle == nil {
		return "", "", false, "", false, "", false
	}
	b := clan.ActiveBattle
	place, placeOK = placeText(b.Place)
	return b.ID, b.Points, b.Points != "", place, placeOK, b.UserContribution, b.UserContribution != ""
}

// placeText accepts a positive 1-based place. Zero and negative values are
// not a place, so they are not stored as a baseline.
func placeText(n *int64) (string, bool) {
	if n == nil || *n <= 0 {
		return "", false
	}
	return strconv.FormatInt(*n, 10), true
}

func placeMetric(base string, baseOK bool, cur string, curOK bool, elapsed int64) *MetricDelta {
	d := metric(base, baseOK, cur, curOK, elapsed, "clan battle place")
	if d.Status != "unavailable" {
		return d
	}
	switch d.Reason {
	case "clan battle place are absent":
		d.Reason = "clan battle place is absent"
	case "absent prior point":
		d.Reason = "absent prior place"
	case "absent current point":
		d.Reason = "absent current place"
	}
	return d
}

func diffEggs(base Prior, mids []Prior, current map[string]string, known bool, elapsed int64, nowText string) *CurrencyDelta {
	cur, curOK := statAmount(current, known, "Eggs Opened")
	prev, prevOK := statAmount(base.Stats, base.StatsKnown, "Eggs Opened")
	if !curOK && !prevOK {
		return nil
	}
	d := &CurrencyDelta{}
	if prevOK {
		d.Baseline = prev
	}
	if curOK {
		d.Current = cur
	}
	if !prevOK || !curOK {
		d.Status = "unavailable"
		switch {
		case !prevOK && !curOK:
			d.Reason = "eggs opened are absent"
		case !prevOK:
			d.Reason = "absent prior eggs opened"
		default:
			d.Reason = "absent current eggs opened"
		}
		return d
	}
	delta, ok := Sub(cur, prev)
	if !ok {
		d.Status = "unavailable"
		d.Reason = "amount is not an exact integer"
		return d
	}
	d.Delta = delta
	finishCurrencyRate(d, eggPoints(base, mids, current, known, nowText), elapsed, true)
	return d
}

func statAmount(m map[string]string, known bool, key string) (string, bool) {
	if !known || len(m) == 0 {
		return "", false
	}
	for k, v := range m {
		if strings.EqualFold(strings.TrimSpace(k), key) && strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v), true
		}
	}
	return "", false
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

// amountObs is one exact balance or counter observation in time order.
type amountObs struct {
	atText    string
	listKnown bool
	present   bool
	exact     bool
	amount    string
}

type rateBlock struct {
	blocked bool
	status  string
	reason  string
	at      string
}

type continuityEvent struct {
	kind string
	at   string
	idx  int
}

// priorsBetween returns same-account observations strictly after the baseline
// and strictly before the current snapshot. Rate continuity is derived from
// those balances. Old summaries do not need epoch metadata.
func priorsBetween(priors []Prior, base Prior, now time.Time, session string, cur HistoryIdentity) []Prior {
	var mid []Prior
	for _, p := range priors {
		if p.SessionID != session || !SameHistory(cur, p.historyIdentity()) {
			continue
		}
		if !p.ExportedAt.After(base.ExportedAt) || !p.ExportedAt.Before(now) {
			continue
		}
		mid = append(mid, p)
	}
	sort.SliceStable(mid, func(i, j int) bool {
		return mid[i].ExportedAt.Before(mid[j].ExportedAt)
	})
	return mid
}

func priorStamp(p Prior) string {
	if strings.TrimSpace(p.ExportedAtText) != "" {
		return p.ExportedAtText
	}
	if !p.ExportedAt.IsZero() {
		return p.ExportedAt.UTC().Format(time.RFC3339)
	}
	return ""
}

func currencyPoints(base Prior, mids []Prior, current map[string]CurrencyAmount, id, nowText string) []amountObs {
	points := []amountObs{currencyObs(priorStamp(base), base.CurrenciesKnown, base.Currencies, id)}
	for _, p := range mids {
		points = append(points, currencyObs(priorStamp(p), p.CurrenciesKnown, p.Currencies, id))
	}
	points = append(points, currentCurrencyObs(nowText, id, current))
	return points
}

func currencyObs(at string, listKnown bool, amounts map[string]string, id string) amountObs {
	o := amountObs{atText: at, listKnown: listKnown}
	if !listKnown {
		return o
	}
	v, ok := amounts[id]
	if !ok || strings.TrimSpace(v) == "" {
		return o
	}
	o.present = true
	if _, ok := ParseExactInt(v); ok {
		o.exact = true
		o.amount = strings.TrimSpace(v)
	}
	return o
}

func currentCurrencyObs(at, id string, current map[string]CurrencyAmount) amountObs {
	o := amountObs{atText: at, listKnown: true}
	cur, ok := current[id]
	if !ok || strings.TrimSpace(cur.Amount) == "" {
		return o
	}
	o.present = true
	if _, ok := ParseExactInt(cur.Amount); ok {
		o.exact = true
		o.amount = strings.TrimSpace(cur.Amount)
	}
	return o
}

func eggPoints(base Prior, mids []Prior, current map[string]string, known bool, nowText string) []amountObs {
	points := []amountObs{statObs(priorStamp(base), base.StatsKnown, base.Stats)}
	for _, p := range mids {
		points = append(points, statObs(priorStamp(p), p.StatsKnown, p.Stats))
	}
	points = append(points, statObs(nowText, known, current))
	return points
}

func statObs(at string, known bool, stats map[string]string) amountObs {
	o := amountObs{atText: at, listKnown: known}
	v, ok := statAmount(stats, known, "Eggs Opened")
	if !ok {
		return o
	}
	o.present = true
	if _, ok := ParseExactInt(v); ok {
		o.exact = true
		o.amount = v
	}
	return o
}

func comparableObs(o amountObs) bool {
	return o.listKnown && o.present && o.exact
}

// continuityBreak withholds a production rate when any exact decrease or
// missing observation lies strictly after the baseline and at or before the
// current snapshot. A decrease observed at the current snapshot, with no
// earlier break, makes that snapshot the new rate baseline.
func continuityBreak(points []amountObs, counter bool) rateBlock {
	if len(points) < 2 {
		return rateBlock{}
	}
	var events []continuityEvent
	for i := 1; i < len(points); i++ {
		prev, cur := points[i-1], points[i]
		if !comparableObs(prev) || !comparableObs(cur) {
			events = append(events, continuityEvent{kind: "gap", at: cur.atText, idx: i})
			continue
		}
		left, lok := ParseExactInt(prev.amount)
		right, rok := ParseExactInt(cur.amount)
		if !lok || !rok {
			events = append(events, continuityEvent{kind: "gap", at: cur.atText, idx: i})
			continue
		}
		if right.Cmp(left) < 0 {
			events = append(events, continuityEvent{kind: "decrease", at: cur.atText, idx: i})
		}
	}
	if len(events) == 0 {
		return rateBlock{}
	}
	last := events[len(events)-1]
	at := continuityBoundary(points, last)
	onlyTerminalDecrease := len(events) == 1 && last.kind == "decrease" && last.idx == len(points)-1
	if onlyTerminalDecrease {
		reason := "observed balance decrease"
		if counter {
			reason = "counter decreased"
		}
		return rateBlock{blocked: true, status: "reset", reason: reason, at: at}
	}
	for _, ev := range events {
		if ev.kind == "decrease" {
			reason := "balance decrease occurred within interval"
			if counter {
				reason = "counter decreased within interval"
			}
			return rateBlock{blocked: true, status: "unavailable", reason: reason, at: at}
		}
	}
	return rateBlock{blocked: true, status: "unavailable", reason: "rate continuity unavailable across missing observation", at: at}
}

func continuityBoundary(points []amountObs, last continuityEvent) string {
	if last.kind == "decrease" {
		return last.at
	}
	for i := last.idx; i < len(points); i++ {
		if comparableObs(points[i]) {
			return points[i].atText
		}
	}
	return last.at
}

func applyRateBlock(d *CurrencyDelta, block rateBlock) {
	d.RateStatus = block.status
	d.RateReason = block.reason
	d.RateResetAt = block.at
}

func finishCurrencyRate(d *CurrencyDelta, points []amountObs, elapsed int64, counter bool) {
	if block := continuityBreak(points, counter); block.blocked {
		applyRateBlock(d, block)
		return
	}
	if !setCurrencyRate(d, d.Delta, elapsed) {
		d.Status = "unavailable"
		d.Reason = "elapsed time is not positive"
	}
}

func setCurrencyRate(d *CurrencyDelta, delta string, elapsed int64) bool {
	hourExact, hourApprox, ok := NetPerHourParts(delta, elapsed)
	if !ok {
		return false
	}
	minuteExact, minuteApprox, ok := NetPerMinuteParts(delta, elapsed)
	if !ok {
		return false
	}
	d.NetPerHour = hourExact
	if hourApprox != hourExact {
		d.NetPerHourApprox = hourApprox
	}
	d.NetPerMinute = minuteExact
	if minuteApprox != minuteExact {
		d.NetPerMinuteApprox = minuteApprox
	}
	d.RateBasis = CurrencyRateBasis
	d.Estimated = true
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
