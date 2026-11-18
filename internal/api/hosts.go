// Package api provides HTTP clients for the official BIG Games PS99 API.
package api

import (
	"fmt"
	"net/url"
	"strings"
)

const (
	// ProductionAPIBase is the official PS99 API origin.
	ProductionAPIBase = "https://ps99.biggamesapi.io"
	// ProductionOAuthAuthorize is the browser authorize endpoint.
	ProductionOAuthAuthorize = "https://db.biggames.io/oauth/authorize"
	// ProductionOAuthToken is the token exchange endpoint.
	ProductionOAuthToken = "https://db.biggames.io/oauth/token"

	APIHost   = "ps99.biggamesapi.io"
	OAuthHost = "db.biggames.io"
)

// AccountPaths is the allowlist of authenticated GET paths.
var AccountPaths = map[string]struct{}{
	"/v1/account/inventory":       {},
	"/v1/account/profile":         {},
	"/v1/account/extendedProfile": {},
	"/v1/account/itemIndex":       {},
	"/v1/account/trades":          {},
	"/v1/account/booth":           {},
	"/v1/account/mail":            {},
}

// IsAccountPath reports whether path (without query) is an allowlisted account route.
func IsAccountPath(path string) bool {
	_, ok := AccountPaths[path]
	return ok
}

// ValidateBearerTarget ensures Bearer credentials are only sent to exact account routes.
func ValidateBearerTarget(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("account requests require https")
	}
	if !strings.EqualFold(u.Host, APIHost) {
		return fmt.Errorf("bearer token may only be sent to host %s", APIHost)
	}
	if !IsAccountPath(u.Path) {
		return fmt.Errorf("bearer token may only be sent to /v1/account/* paths")
	}
	return nil
}

// ValidateTokenExchangeURL ensures Basic credentials are only sent to the exact token URL.
func ValidateTokenExchangeURL(rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}
	if !strings.EqualFold(u.Scheme, "https") {
		return fmt.Errorf("token exchange requires https")
	}
	if !strings.EqualFold(u.Host, OAuthHost) {
		return fmt.Errorf("client credentials may only be sent to host %s", OAuthHost)
	}
	if u.Path != "/oauth/token" {
		return fmt.Errorf("client credentials may only be sent to /oauth/token")
	}
	return nil
}

// IsPublicPath reports whether a path is a documented public route prefix.
func IsPublicPath(path string) bool {
	switch {
	case strings.HasPrefix(path, "/v1/players/"):
		return true
	case path == "/v1/leaderboards" || strings.HasPrefix(path, "/v1/leaderboards/"):
		return true
	case path == "/api/collections" || strings.HasPrefix(path, "/api/collection/"):
		return true
	case path == "/api/rap" || path == "/api/exists":
		return true
	case path == "/v1/releases" || strings.HasPrefix(path, "/v1/releases/"):
		return true
	case path == "/v1/items/changes":
		return true
	case strings.HasPrefix(path, "/v1/leagues"):
		return true
	case strings.HasPrefix(path, "/v1/clans/"):
		return true
	default:
		return false
	}
}
