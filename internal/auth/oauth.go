package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/api"
)

const (
	tokenBodyLimit  = 1 << 20 // 1 MiB
	tokenTypeBearer = "Bearer"
)

// TokenResponse is the OAuth token endpoint JSON body.
type TokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	ExpiresIn   int64  `json:"expires_in"`
	Scope       string `json:"scope"`
	Error       string `json:"error"`
	ErrorDesc   string `json:"error_description"`
}

// AuthorizeParams builds the browser authorize URL.
type AuthorizeParams struct {
	ClientID      string
	RedirectURI   string
	Scopes        []string
	State         string
	CodeChallenge string
	AuthorizeURL  string // tests may override; production uses ProductionOAuthAuthorize
}

// BuildAuthorizeURL constructs the BIG Games authorize URL.
func BuildAuthorizeURL(p AuthorizeParams) (string, error) {
	base := p.AuthorizeURL
	if base == "" {
		base = api.ProductionOAuthAuthorize
	}
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	q := u.Query()
	q.Set("client_id", p.ClientID)
	q.Set("redirect_uri", p.RedirectURI)
	q.Set("scope", ScopeString(p.Scopes))
	q.Set("code_challenge", p.CodeChallenge)
	q.Set("code_challenge_method", "S256")
	q.Set("state", p.State)
	q.Set("response_type", "code")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// ExchangeParams are inputs for the one-time token exchange.
type ExchangeParams struct {
	TokenURL     string // tests inject; production must be ProductionOAuthToken
	ClientID     string
	ClientSecret string
	Code         string
	RedirectURI  string
	Verifier     string
	HTTPClient   *http.Client
}

// ExchangeCode performs a single token exchange with no redirects and no retries.
func ExchangeCode(ctx context.Context, p ExchangeParams) (TokenResponse, error) {
	tokenURL := p.TokenURL
	if tokenURL == "" {
		tokenURL = api.ProductionOAuthToken
	}
	if err := api.ValidateTokenExchangeURL(tokenURL); err != nil {
		// Allow http://127.0.0.1 test servers only when explicitly injected and host is loopback.
		u, parseErr := url.Parse(tokenURL)
		if parseErr != nil || (u.Hostname() != "127.0.0.1" && u.Hostname() != "localhost") {
			return TokenResponse{}, err
		}
	}

	form := url.Values{}
	form.Set("grant_type", "authorization_code")
	form.Set("code", p.Code)
	form.Set("redirect_uri", p.RedirectURI)
	form.Set("code_verifier", p.Verifier)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return TokenResponse{}, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.SetBasicAuth(p.ClientID, p.ClientSecret)

	client := p.HTTPClient
	if client == nil {
		client = &http.Client{
			Timeout: 30 * time.Second,
			CheckRedirect: func(req *http.Request, via []*http.Request) error {
				return fmt.Errorf("token exchange must not follow redirects")
			},
		}
	} else if client.CheckRedirect == nil {
		c := *client
		c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
			return fmt.Errorf("token exchange must not follow redirects")
		}
		client = &c
	}

	resp, err := client.Do(req)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("token exchange request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, tokenBodyLimit+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		return TokenResponse{}, fmt.Errorf("read token response: %w", err)
	}
	if len(body) > tokenBodyLimit {
		return TokenResponse{}, fmt.Errorf("token response exceeds %d bytes", tokenBodyLimit)
	}

	var tr TokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return TokenResponse{}, fmt.Errorf("malformed token response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		msg := tr.Error
		if tr.ErrorDesc != "" {
			msg = tr.Error + ": " + tr.ErrorDesc
		}
		if msg == "" {
			msg = fmt.Sprintf("token endpoint returned HTTP %d", resp.StatusCode)
		}
		return tr, fmt.Errorf("token exchange failed: %s", msg)
	}
	if !strings.EqualFold(tr.TokenType, tokenTypeBearer) {
		return tr, fmt.Errorf("unexpected token_type %q", tr.TokenType)
	}
	if tr.ExpiresIn <= 0 {
		return tr, fmt.Errorf("expires_in must be positive")
	}
	if tr.AccessToken == "" {
		return tr, fmt.Errorf("token response missing access_token")
	}
	return tr, nil
}

// ValidateGrantedScopes ensures the token includes every requested scope.
func ValidateGrantedScopes(tr TokenResponse, requested []string) error {
	missing := MissingScopes(tr.Scope, requested)
	if len(missing) > 0 {
		return fmt.Errorf("token missing required scopes: %s", strings.Join(missing, ", "))
	}
	return nil
}
