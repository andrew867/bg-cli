package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	BodyLimitDefault   = 32 << 20  // 32 MiB
	BodyLimitInventory = 128 << 20 // 128 MiB
	BodyLimitRAP       = 128 << 20
	BodyLimitExists    = 128 << 20
)

// Client talks to the official PS99 API.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	UserAgent  string
}

func (c *Client) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return ProductionAPIBase
}

func (c *Client) http() *http.Client {
	if c.HTTPClient != nil {
		return c.HTTPClient
	}
	return &http.Client{Timeout: 15 * time.Second}
}

// Result is a decoded upstream response plus observed metadata.
type Result struct {
	HTTPStatus int
	Headers    http.Header
	Body       []byte
	Status     string
	Data       json.RawMessage
	Refresh    json.RawMessage
	ErrorMsg   string
	Endpoint   string
}

// RateLimitInfo extracts selected rate-limit headers.
type RateLimitInfo struct {
	Limit             string `json:"limit,omitempty"`
	Remaining         string `json:"remaining,omitempty"`
	RetryAfter        string `json:"retryAfter,omitempty"`
	QuotaExhaustedHdr string `json:"quotaExhaustedHeader,omitempty"`
}

func (r Result) RateLimit() RateLimitInfo {
	h := r.Headers
	return RateLimitInfo{
		Limit:             h.Get("X-RateLimit-Limit"),
		Remaining:         h.Get("X-RateLimit-Remaining"),
		RetryAfter:        h.Get("Retry-After"),
		QuotaExhaustedHdr: h.Get("X-RateLimit-Quota-Exhausted"),
	}
}

type upstreamEnvelope struct {
	Status  string          `json:"status"`
	Data    json.RawMessage `json:"data"`
	Refresh json.RawMessage `json:"refresh"`
	Error   *struct {
		Message string `json:"message"`
		Ignore  bool   `json:"ignore"`
	} `json:"error"`
}

// GetAccount performs an authenticated GET to an allowlisted account path.
// Bearer is attached only after ValidateBearerTarget succeeds.
func (c *Client) GetAccount(ctx context.Context, path string, query url.Values, token string, bodyLimit int64) (Result, error) {
	if bodyLimit <= 0 {
		bodyLimit = BodyLimitDefault
	}
	u, err := url.Parse(c.base() + path)
	if err != nil {
		return Result{}, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	full := u.String()
	if err := ValidateBearerTarget(full); err != nil {
		// Allow loopback test bases that still use account paths.
		if !isLoopbackTestURL(full) || !IsAccountPath(u.Path) {
			return Result{}, err
		}
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, full, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/json")
	if c.UserAgent != "" {
		req.Header.Set("User-Agent", c.UserAgent)
	}

	client := c.http()
	// Account requests must not follow redirects.
	transportClient := *client
	transportClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return fmt.Errorf("account requests must not follow redirects")
	}

	resp, err := transportClient.Do(req)
	if err != nil {
		return Result{Endpoint: path}, fmt.Errorf("account request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, bodyLimit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Result{HTTPStatus: resp.StatusCode, Headers: resp.Header, Endpoint: path}, err
	}
	if int64(len(body)) > bodyLimit {
		return Result{HTTPStatus: resp.StatusCode, Headers: resp.Header, Endpoint: path},
			fmt.Errorf("response body exceeds %d bytes", bodyLimit)
	}

	res := Result{
		HTTPStatus: resp.StatusCode,
		Headers:    resp.Header.Clone(),
		Body:       body,
		Endpoint:   path,
	}

	var env upstreamEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return res, fmt.Errorf("invalid JSON: %w", err)
	}
	res.Status = env.Status
	res.Data = env.Data
	res.Refresh = env.Refresh
	if env.Error != nil {
		res.ErrorMsg = env.Error.Message
	}
	return res, nil
}

// GetPublic performs an unauthenticated GET. Never attaches Authorization.
func (c *Client) GetPublic(ctx context.Context, path string, query url.Values, bodyLimit int64) (Result, error) {
	if bodyLimit <= 0 {
		bodyLimit = BodyLimitDefault
	}
	u, err := url.Parse(c.base() + path)
	if err != nil {
		return Result{}, err
	}
	if query != nil {
		u.RawQuery = query.Encode()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/json")
	// Deliberately no Authorization header.
	resp, err := c.http().Do(req)
	if err != nil {
		return Result{Endpoint: path}, err
	}
	defer resp.Body.Close()
	limited := io.LimitReader(resp.Body, bodyLimit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return Result{HTTPStatus: resp.StatusCode, Headers: resp.Header, Endpoint: path}, err
	}
	if int64(len(body)) > bodyLimit {
		return Result{HTTPStatus: resp.StatusCode, Headers: resp.Header, Endpoint: path},
			fmt.Errorf("response body exceeds %d bytes", bodyLimit)
	}
	res := Result{HTTPStatus: resp.StatusCode, Headers: resp.Header.Clone(), Body: body, Endpoint: path}
	var env upstreamEnvelope
	if err := json.Unmarshal(body, &env); err != nil {
		return res, fmt.Errorf("invalid JSON: %w", err)
	}
	res.Status = env.Status
	res.Data = env.Data
	res.Refresh = env.Refresh
	if env.Error != nil {
		res.ErrorMsg = env.Error.Message
	}
	return res, nil
}

func isLoopbackTestURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := u.Hostname()
	return host == "127.0.0.1" || host == "localhost"
}

// ParseRetryAfterSeconds returns Retry-After as seconds when numeric.
func ParseRetryAfterSeconds(h http.Header) *int {
	v := strings.TrimSpace(h.Get("Retry-After"))
	if v == "" {
		return nil
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < 0 {
		return nil
	}
	return &n
}
