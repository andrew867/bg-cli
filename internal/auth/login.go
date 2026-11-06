package auth

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/redact"
)

// LoginOptions configure the interactive OAuth login flow.
type LoginOptions struct {
	Profile      string
	ClientID     string
	RedirectURI  string
	Scopes       []string
	NoOpen       bool
	AuthorizeURL string
	TokenURL     string
	HTTPClient   *http.Client
	TokenStore   TokenStore
	SecretSource SecretSource
	Stdout       io.Writer // unused for secrets; reserved
	Stderr       io.Writer
	OpenURL      func(string) error
	Now          func() time.Time
	CallbackWait time.Duration
}

// LoginResult is the non-secret outcome of a successful login.
type LoginResult struct {
	Profile   string
	Scopes    []string
	ExpiresAt time.Time
	IssuedAt  time.Time
}

// Login runs Authorization Code + PKCE against BIG Games (or injected test endpoints).
func Login(ctx context.Context, opt LoginOptions) (LoginResult, *redact.Secrets, error) {
	secrets := &redact.Secrets{}
	if err := config.ValidateAlias(opt.Profile); err != nil {
		return LoginResult{}, secrets, err
	}
	if opt.ClientID == "" {
		return LoginResult{}, secrets, fmt.Errorf("client-id is required")
	}
	if opt.RedirectURI == "" {
		return LoginResult{}, secrets, fmt.Errorf("redirect-uri is required")
	}
	scopes := opt.Scopes
	if len(scopes) == 0 {
		scopes = append([]string{}, DefaultScopes...)
	}
	if err := ValidateScopes(scopes); err != nil {
		return LoginResult{}, secrets, err
	}
	store := opt.TokenStore
	if store == nil {
		store = KeyringStore{}
	}
	stderr := opt.Stderr
	if stderr == nil {
		stderr = io.Discard
	}
	now := opt.Now
	if now == nil {
		now = func() time.Time { return time.Now().UTC() }
	}

	clientSecret, err := opt.SecretSource.ReadClientSecret()
	if err != nil {
		return LoginResult{}, secrets, err
	}
	secrets.Add(clientSecret)
	secrets.AddBasic(opt.ClientID, clientSecret)
	defer func() { clientSecret = "" }()

	pkce, err := GeneratePKCE()
	if err != nil {
		return LoginResult{}, secrets, err
	}
	secrets.Add(pkce.Verifier)
	state, err := GenerateState()
	if err != nil {
		return LoginResult{}, secrets, err
	}

	loop, err := StartLoopback(opt.RedirectURI)
	if err != nil {
		return LoginResult{}, secrets, err
	}
	defer loop.Close()
	if opt.CallbackWait > 0 {
		loop.Timeout = opt.CallbackWait
	}

	// Exact redirect URI must match registration; use the caller-provided URI
	// (including fixed port) rather than rebinding.
	redirectURI := opt.RedirectURI
	authURL, err := BuildAuthorizeURL(AuthorizeParams{
		ClientID:      opt.ClientID,
		RedirectURI:   redirectURI,
		Scopes:        scopes,
		State:         state,
		CodeChallenge: pkce.Challenge,
		AuthorizeURL:  opt.AuthorizeURL,
	})
	if err != nil {
		return LoginResult{}, secrets, err
	}

	openFn := opt.OpenURL
	if openFn == nil {
		openFn = openBrowser
	}
	if opt.NoOpen {
		fmt.Fprintf(stderr, "Open this URL to authorize:\n%s\n", authURL)
	} else if err := openFn(authURL); err != nil {
		fmt.Fprintf(stderr, "Could not open browser (%v). Open this URL to authorize:\n%s\n", err, authURL)
	} else {
		fmt.Fprintf(stderr, "Waiting for authorization callback on %s ...\n", redirectURI)
	}

	cb, err := loop.Wait(ctx, state)
	if err != nil {
		return LoginResult{}, secrets, err
	}
	secrets.Add(cb.Code)

	tr, err := ExchangeCode(ctx, ExchangeParams{
		TokenURL:     opt.TokenURL,
		ClientID:     opt.ClientID,
		ClientSecret: clientSecret,
		Code:         cb.Code,
		RedirectURI:  redirectURI,
		Verifier:     pkce.Verifier,
		HTTPClient:   opt.HTTPClient,
	})
	if err != nil {
		return LoginResult{}, secrets, err
	}
	secrets.Add(tr.AccessToken)

	if err := ValidateGrantedScopes(tr, scopes); err != nil {
		return LoginResult{}, secrets, err
	}

	issued := now()
	expires := issued.Add(time.Duration(tr.ExpiresIn) * time.Second)
	if err := store.Set(opt.Profile, tr.AccessToken); err != nil {
		return LoginResult{}, secrets, err
	}

	cfg, err := config.Load()
	if err != nil {
		_ = store.Delete(opt.Profile)
		return LoginResult{}, secrets, err
	}
	granted := strings.Fields(tr.Scope)
	p := config.Profile{
		Alias:       opt.Profile,
		ClientID:    opt.ClientID,
		Scopes:      granted,
		IssuedAt:    issued,
		ExpiresAt:   expires,
		RedirectURI: redirectURI,
	}
	if err := config.UpsertProfile(cfg, p, cfg.DefaultProfile == "" || cfg.DefaultProfile == opt.Profile); err != nil {
		_ = store.Delete(opt.Profile)
		return LoginResult{}, secrets, err
	}
	if err := config.Save(cfg); err != nil {
		_ = store.Delete(opt.Profile)
		return LoginResult{}, secrets, err
	}

	return LoginResult{
		Profile:   opt.Profile,
		Scopes:    granted,
		ExpiresAt: expires,
		IssuedAt:  issued,
	}, secrets, nil
}

func openBrowser(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	default:
		cmd = exec.Command("xdg-open", rawURL)
	}
	return cmd.Start()
}
