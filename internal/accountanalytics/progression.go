package accountanalytics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/snapshot"
)

const progressionMethodID = "accountanalytics.progression.v1"

// ScalarChange is a numeric or string field delta between profile snapshots.
type ScalarChange struct {
	Field    string `json:"field"`
	Previous string `json:"previous,omitempty"`
	Current  string `json:"current,omitempty"`
	Delta    string `json:"delta,omitempty"`
	Kind     string `json:"kind"` // exact_int|string|bool
}

// AchievementStageChange tracks structured achievement stage movement (not naive negatives).
type AchievementStageChange struct {
	AchievementID string `json:"achievementId"`
	FromStage     string `json:"fromStage,omitempty"`
	ToStage       string `json:"toStage,omitempty"`
	ChangeType    string `json:"changeType"` // advanced|regressed|set|cleared|unknown
}

// MapChange captures keyed map field deltas (Goals, Mastery, slots, upgrades).
type MapChange struct {
	Field  string `json:"field"`
	Key    string `json:"key"`
	Kind   string `json:"kind"` // exact_int|bool|json
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
	Delta  string `json:"delta,omitempty"`
}

// ProgressionDiff compares progression-related profile fields.
type ProgressionDiff struct {
	MethodID     string                   `json:"methodId"`
	Scalars      []ScalarChange           `json:"scalars,omitempty"`
	Achievements []AchievementStageChange `json:"achievements,omitempty"`
	MapChanges   []MapChange              `json:"mapChanges,omitempty"`
	Warnings     []string                 `json:"warnings,omitempty"`
}

var progressionScalarFields = []string{
	"Rank", "RankStars", "Rebirths",
}

var progressionMapFields = []string{
	"Goals", "Mastery", "UnlockedZones", "PetEquips", "Upgrades", "UpgradeSlots",
}

// DiffProgression diffs whitelisted progression fields from two profile payloads.
func DiffProgression(fromData, toData json.RawMessage) (ProgressionDiff, error) {
	var fromTop, toTop map[string]json.RawMessage
	if err := json.Unmarshal(fromData, &fromTop); err != nil {
		return ProgressionDiff{}, fmt.Errorf("from profile: %w", err)
	}
	if err := json.Unmarshal(toData, &toTop); err != nil {
		return ProgressionDiff{}, fmt.Errorf("to profile: %w", err)
	}
	out := ProgressionDiff{MethodID: progressionMethodID}
	for _, field := range progressionScalarFields {
		chg := diffScalarField(field, fromTop[field], toTop[field])
		if chg != nil {
			out.Scalars = append(out.Scalars, *chg)
		}
	}
	sort.Slice(out.Scalars, func(i, j int) bool { return out.Scalars[i].Field < out.Scalars[j].Field })

	ach := diffAchievements(fromTop["Achievements"], toTop["Achievements"])
	out.Achievements = ach

	for _, field := range progressionMapFields {
		m := diffMapField(field, fromTop[field], toTop[field])
		out.MapChanges = append(out.MapChanges, m...)
	}
	sort.Slice(out.MapChanges, func(i, j int) bool {
		if out.MapChanges[i].Field != out.MapChanges[j].Field {
			return out.MapChanges[i].Field < out.MapChanges[j].Field
		}
		return out.MapChanges[i].Key < out.MapChanges[j].Key
	})
	return out, nil
}

// DiffProgressionBundles is a convenience wrapper around bundle profile payloads.
func DiffProgressionBundles(from, to *snapshot.Bundle) (ProgressionDiff, error) {
	if from == nil || to == nil || from.ProfileData == nil || to.ProfileData == nil {
		return ProgressionDiff{}, fmt.Errorf("profile data missing")
	}
	return DiffProgression(from.ProfileData, to.ProfileData)
}

func diffScalarField(field string, fromRaw, toRaw json.RawMessage) *ScalarChange {
	if len(fromRaw) == 0 && len(toRaw) == 0 {
		return nil
	}
	from := snapshot.ParseExactInt(fromRaw)
	to := snapshot.ParseExactInt(toRaw)
	if from.ParseOK && to.ParseOK && from.Int != nil && to.Int != nil {
		d, err := snapshot.SubExact(to, from)
		if err != nil || d.Int.Sign() == 0 {
			if from.Text == to.Text {
				return nil
			}
		}
		return &ScalarChange{
			Field: field, Previous: from.Text, Current: to.Text, Delta: d.Text, Kind: "exact_int",
		}
	}
	fs, ts := scalarJSON(fromRaw), scalarJSON(toRaw)
	if fs == ts {
		return nil
	}
	return &ScalarChange{Field: field, Previous: fs, Current: ts, Kind: "string"}
}

func diffMapField(field string, fromRaw, toRaw json.RawMessage) []MapChange {
	var fromM, toM map[string]json.RawMessage
	_ = json.Unmarshal(fromRaw, &fromM)
	_ = json.Unmarshal(toRaw, &toM)
	keys := map[string]struct{}{}
	for k := range fromM {
		keys[k] = struct{}{}
	}
	for k := range toM {
		keys[k] = struct{}{}
	}
	var keyList []string
	for k := range keys {
		keyList = append(keyList, k)
	}
	sort.Strings(keyList)
	var out []MapChange
	for _, k := range keyList {
		fr, tr := fromM[k], toM[k]
		if string(fr) == string(tr) {
			continue
		}
		fe, te := snapshot.ParseExactInt(fr), snapshot.ParseExactInt(tr)
		if fe.ParseOK && te.ParseOK && fe.Int != nil && te.Int != nil {
			d, _ := snapshot.SubExact(te, fe)
			out = append(out, MapChange{
				Field: field, Key: k, Kind: "exact_int",
				Before: fe.Text, After: te.Text, Delta: d.Text,
			})
			continue
		}
		fs, ts := strings.TrimSpace(string(fr)), strings.TrimSpace(string(tr))
		if fs == "true" || fs == "false" || ts == "true" || ts == "false" {
			out = append(out, MapChange{Field: field, Key: k, Kind: "bool", Before: fs, After: ts})
			continue
		}
		out = append(out, MapChange{Field: field, Key: k, Kind: "json", Before: fs, After: ts})
	}
	return out
}

func diffAchievements(fromRaw, toRaw json.RawMessage) []AchievementStageChange {
	var fromM, toM map[string]json.RawMessage
	_ = json.Unmarshal(fromRaw, &fromM)
	_ = json.Unmarshal(toRaw, &toM)
	keys := map[string]struct{}{}
	for k := range fromM {
		keys[k] = struct{}{}
	}
	for k := range toM {
		keys[k] = struct{}{}
	}
	var ids []string
	for k := range keys {
		ids = append(ids, k)
	}
	sort.Strings(ids)
	var out []AchievementStageChange
	for _, id := range ids {
		fs := achievementStage(fromM[id])
		ts := achievementStage(toM[id])
		if fs == ts {
			continue
		}
		chg := AchievementStageChange{
			AchievementID: id,
			FromStage:     fs,
			ToStage:       ts,
		}
		fi, fok := parseStageInt(fs)
		ti, tok := parseStageInt(ts)
		switch {
		case fs == "" && ts != "":
			chg.ChangeType = "set"
		case fs != "" && ts == "":
			chg.ChangeType = "cleared"
		case fok && tok && ti > fi:
			chg.ChangeType = "advanced"
		case fok && tok && ti < fi:
			chg.ChangeType = "regressed"
		default:
			chg.ChangeType = "unknown"
		}
		out = append(out, chg)
	}
	return out
}

func achievementStage(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) == nil {
		if st, ok := top["stage"]; ok {
			return scalarJSON(st)
		}
		if st, ok := top["Stage"]; ok {
			return scalarJSON(st)
		}
		if st, ok := top["currentStage"]; ok {
			return scalarJSON(st)
		}
	}
	return scalarJSON(raw)
}

func parseStageInt(s string) (int, bool) {
	ei := snapshot.ParseExactIntString(s)
	if !ei.ParseOK || ei.Int == nil {
		return 0, false
	}
	return int(ei.Int.Int64()), true
}
