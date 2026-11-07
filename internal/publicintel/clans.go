package publicintel

import (
	"encoding/json"

	"github.com/andrew867/bg-cli/internal/analysis"
	"github.com/andrew867/bg-cli/internal/jsonutil"
)

// ClanSampleMeta documents default clan endpoint sampling.
type ClanSampleMeta struct {
	PlayersAggregateClans int `json:"playersAggregateClans"` // default 25 clans
	BattleTopClans        int `json:"battleTopClans"`        // top 100 by all-time points
	BattleTopContributors int `json:"battleTopContributors"` // capped at 200
}

// DefaultClanSampleMeta returns documented caps.
func DefaultClanSampleMeta() ClanSampleMeta {
	return ClanSampleMeta{
		PlayersAggregateClans: 25,
		BattleTopClans:        100,
		BattleTopContributors: 200,
	}
}

// ClanPlayerRow is PascalCase (GET /v1/clans/players).
type ClanPlayerRow struct {
	UserID             ExactInt `json:"userId"`
	DisplayName        string   `json:"displayName"`
	ActiveBattlePoints ExactInt `json:"activeBattlePoints"`
	AllTimeDiamonds    ExactInt `json:"allTimeDiamonds"`
	TotalBattles       ExactInt `json:"totalBattles"`
	EarnedMedals       ExactInt `json:"earnedMedals"`
	AvgPlace           *float64 `json:"avgPlace,omitempty"`
	Clan               struct {
		Name        string    `json:"name"`
		Icon        string    `json:"icon"`
		CountryCode string    `json:"countryCode"`
		Place       *ExactInt `json:"place,omitempty"`
	} `json:"clan"`
}

// ClanPlayersReport is the sampled player aggregate.
type ClanPlayersReport struct {
	OK                 bool                  `json:"ok"`
	Players            []ClanPlayerRow       `json:"players"`
	ActiveBattleConfig *string               `json:"activeBattleConfigName,omitempty"`
	SampledClans       ExactInt              `json:"sampledClans"`
	Total              ExactInt              `json:"total"`
	Page               *ExactInt             `json:"page,omitempty"`
	PageSize           *ExactInt             `json:"pageSize,omitempty"`
	LoadError          *string               `json:"loadError,omitempty"`
	Completeness       analysis.Completeness `json:"completeness"`
	Warnings           []string              `json:"warnings,omitempty"`
	MethodID           string                `json:"methodId"`
	Sample             ClanSampleMeta        `json:"sample"`
}

// ClanBattleReport is GET /v1/clans/battles/:battleId (camelCase entity fields).
type ClanBattleReport struct {
	OK           bool                  `json:"ok"`
	BattleID     string                `json:"battleId"`
	Meta         json.RawMessage       `json:"meta,omitempty"`
	Stats        ClanBattleStats       `json:"stats"`
	TopClans     []ClanBattleClanRow   `json:"topClans,omitempty"`
	TopPlayers   []ClanBattlePlayerRow `json:"topPlayers,omitempty"`
	Completeness analysis.Completeness `json:"completeness"`
	Warnings     []string              `json:"warnings,omitempty"`
	MethodID     string                `json:"methodId"`
	Sample       ClanSampleMeta        `json:"sample"`
}

type ClanBattleStats struct {
	ParticipatingClans ExactInt `json:"participatingClans"`
	SampledClans       ExactInt `json:"sampledClans"`
	TotalClanPoints    ExactInt `json:"totalClanPoints"`
	TotalContributors  ExactInt `json:"totalContributors"`
}

type ClanBattleClanRow struct {
	Rank          ExactInt  `json:"rank"`
	Name          string    `json:"name"`
	Points        ExactInt  `json:"points"`
	Members       ExactInt  `json:"members"`
	ReportedPlace *ExactInt `json:"reportedPlace,omitempty"`
}

type ClanBattlePlayerRow struct {
	Rank        ExactInt `json:"rank"`
	UserID      ExactInt `json:"userId"`
	DisplayName string   `json:"displayName"`
	Points      ExactInt `json:"points"`
	ClanName    string   `json:"clanName"`
}

// ParseClanPlayers parses GET /v1/clans/players (PascalCase players[]).
func ParseClanPlayers(body []byte) (ClanPlayersReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return ClanPlayersReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return ClanPlayersReport{}, err
	}
	rep := ClanPlayersReport{
		OK:           true,
		MethodID:     "publicintel-clan-players.v1",
		Completeness: analysis.Sampled,
		Sample:       DefaultClanSampleMeta(),
		Warnings: []string{
			"aggregate samples top clans only; absence does not prove a player is not in-game",
		},
	}
	if v, ok := raw["activeBattleConfigName"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.ActiveBattleConfig = &s
		}
	}
	rep.SampledClans = parseFieldExact(raw, "sampledClans")
	rep.Total = parseFieldExact(raw, "total")
	if v, ok := raw["page"]; ok {
		ei := parseExactIntJSON(v)
		rep.Page = &ei
	}
	if v, ok := raw["pageSize"]; ok {
		ei := parseExactIntJSON(v)
		rep.PageSize = &ei
	}
	if v, ok := raw["loadError"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.LoadError = &s
			rep.Completeness = analysis.Partial
			rep.Warnings = append(rep.Warnings, "loadError present: "+s)
		}
	}
	if v, ok := raw["players"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseClanPlayerRow(item)
				if err != nil {
					return rep, err
				}
				rep.Players = append(rep.Players, row)
			}
		}
	}
	if rep.Total.ParseOK {
		if total, err := rep.Total.Int64(); err == nil && int64(len(rep.Players)) < total && rep.Page == nil {
			rep.Warnings = append(rep.Warnings, "response truncated without explicit page; treat as partial sample")
			rep.Completeness = analysis.Partial
		}
	}
	return rep, nil
}

func parseClanPlayerRow(item json.RawMessage) (ClanPlayerRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ClanPlayerRow{}, err
	}
	row := ClanPlayerRow{
		UserID:             pickFieldExact(m, "UserID", "userId"),
		DisplayName:        pickString(m, "DisplayName", "displayName"),
		ActiveBattlePoints: pickFieldExact(m, "ActiveBattlePoints", "activeBattlePoints"),
		AllTimeDiamonds:    pickFieldExact(m, "AllTimeDiamonds", "allTimeDiamonds"),
		TotalBattles:       pickFieldExact(m, "TotalBattles", "totalBattles"),
		EarnedMedals:       pickFieldExact(m, "EarnedMedals", "earnedMedals"),
	}
	if v := pickRaw(m, "AvgPlace", "avgPlace"); v != nil && string(v) != "null" {
		var f float64
		if json.Unmarshal(v, &f) == nil {
			row.AvgPlace = &f
		}
	}
	if v, ok := m["Clan"]; ok {
		decodeClanRef(v, &row.Clan)
	} else if v, ok := m["clan"]; ok {
		decodeClanRef(v, &row.Clan)
	}
	return row, nil
}

func decodeClanRef(raw json.RawMessage, ref *struct {
	Name        string    `json:"name"`
	Icon        string    `json:"icon"`
	CountryCode string    `json:"countryCode"`
	Place       *ExactInt `json:"place,omitempty"`
}) {
	var m map[string]json.RawMessage
	if jsonutil.Decode(raw, &m) != nil {
		return
	}
	ref.Name = pickString(m, "Name", "name")
	ref.Icon = pickString(m, "Icon", "icon")
	ref.CountryCode = pickString(m, "CountryCode", "countryCode")
	if v := pickRaw(m, "Place", "place"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		ref.Place = &ei
	}
}

// ParseClanBattle parses GET /v1/clans/battles/:battleId (camelCase battle detail).
func ParseClanBattle(body []byte) (ClanBattleReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return ClanBattleReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return ClanBattleReport{}, err
	}
	rep := ClanBattleReport{
		OK:           true,
		MethodID:     "publicintel-clan-battle.v1",
		Completeness: analysis.Sampled,
		Sample:       DefaultClanSampleMeta(),
		Warnings: []string{
			"leaderboards built from top clans by all-time points; capped contributor depth",
		},
	}
	if v, ok := raw["meta"]; ok {
		rep.Meta = append(json.RawMessage(nil), v...)
		var mm map[string]json.RawMessage
		if jsonutil.Decode(v, &mm) == nil {
			rep.BattleID = pickString(mm, "id")
		}
	}
	if v, ok := raw["stats"]; ok {
		var sm map[string]json.RawMessage
		if jsonutil.Decode(v, &sm) == nil {
			rep.Stats = ClanBattleStats{
				ParticipatingClans: parseFieldExact(sm, "participatingClans"),
				SampledClans:       parseFieldExact(sm, "sampledClans"),
				TotalClanPoints:    parseFieldExact(sm, "totalClanPoints"),
				TotalContributors:  parseFieldExact(sm, "totalContributors"),
			}
		}
	}
	if v, ok := raw["topClans"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseClanBattleClanRow(item)
				if err != nil {
					return rep, err
				}
				rep.TopClans = append(rep.TopClans, row)
			}
		}
	}
	if v, ok := raw["topPlayers"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseClanBattlePlayerRow(item)
				if err != nil {
					return rep, err
				}
				rep.TopPlayers = append(rep.TopPlayers, row)
			}
		}
	}
	if len(rep.TopPlayers) > rep.Sample.BattleTopContributors {
		rep.Warnings = append(rep.Warnings, "topPlayers exceeds documented 200 cap")
	}
	return rep, nil
}

func parseClanBattleClanRow(item json.RawMessage) (ClanBattleClanRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ClanBattleClanRow{}, err
	}
	row := ClanBattleClanRow{
		Rank:    pickFieldExact(m, "rank"),
		Name:    pickString(m, "name"),
		Points:  pickFieldExact(m, "points"),
		Members: pickFieldExact(m, "members"),
	}
	if v := pickRaw(m, "reportedPlace"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		row.ReportedPlace = &ei
	}
	return row, nil
}

func parseClanBattlePlayerRow(item json.RawMessage) (ClanBattlePlayerRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ClanBattlePlayerRow{}, err
	}
	row := ClanBattlePlayerRow{
		Rank:        pickFieldExact(m, "rank"),
		UserID:      pickFieldExact(m, "userId"),
		DisplayName: pickString(m, "displayName"),
		Points:      pickFieldExact(m, "points"),
	}
	if v, ok := m["clan"]; ok {
		var cm map[string]json.RawMessage
		if jsonutil.Decode(v, &cm) == nil {
			row.ClanName = pickString(cm, "name")
		}
	} else if v, ok := m["name"]; ok {
		// some rows flatten clan name
		_ = json.Unmarshal(v, &row.ClanName)
	}
	if row.ClanName == "" {
		row.ClanName = pickString(m, "clanName")
	}
	return row, nil
}
