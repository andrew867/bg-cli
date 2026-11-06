package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/cache"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

type publicResult struct {
	Res            api.Result
	FromLocalCache bool
	RequestURL     string
}

func (a *App) getPublicCached(ctx context.Context, path string, query url.Values, limit int64) (publicResult, error) {
	client := a.apiClient()
	base := client.BaseURL
	if base == "" {
		base = api.ProductionAPIBase
	}
	u, err := url.Parse(strings.TrimRight(base, "/") + path)
	if err != nil {
		return publicResult{}, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	rawURL := u.String()
	now := time.Now().UTC()
	if a.Deps.Clock != nil {
		now = a.Deps.Clock.Now()
	}

	store := cache.Store{}
	if !a.Flags.NoCache {
		if st, body, ok, gerr := store.Get(rawURL, now); gerr == nil && ok {
			res := api.Result{HTTPStatus: st, Body: body, Endpoint: path, Headers: http.Header{}}
			var env struct {
				Status  string          `json:"status"`
				Data    json.RawMessage `json:"data"`
				Refresh json.RawMessage `json:"refresh"`
				Error   *struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(body, &env) == nil {
				res.Status = env.Status
				res.Data = env.Data
				res.Refresh = env.Refresh
				if env.Error != nil {
					res.ErrorMsg = env.Error.Message
				}
			}
			return publicResult{Res: res, FromLocalCache: true, RequestURL: rawURL}, nil
		}
	}

	res, err := client.GetPublic(ctx, path, query, limit)
	if err != nil {
		return publicResult{RequestURL: rawURL}, err
	}
	if !a.Flags.NoCache {
		maxAge := cache.ParseMaxAge(res.Headers.Get("Cache-Control"))
		_ = store.Put(rawURL, res.HTTPStatus, res.Body, res.Headers.Get("Content-Type"), maxAge, now)
	}
	return publicResult{Res: res, FromLocalCache: false, RequestURL: rawURL}, nil
}

func (a *App) writePublicSuccess(command, endpoint string, pr publicResult, data any, snapshotAt *time.Time) error {
	retrieved := time.Now().UTC()
	if a.Deps.Clock != nil {
		retrieved = a.Deps.Clock.Now()
	}
	source := &output.Source{
		Provider:       "BIG Games",
		Classification: "official",
		BaseURL:        api.ProductionAPIBase,
		Endpoint:       endpoint,
		HTTPStatus:     pr.Res.HTTPStatus,
		RetrievedAt:    retrieved,
		FromLocalCache: pr.FromLocalCache,
	}
	if a.Flags.Output == "raw" {
		if _, err := a.stdout().Write(pr.Res.Body); err != nil {
			return err
		}
		if len(pr.Res.Body) == 0 || pr.Res.Body[len(pr.Res.Body)-1] != '\n' {
			fmt.Fprintln(a.stdout())
		}
		return nil
	}
	return a.emit(output.NewSuccess(command, source, freshnessFromResult(pr.Res, snapshotAt), data))
}

func (a *App) mapPublicHTTP(command, endpoint string, pr publicResult) error {
	retrieved := time.Now().UTC()
	if a.Deps.Clock != nil {
		retrieved = a.Deps.Clock.Now()
	}
	source := &output.Source{
		Provider:       "BIG Games",
		Classification: "official",
		BaseURL:        api.ProductionAPIBase,
		Endpoint:       endpoint,
		HTTPStatus:     pr.Res.HTTPStatus,
		RetrievedAt:    retrieved,
		FromLocalCache: pr.FromLocalCache,
	}
	st := pr.Res.HTTPStatus
	switch {
	case st == 404:
		msg := pr.Res.ErrorMsg
		if msg == "" {
			msg = "public resource not found"
		}
		_ = a.emit(output.NewError(command, source, output.DefaultFreshnessNull(), &output.ErrorBody{
			Code:           "not_found",
			Message:        msg,
			Retryable:      false,
			UpstreamStatus: &st,
		}))
		return &ExitError{Code: exitcode.NotFound, Msg: msg}
	case st == 429:
		ra := api.ParseRetryAfterSeconds(pr.Res.Headers)
		_ = a.emit(output.NewError(command, source, output.DefaultFreshnessNull(), &output.ErrorBody{
			Code:              "rate_limited",
			Message:           "HTTP 429 rate limited; Retry-After reported without sleeping",
			Retryable:         true,
			UpstreamStatus:    &st,
			RetryAfterSeconds: ra,
		}))
		return &ExitError{Code: exitcode.RateLimit, Msg: "rate limited"}
	case st < 200 || st >= 300:
		msg := pr.Res.ErrorMsg
		if msg == "" {
			msg = fmt.Sprintf("upstream HTTP %d", st)
		}
		_ = a.emit(output.NewError(command, source, output.DefaultFreshnessNull(), &output.ErrorBody{
			Code:           "upstream_error",
			Message:        msg,
			Retryable:      false,
			UpstreamStatus: &st,
		}))
		return &ExitError{Code: exitcode.Upstream, Msg: msg}
	default:
		return nil
	}
}
