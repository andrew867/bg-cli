package api

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// GetPlayer fetches opted-in public player views.
func (c *Client) GetPlayer(ctx context.Context, slug string, include []string) (Result, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return Result{}, fmt.Errorf("player slug is required")
	}
	q := url.Values{}
	if len(include) > 0 {
		q.Set("include", strings.Join(include, ","))
	}
	path := "/v1/players/" + url.PathEscape(slug)
	return c.GetPublic(ctx, path, q, BodyLimitDefault)
}

// ListLeaderboards fetches /v1/leaderboards.
func (c *Client) ListLeaderboards(ctx context.Context) (Result, error) {
	return c.GetPublic(ctx, "/v1/leaderboards", nil, BodyLimitDefault)
}

// GetLeaderboardPage fetches a leaderboard page.
func (c *Client) GetLeaderboardPage(ctx context.Context, boardID, source string, page, pageSize int) (Result, error) {
	boardID = strings.TrimSpace(boardID)
	if boardID == "" {
		return Result{}, fmt.Errorf("board id is required")
	}
	if source == "" {
		source = "live"
	}
	if source != "live" && source != "final" {
		return Result{}, fmt.Errorf("source must be live or final")
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		return Result{}, fmt.Errorf("page-size must be between 1 and 100")
	}
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("pageSize", strconv.Itoa(pageSize))
	q.Set("source", source)
	path := "/v1/leaderboards/" + url.PathEscape(boardID)
	return c.GetPublic(ctx, path, q, BodyLimitDefault)
}

// GetLeaderboardPlayer fetches one retained player row.
func (c *Client) GetLeaderboardPlayer(ctx context.Context, boardID, userID, source string) (Result, error) {
	if strings.TrimSpace(boardID) == "" || strings.TrimSpace(userID) == "" {
		return Result{}, fmt.Errorf("board id and roblox user id are required")
	}
	if source == "" {
		source = "live"
	}
	if source != "live" && source != "final" {
		return Result{}, fmt.Errorf("source must be live or final")
	}
	q := url.Values{}
	q.Set("source", source)
	path := "/v1/leaderboards/" + url.PathEscape(boardID) + "/players/" + url.PathEscape(userID)
	return c.GetPublic(ctx, path, q, BodyLimitDefault)
}

// ListCollections fetches /api/collections.
func (c *Client) ListCollections(ctx context.Context) (Result, error) {
	return c.GetPublic(ctx, "/api/collections", nil, BodyLimitDefault)
}

// GetCollection fetches /api/collection/:name.
func (c *Client) GetCollection(ctx context.Context, name string, filters url.Values) (Result, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Result{}, fmt.Errorf("collection name is required")
	}
	path := "/api/collection/" + url.PathEscape(name)
	return c.GetPublic(ctx, path, filters, BodyLimitDefault)
}

// GetRAP fetches /api/rap.
func (c *Client) GetRAP(ctx context.Context, category, id string) (Result, error) {
	q := url.Values{}
	if category != "" {
		q.Set("category", category)
	}
	if id != "" {
		q.Set("id", id)
	}
	return c.GetPublic(ctx, "/api/rap", q, BodyLimitRAP)
}

// ListReleases fetches GET /v1/releases.
func (c *Client) ListReleases(ctx context.Context, page, pageSize int) (Result, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	return c.GetPublic(ctx, "/v1/releases", q, BodyLimitDefault)
}

// GetRelease fetches GET /v1/releases/:version (version number or "latest").
func (c *Client) GetRelease(ctx context.Context, version string) (Result, error) {
	version = strings.TrimSpace(version)
	if version == "" {
		return Result{}, fmt.Errorf("release version is required")
	}
	path := "/v1/releases/" + url.PathEscape(version)
	return c.GetPublic(ctx, path, nil, BodyLimitDefault)
}

// ListItemChanges fetches GET /v1/items/changes (since required).
func (c *Client) ListItemChanges(ctx context.Context, since string, page, pageSize int) (Result, error) {
	since = strings.TrimSpace(since)
	if since == "" {
		return Result{}, fmt.Errorf("since is required")
	}
	q := url.Values{}
	q.Set("since", since)
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	return c.GetPublic(ctx, "/v1/items/changes", q, BodyLimitDefault)
}

// ListLeagueSeasons fetches GET /v1/leagues/seasons.
func (c *Client) ListLeagueSeasons(ctx context.Context) (Result, error) {
	return c.GetPublic(ctx, "/v1/leagues/seasons", nil, BodyLimitDefault)
}

// GetLeagueSeason fetches GET /v1/leagues/seasons/:seasonId.
func (c *Client) GetLeagueSeason(ctx context.Context, seasonID string) (Result, error) {
	seasonID = strings.TrimSpace(seasonID)
	if seasonID == "" {
		return Result{}, fmt.Errorf("season id is required")
	}
	path := "/v1/leagues/seasons/" + url.PathEscape(seasonID)
	return c.GetPublic(ctx, path, nil, BodyLimitDefault)
}

// ListLeagues fetches GET /v1/leagues (PascalCase league rows).
func (c *Client) ListLeagues(ctx context.Context, season string, page, pageSize int) (Result, error) {
	q := url.Values{}
	if season != "" {
		q.Set("season", season)
	}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	return c.GetPublic(ctx, "/v1/leagues", q, BodyLimitDefault)
}

// ListLeagueContributors fetches GET /v1/leagues/players (top-500 sample).
func (c *Client) ListLeagueContributors(ctx context.Context, season string) (Result, error) {
	q := url.Values{}
	if season != "" {
		q.Set("season", season)
	}
	return c.GetPublic(ctx, "/v1/leagues/players", q, BodyLimitDefault)
}

// ListClanPlayersAggregate fetches GET /v1/clans/players.
func (c *Client) ListClanPlayersAggregate(ctx context.Context, page, pageSize int) (Result, error) {
	q := url.Values{}
	if page > 0 {
		q.Set("page", strconv.Itoa(page))
	}
	if pageSize > 0 {
		q.Set("pageSize", strconv.Itoa(pageSize))
	}
	return c.GetPublic(ctx, "/v1/clans/players", q, BodyLimitDefault)
}

// GetClanBattle fetches GET /v1/clans/battles/:battleId (camelCase detail).
func (c *Client) GetClanBattle(ctx context.Context, battleID string) (Result, error) {
	battleID = strings.TrimSpace(battleID)
	if battleID == "" {
		return Result{}, fmt.Errorf("battle id is required")
	}
	path := "/v1/clans/battles/" + url.PathEscape(battleID)
	return c.GetPublic(ctx, path, nil, BodyLimitDefault)
}

// GetExists fetches /api/exists.
func (c *Client) GetExists(ctx context.Context, category, id string) (Result, error) {
	q := url.Values{}
	if category != "" {
		q.Set("category", category)
	}
	if id != "" {
		q.Set("id", id)
	}
	return c.GetPublic(ctx, "/api/exists", q, BodyLimitExists)
}
