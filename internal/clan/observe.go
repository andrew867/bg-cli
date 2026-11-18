package clan

import (
	"encoding/json"
	"strings"
	"time"
)

// ClanCurrent is the clan section stored on a summary.
type ClanCurrent struct {
	Tag              string         `json:"tag,omitempty"`
	Name             string         `json:"name,omitempty"`
	MemberCount      *int64         `json:"memberCount,omitempty"`
	ContributorCount *int64         `json:"contributorCount,omitempty"`
	ActiveBattle     *BattleCurrent `json:"activeBattle,omitempty"`
	Reason           string         `json:"reason,omitempty"`
}

// BattleCurrent is the signed-in clan's row in the active battle.
type BattleCurrent struct {
	ID                  string `json:"id,omitempty"`
	Title               string `json:"title,omitempty"`
	StartTime           string `json:"startTime,omitempty"`
	FinishTime          string `json:"finishTime,omitempty"`
	Place               *int64 `json:"place,omitempty"`
	Points              string `json:"points,omitempty"`
	UserContribution    string `json:"userContribution,omitempty"`
	UserContributorRank *int64 `json:"userContributorRank,omitempty"`
	Reason              string `json:"reason,omitempty"`

	contrib *int64
}

// Observe builds the current clan block from the public clan document and
// the active-battle document. Amounts stay exact. A finished battle is not
// reported as active.
func Observe(now time.Time, userID, tag string, clanRaw, battleRaw json.RawMessage, clanOK, battleFetched bool) ClanCurrent {
	out := ClanCurrent{}
	if tag != "" {
		out.Tag = tag
	}
	if clanOK {
		applyClanIdentity(&out, clanRaw)
	} else if tag == "" {
		out.Reason = "clan tag is absent from profile and extended profile"
	}

	active, sched := parseActive(now, battleRaw, battleFetched)
	if !active {
		if out.Reason == "" {
			out.Reason = sched.reason
		}
		return out
	}

	battle := &BattleCurrent{
		ID:         sched.id,
		Title:      sched.title,
		StartTime:  sched.start,
		FinishTime: sched.finish,
	}
	if !clanOK {
		battle.Reason = "clan payload is absent"
		out.ActiveBattle = battle
		return out
	}
	applyBattleRow(battle, clanRaw, sched.id, userID)
	out.ActiveBattle = battle
	if battle.ContributorCount() != nil {
		out.ContributorCount = battle.ContributorCount()
	}
	return out
}

// contributorCount is stored on the clan object; the battle helper fills it
// via a side channel on BattleCurrent using an unexported field.
func (b *BattleCurrent) ContributorCount() *int64 { return b.contrib }

// contrib is not serialized. Observe copies it onto ClanCurrent.
func (b *BattleCurrent) setContrib(n *int64) { b.contrib = n }

type schedule struct {
	active bool
	id     string
	title  string
	start  string
	finish string
	reason string
}

func parseActive(now time.Time, raw json.RawMessage, fetched bool) (bool, schedule) {
	if !fetched {
		return false, schedule{reason: "active clan battle request failed"}
	}
	if len(raw) == 0 || string(raw) == "null" {
		return false, schedule{reason: "active clan battle payload is absent"}
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return false, schedule{reason: "battle schedule is absent"}
	}
	cfg := top
	if inner, ok := top["configData"]; ok && string(inner) != "null" {
		var nested map[string]json.RawMessage
		if json.Unmarshal(inner, &nested) == nil && nested != nil {
			cfg = nested
		}
	}
	if rawEnabled, ok := cfg["Enabled"]; ok && string(rawEnabled) != "null" {
		var enabled bool
		if json.Unmarshal(rawEnabled, &enabled) == nil && !enabled {
			return false, schedule{reason: "battle is not enabled"}
		}
	}
	id, _ := jsonString(top["configName"])
	if id == "" {
		id, _ = jsonString(cfg["configName"])
	}
	title, _ := jsonString(cfg["Title"])
	if title == "" {
		title, _ = jsonString(top["Title"])
	}
	start, startOK := unixString(cfg["StartTime"])
	finish, finishOK := unixString(cfg["FinishTime"])
	if id == "" || !startOK || !finishOK || finish <= start {
		return false, schedule{reason: "battle schedule is absent"}
	}
	nowUnix := now.Unix()
	if nowUnix < start || nowUnix >= finish {
		return false, schedule{reason: "no battle is active"}
	}
	return true, schedule{
		active: true,
		id:     id,
		title:  title,
		start:  time.Unix(start, 0).UTC().Format(time.RFC3339),
		finish: time.Unix(finish, 0).UTC().Format(time.RFC3339),
	}
}

func unixString(raw json.RawMessage) (int64, bool) {
	if len(raw) == 0 || string(raw) == "null" {
		return 0, false
	}
	n, ok := ParseExactInt(strings.TrimSpace(string(raw)))
	if !ok || !n.IsInt64() || n.Sign() < 0 {
		return 0, false
	}
	return n.Int64(), true
}

func applyClanIdentity(out *ClanCurrent, clanRaw json.RawMessage) {
	var top map[string]json.RawMessage
	if json.Unmarshal(clanRaw, &top) != nil {
		if out.Reason == "" {
			out.Reason = "clan payload is absent"
		}
		return
	}
	if name, ok := jsonString(top["Name"]); ok {
		out.Name = name
	}
	if members, ok := top["Members"]; ok && string(members) != "null" {
		var arr []json.RawMessage
		if json.Unmarshal(members, &arr) == nil {
			n := int64(len(arr))
			out.MemberCount = &n
		}
	}
}

func applyBattleRow(battle *BattleCurrent, clanRaw json.RawMessage, battleID, userID string) {
	var top map[string]json.RawMessage
	if json.Unmarshal(clanRaw, &top) != nil {
		battle.Reason = "clan payload is absent"
		return
	}
	rawBattles, ok := top["Battles"]
	if !ok || string(rawBattles) == "null" {
		battle.Reason = "active battle is absent from the clan Battles object"
		return
	}
	var battles map[string]json.RawMessage
	if json.Unmarshal(rawBattles, &battles) != nil {
		battle.Reason = "active battle is absent from the clan Battles object"
		return
	}
	raw, ok := matchBattle(battles, battleID)
	if !ok {
		battle.Reason = "active battle is absent from the clan Battles object"
		return
	}
	var row map[string]json.RawMessage
	if json.Unmarshal(raw, &row) != nil {
		battle.Reason = "active battle is absent from the clan Battles object"
		return
	}
	if points, ok := exactField(row, "Points"); ok {
		battle.Points = points
	} else {
		battle.Reason = "clan battle points are absent"
	}
	if place, ok := exactInt(row["Place"]); ok {
		battle.Place = &place
	}
	contribRaw, hasContrib := row["PointContributions"]
	if !hasContrib || string(contribRaw) == "null" {
		if battle.Reason == "" {
			battle.Reason = "player contribution is absent"
		}
		return
	}
	var contribs []map[string]json.RawMessage
	if json.Unmarshal(contribRaw, &contribs) != nil {
		battle.Reason = joinReason(battle.Reason, "player contribution is absent")
		return
	}
	n := int64(len(contribs))
	battle.setContrib(&n)
	userCanon, userOK := ParseExactInt(userID)
	if !userOK {
		battle.Reason = joinReason(battle.Reason, "configured user id is not an integer")
		return
	}
	var (
		mine       string
		mineOK     bool
		dups       int
		allNumeric = true
	)
	for _, c := range contribs {
		if _, ptsOK := exactField(c, "Points"); !ptsOK {
			allNumeric = false
		}
		uid, uidOK := exactField(c, "UserID")
		if !uidOK {
			continue
		}
		id, idOK := ParseExactInt(uid)
		if !idOK || id.Cmp(userCanon) != 0 {
			continue
		}
		dups++
		if pts, ptsOK := exactField(c, "Points"); ptsOK {
			mine = pts
			mineOK = true
		}
	}
	if dups == 0 {
		battle.Reason = joinReason(battle.Reason, "player is absent from PointContributions")
		return
	}
	if dups > 1 {
		battle.Reason = joinReason(battle.Reason, "duplicate contribution for the configured user")
		return
	}
	if !mineOK {
		battle.Reason = joinReason(battle.Reason, "player contribution is absent")
		return
	}
	battle.UserContribution = mine
	if !allNumeric {
		battle.Reason = joinReason(battle.Reason, "contribution points are not all numeric")
		return
	}
	mineN, _ := ParseExactInt(mine)
	var greater int64
	for _, c := range contribs {
		pts, ptsOK := exactField(c, "Points")
		if !ptsOK {
			continue
		}
		ptN, ok := ParseExactInt(pts)
		if ok && ptN.Cmp(mineN) > 0 {
			greater++
		}
	}
	rank := greater + 1
	battle.UserContributorRank = &rank
}

func matchBattle(battles map[string]json.RawMessage, id string) (json.RawMessage, bool) {
	if raw, ok := battles[id]; ok {
		return raw, true
	}
	var found json.RawMessage
	matches := 0
	for key, raw := range battles {
		if strings.EqualFold(key, id) {
			found = raw
			matches++
			continue
		}
		var row map[string]json.RawMessage
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		if bid, ok := jsonString(row["BattleID"]); ok && strings.EqualFold(bid, id) {
			found = raw
			matches++
		}
	}
	if matches == 1 {
		return found, true
	}
	return nil, false
}

func exactField(obj map[string]json.RawMessage, key string) (string, bool) {
	raw, ok := obj[key]
	if !ok || string(raw) == "null" {
		return "", false
	}
	text := rawNumberText(raw)
	if n, ok := ParseExactInt(text); ok {
		return n.String(), true
	}
	return "", false
}

func exactInt(raw json.RawMessage) (int64, bool) {
	n, ok := unixString(raw)
	return n, ok
}

func joinReason(a, b string) string {
	if a == "" {
		return b
	}
	if strings.Contains(a, b) {
		return a
	}
	return a + "; " + b
}
