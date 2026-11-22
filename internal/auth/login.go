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
	// Discover reads the account that authorized this grant. It must use the
	// new access token only against an OAuth account route. A nil Discover
	// means that identity is unavailable. Errors are reported without storing
	// a token on a conflicting alias; a failed read keeps the named profile.
	Discover func(ctx context.Context, accessToken string) (config.GrantIdentity, error)
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
	discovered := config.GrantIdentity{}
	if opt.Discover != nil {
		id, derr := opt.Discover(ctx, tr.AccessToken)
		if derr != nil {
			fmt.Fprintf(stderr, "The account profile did not identify this grant. The token stays on the profile you named.\n")
		} else {
			discovered = id
			if strings.TrimSpace(id.Warning) != "" {
				fmt.Fprintf(stderr, "%s\n", id.Warning)
			}
		}
	}

	cfg, err := config.Load()
	if err != nil {
		return LoginResult{}, secrets, err
	}
	target, notice, err := config.ReconcileGrant(cfg, opt.Profile, discovered)
	if err != nil {
		return LoginResult{}, secrets, err
	}
	if notice != "" {
		fmt.Fprintf(stderr, "%s\n", notice)
	}
	granted := strings.Fields(tr.Scope)
	p := config.Profile{
		Alias:       target,
		ClientID:    opt.ClientID,
		Scopes:      granted,
		IssuedAt:    issued,
		ExpiresAt:   expires,
		RedirectURI: redirectURI,
	}
	if prev, ok := config.Get(cfg, target); ok {
		p = config.PreserveProfileIdentity(p, prev)
		if kept := keptIdentityFields(prev); len(kept) > 0 {
			fmt.Fprintf(stderr, "Kept %s for profile %s\n", formatKeptList(kept), target)
		}
	}
	p, err = config.ApplyDiscovered(p, discovered)
	if err != nil {
		return LoginResult{}, secrets, err
	}
	if err := store.Set(target, tr.AccessToken); err != nil {
		return LoginResult{}, secrets, err
	}
	makeDefault := cfg.DefaultProfile == "" || cfg.DefaultProfile == opt.Profile || cfg.DefaultProfile == target
	if err := config.UpsertProfile(cfg, p, makeDefault); err != nil {
		_ = store.Delete(target)
		return LoginResult{}, secrets, err
	}
	if err := config.Save(cfg); err != nil {
		_ = store.Delete(target)
		return LoginResult{}, secrets, err
	}

	return LoginResult{
		Profile:   target,
		Scopes:    granted,
		ExpiresAt: expires,
		IssuedAt:  issued,
	}, secrets, nil
}

func keptIdentityFields(p config.Profile) []string {
	var kept []string
	if strings.TrimSpace(p.AccountID) != "" {
		kept = append(kept, "accountId")
	}
	if strings.TrimSpace(p.PS99UserID) != "" {
		kept = append(kept, "ps99UserId")
	}
	if strings.TrimSpace(p.PS99ClanTag) != "" {
		kept = append(kept, "ps99ClanTag")
	}
	if strings.TrimSpace(p.LinkedAccountID) != "" {
		kept = append(kept, "linkedAccountId")
	}
	return kept
}

func formatKeptList(fields []string) string {
	switch len(fields) {
	case 0:
		return ""
	case 1:
		return fields[0]
	case 2:
		return fields[0] + " and " + fields[1]
	default:
		return strings.Join(fields[:len(fields)-1], ", ") + ", and " + fields[len(fields)-1]
	}
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
