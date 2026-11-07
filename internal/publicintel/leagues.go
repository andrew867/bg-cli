package publicintel

import (
	"encoding/json"

	"github.com/andrew867/bg-cli/internal/analysis"
	"github.com/andrew867/bg-cli/internal/jsonutil"
)

// RankingDepthMeta documents bounded leaderboard depth from official docs.
type RankingDepthMeta struct {
	LiveBoardTopN          int    `json:"liveBoardTopN"`          // top 1000 leagues by stored points
	FinalizedStandingsHint string `json:"finalizedStandingsHint"` // rewardedThroughRank from finalized seasons
	ContributorSampleCap   int    `json:"contributorSampleCap"`   // /v1/leagues/players returns up to 500
	SeasonDetailTopLeagues int    `json:"seasonDetailTopLeagues"` // season detail samples top 100
}

// DefaultLeagueRankingDepth returns documented depth limits.
func DefaultLeagueRankingDepth() RankingDepthMeta {
	return RankingDepthMeta{
		LiveBoardTopN:          1000,
		FinalizedStandingsHint: "see meta.finalized.rewardedThroughRank",
		ContributorSampleCap:   500,
		SeasonDetailTopLeagues: 100,
	}
}

// SeasonMeta is camelCase (GET /v1/leagues/seasons).
type SeasonMeta struct {
	ID              string          `json:"id"`
	Title           string          `json:"title"`
	StartTime       ExactInt        `json:"startTime"`
	FinishTime      ExactInt        `json:"finishTime"`
	DurationSeconds ExactInt        `json:"durationSeconds"`
	State           string          `json:"state"`
	Run             *string         `json:"run,omitempty"`
	Finalized       json.RawMessage `json:"finalized,omitempty"`
}

// SeasonsReport parses GET /v1/leagues/seasons (camelCase season fields).
type SeasonsReport struct {
	OK           bool                  `json:"ok"`
	Active       *string               `json:"active,omitempty"`
	Seasons      []SeasonMeta          `json:"seasons"`
	Completeness analysis.Completeness `json:"completeness"`
	MethodID     string                `json:"methodId"`
	RankingDepth RankingDepthMeta      `json:"rankingDepth"`
}

// LeagueBoardRow is PascalCase (GET /v1/leagues?page=&pageSize=).
type LeagueBoardRow struct {
	Name             string    `json:"name"`
	NameLower        string    `json:"nameLower"`
	ID               string    `json:"id"`
	Icon             *string   `json:"icon,omitempty"`
	Level            ExactInt  `json:"level"`
	Rank             *ExactInt `json:"rank,omitempty"`
	Points           ExactInt  `json:"points"`
	Members          ExactInt  `json:"members"`
	MemberCapacity   ExactInt  `json:"memberCapacity"`
	ContributorCount ExactInt  `json:"contributorCount"`
	Owner            *ExactInt `json:"owner,omitempty"`
	Created          *ExactInt `json:"created,omitempty"`
}

// LeagueLeaderboardReport is a paginated leaderboard page.
type LeagueLeaderboardReport struct {
	OK           bool                  `json:"ok"`
	Season       *string               `json:"season,omitempty"`
	Leagues      []LeagueBoardRow      `json:"leagues"`
	Total        ExactInt              `json:"total"`
	Page         ExactInt              `json:"page"`
	PageSize     ExactInt              `json:"pageSize"`
	Completeness analysis.Completeness `json:"completeness"`
	Warnings     []string              `json:"warnings,omitempty"`
	MethodID     string                `json:"methodId"`
	RankingDepth RankingDepthMeta      `json:"rankingDepth"`
}

// LeagueContributorRow is PascalCase (GET /v1/leagues/players, top 500 sample).
type LeagueContributorRow struct {
	UserID      ExactInt  `json:"userId"`
	DisplayName string    `json:"displayName"`
	Points      ExactInt  `json:"points"`
	Timestamp   *ExactInt `json:"timestamp,omitempty"`
	League      struct {
		Name   string   `json:"name"`
		ID     string   `json:"id"`
		Icon   *string  `json:"icon,omitempty"`
		Points ExactInt `json:"points"`
	} `json:"league"`
}

// LeaguePlayersReport is the contributor aggregate (sampled, not exhaustive).
type LeaguePlayersReport struct {
	OK           bool                   `json:"ok"`
	Season       *string                `json:"season,omitempty"`
	Players      []LeagueContributorRow `json:"players"`
	Completeness analysis.Completeness  `json:"completeness"`
	Warnings     []string               `json:"warnings,omitempty"`
	MethodID     string                 `json:"methodId"`
	RankingDepth RankingDepthMeta       `json:"rankingDepth"`
	SampledCap   ExactInt               `json:"sampledCap"`
}

// ParseSeasons parses GET /v1/leagues/seasons (camelCase entity fields).
func ParseSeasons(body []byte) (SeasonsReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return SeasonsReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return SeasonsReport{}, err
	}
	rep := SeasonsReport{
		OK:           true,
		MethodID:     "publicintel-league-seasons.v1",
		Completeness: analysis.Complete,
		RankingDepth: DefaultLeagueRankingDepth(),
	}
	if v, ok := raw["active"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.Active = &s
		}
	}
	if v, ok := raw["seasons"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				sm, err := parseSeasonMeta(item)
				if err != nil {
					return rep, err
				}
				rep.Seasons = append(rep.Seasons, sm)
			}
		}
	}
	return rep, nil
}

func parseSeasonMeta(item json.RawMessage) (SeasonMeta, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return SeasonMeta{}, err
	}
	sm := SeasonMeta{
		ID:              stringField(m, "id"),
		Title:           stringField(m, "title"),
		StartTime:       parseFieldExact(m, "startTime"),
		FinishTime:      parseFieldExact(m, "finishTime"),
		DurationSeconds: parseFieldExact(m, "durationSeconds"),
		State:           stringField(m, "state"),
	}
	if v, ok := m["run"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			sm.Run = &s
		}
	}
	if v, ok := m["finalized"]; ok {
		sm.Finalized = append(json.RawMessage(nil), v...)
	}
	return sm, nil
}

// ParseLeagueLeaderboard parses GET /v1/leagues (PascalCase league entity fields).
func ParseLeagueLeaderboard(body []byte) (LeagueLeaderboardReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return LeagueLeaderboardReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return LeagueLeaderboardReport{}, err
	}
	rep := LeagueLeaderboardReport{
		OK:           true,
		MethodID:     "publicintel-league-board.v1",
		RankingDepth: DefaultLeagueRankingDepth(),
	}
	if v, ok := raw["season"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.Season = &s
		}
	}
	rep.Total = parseFieldExact(raw, "total")
	rep.Page = parseFieldExact(raw, "page")
	rep.PageSize = parseFieldExact(raw, "pageSize")
	if v, ok := raw["leagues"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseLeagueBoardRow(item)
				if err != nil {
					return rep, err
				}
				rep.Leagues = append(rep.Leagues, row)
			}
		}
	}
	total, errT := rep.Total.Int64()
	if errT == nil && int64(len(rep.Leagues)) < total {
		rep.Completeness = analysis.Partial
		rep.Warnings = append(rep.Warnings, "single page; use pagination for full leaderboard")
	} else {
		rep.Completeness = analysis.Complete
	}
	return rep, nil
}

func parseLeagueBoardRow(item json.RawMessage) (LeagueBoardRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return LeagueBoardRow{}, err
	}
	row := LeagueBoardRow{
		Name:             pickString(m, "Name", "name"),
		NameLower:        pickString(m, "NameLower", "nameLower"),
		ID:               pickString(m, "ID", "id"),
		Level:            pickFieldExact(m, "Level", "level"),
		Points:           pickFieldExact(m, "Points", "points"),
		Members:          pickFieldExact(m, "Members", "members"),
		MemberCapacity:   pickFieldExact(m, "MemberCapacity", "memberCapacity"),
		ContributorCount: pickFieldExact(m, "ContributorCount", "contributorCount"),
	}
	if v := pickRaw(m, "Icon", "icon"); v != nil && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			row.Icon = &s
		}
	}
	if v := pickRaw(m, "Rank", "rank"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		row.Rank = &ei
	}
	if v := pickRaw(m, "Owner", "owner"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		row.Owner = &ei
	}
	if v := pickRaw(m, "Created", "created"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		row.Created = &ei
	}
	return row, nil
}

// ParseLeaguePlayers parses GET /v1/leagues/players (PascalCase; top 500 sample).
func ParseLeaguePlayers(body []byte) (LeaguePlayersReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return LeaguePlayersReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return LeaguePlayersReport{}, err
	}
	depth := DefaultLeagueRankingDepth()
	rep := LeaguePlayersReport{
		OK:           true,
		MethodID:     "publicintel-league-players.v1",
		Completeness: analysis.Sampled,
		RankingDepth: depth,
		SampledCap:   ExactInt{Text: "500", ParseOK: true, Value: int64Ptr(500)},
		Warnings: []string{
			"contributor list is a top-500 sample, not an exhaustive season ranking",
		},
	}
	if v, ok := raw["season"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.Season = &s
		}
	}
	if v, ok := raw["players"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseLeagueContributorRow(item)
				if err != nil {
					return rep, err
				}
				rep.Players = append(rep.Players, row)
			}
		}
	}
	if len(rep.Players) > depth.ContributorSampleCap {
		rep.Warnings = append(rep.Warnings, "players length exceeds documented 500 cap")
		rep.Completeness = analysis.BoundedDepth
	}
	return rep, nil
}

func parseLeagueContributorRow(item json.RawMessage) (LeagueContributorRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return LeagueContributorRow{}, err
	}
	row := LeagueContributorRow{
		UserID:      pickFieldExact(m, "UserID", "userId"),
		DisplayName: pickString(m, "DisplayName", "displayName"),
		Points:      pickFieldExact(m, "Points", "points"),
	}
	if v := pickRaw(m, "Timestamp", "timestamp"); v != nil && string(v) != "null" {
		ei := parseExactIntJSON(v)
		row.Timestamp = &ei
	}
	if v, ok := m["League"]; ok {
		_ = decodeLeagueRef(v, &row.League.Name, &row.League.ID, &row.League.Icon, &row.League.Points)
	} else if v, ok := m["league"]; ok {
		_ = decodeLeagueRef(v, &row.League.Name, &row.League.ID, &row.League.Icon, &row.League.Points)
	}
	return row, nil
}

func decodeLeagueRef(raw json.RawMessage, name, id *string, icon **string, points *ExactInt) error {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(raw, &m); err != nil {
		return err
	}
	*name = pickString(m, "Name", "name")
	*id = pickString(m, "ID", "id")
	*points = pickFieldExact(m, "Points", "points")
	if v := pickRaw(m, "Icon", "icon"); v != nil && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			*icon = &s
		}
	}
	return nil
}

func pickString(m map[string]json.RawMessage, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			var s string
			if json.Unmarshal(v, &s) == nil {
				return s
			}
		}
	}
	return ""
}

func pickRaw(m map[string]json.RawMessage, keys ...string) json.RawMessage {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return v
		}
	}
	return nil
}

func pickFieldExact(m map[string]json.RawMessage, keys ...string) ExactInt {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			return parseExactIntJSON(v)
		}
	}
	return ExactInt{}
}
