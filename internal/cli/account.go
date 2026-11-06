package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/zalando/go-keyring"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/jsonutil"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/redact"
)

func newAccountCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "account",
		Short: "Read private account data for the authorizing profile",
		Long: `Account commands call official GET /v1/account/* endpoints using the access
token stored for --profile. One token reads only the Roblox account that
authorized that profile. Data is a cached snapshot, not real time.

This client is unofficial and not endorsed by BIG Games.`,
	}
	cmd.AddCommand(newAccountInventoryCmd(app))
	cmd.AddCommand(newAccountProfileCmd(app))
	cmd.AddCommand(newAccountCurrenciesCmd(app))
	cmd.AddCommand(newAccountPassthroughCmd(app, "extended-profile", "extendedProfile", api.ScopeExtendedProfile, "Fetch authorizing account extended profile"))
	cmd.AddCommand(newAccountPassthroughCmd(app, "item-index", "itemIndex", api.ScopeItemIndex, "Fetch authorizing account item index"))
	cmd.AddCommand(newAccountPassthroughCmd(app, "trades", "trades", api.ScopeTrades, "Fetch authorizing account trade history"))
	cmd.AddCommand(newAccountPassthroughCmd(app, "booth", "booth", api.ScopeBooth, "Fetch authorizing account booth history"))
	cmd.AddCommand(newAccountPassthroughCmd(app, "mail", "mail", api.ScopeMail, "Fetch authorizing account mail history"))
	return cmd
}

type accountFlags struct {
	refresh bool
	yes     bool
}

func (a *App) apiClient() *api.Client {
	c := &api.Client{UserAgent: "bg-cli/" + Version}
	if a.Deps.BaseURL != "" {
		c.BaseURL = a.Deps.BaseURL
	}
	if a.Deps.HTTPClient != nil {
		c.HTTPClient = a.Deps.HTTPClient
	} else {
		c.HTTPClient = &http.Client{Timeout: a.Flags.Timeout}
	}
	return c
}

func (a *App) resolveProfileToken(requiredScope string) (config.Profile, string, *redact.Secrets, error) {
	secrets := &redact.Secrets{}
	cfg, err := config.Load()
	if err != nil {
		return config.Profile{}, "", secrets, &ExitError{Code: exitcode.Usage, Msg: err.Error()}
	}
	alias := a.Flags.Profile
	if alias == "" {
		alias = cfg.DefaultProfile
	}
	if alias == "" {
		return config.Profile{}, "", secrets, &ExitError{Code: exitcode.AuthRequired, Msg: "not logged in; run auth login"}
	}
	p, ok := config.Get(cfg, alias)
	if !ok {
		return config.Profile{}, "", secrets, &ExitError{Code: exitcode.AuthRequired, Msg: fmt.Sprintf("profile %q not found; run auth login", alias)}
	}
	if requiredScope != "" && !containsScope(p.Scopes, requiredScope) {
		return p, "", secrets, &ExitError{
			Code: exitcode.AuthRequired,
			Msg:  fmt.Sprintf("profile %q missing scope %s; run auth login again (defaults to all account scopes)", alias, requiredScope),
		}
	}
	tok, err := a.tokenStore().Get(alias)
	if err != nil {
		if err == keyring.ErrNotFound || strings.Contains(strings.ToLower(err.Error()), "not found") {
			return p, "", secrets, &ExitError{Code: exitcode.AuthRequired, Msg: fmt.Sprintf("no access token for profile %q; run auth login", alias)}
		}
		return p, "", secrets, &ExitError{Code: exitcode.CredentialStore, Msg: err.Error()}
	}
	secrets.Add(tok)
	return p, tok, secrets, nil
}

func containsScope(scopes []string, want string) bool {
	for _, s := range scopes {
		if s == want {
			return true
		}
	}
	return false
}

func (a *App) confirmRefresh(af accountFlags, command string) error {
	if !af.refresh {
		return nil
	}
	a.notice("Warning: --refresh may consume daily refresh quota and cannot bypass the daily limit.")
	isTTY := false
	if a.Deps.IsTTY != nil {
		isTTY = a.Deps.IsTTY()
	} else {
		isTTY = isStdinTTY()
	}
	if !isTTY && !af.yes {
		return &ExitError{Code: exitcode.Usage, Msg: "--refresh requires --yes in non-interactive mode"}
	}
	_ = command
	return nil
}

func isStdinTTY() bool {
	fi, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (fi.Mode() & os.ModeCharDevice) != 0
}

func newAccountInventoryCmd(app *App) *cobra.Command {
	var af accountFlags
	var class, id string
	var equippedOnly bool
	cmd := &cobra.Command{
		Use:   "inventory",
		Short: "Fetch authorizing account inventory snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runAccountView(cmd.Context(), accountViewRequest{
				Command:       "account inventory",
				View:          "inventory",
				RequiredScope: api.ScopeInventory,
				Flags:         af,
				Class:         class,
				ID:            id,
				EquippedOnly:  equippedOnly,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	cmd.Flags().StringVar(&class, "class", "", "exact case-insensitive item class filter")
	cmd.Flags().StringVar(&id, "id", "", "exact case-insensitive item id filter")
	cmd.Flags().BoolVar(&equippedOnly, "equipped-only", false, "keep items referenced in equipped payload only")
	return cmd
}

func newAccountProfileCmd(app *App) *cobra.Command {
	var af accountFlags
	cmd := &cobra.Command{
		Use:   "profile",
		Short: "Fetch authorizing account profile snapshot",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runAccountView(cmd.Context(), accountViewRequest{
				Command:       "account profile",
				View:          "profile",
				RequiredScope: api.ScopeProfile,
				Flags:         af,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	return cmd
}

func newAccountCurrenciesCmd(app *App) *cobra.Command {
	var af accountFlags
	var id string
	cmd := &cobra.Command{
		Use:   "currencies",
		Short: "Extract Currency map from authorizing account profile",
		Long: `Reads /v1/account/profile and returns only data.Currency entries.
Missing Currency is reported with currencyFieldPresent=false (not invented zeroes).`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runAccountView(cmd.Context(), accountViewRequest{
				Command:       "account currencies",
				View:          "currencies",
				RequiredScope: api.ScopeProfile,
				Flags:         af,
				ID:            id,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	cmd.Flags().StringVar(&id, "id", "", "exact case-insensitive currency id / map key filter")
	return cmd
}

func newAccountPassthroughCmd(app *App, use, view, scope, short string) *cobra.Command {
	var af accountFlags
	cmd := &cobra.Command{
		Use:   use,
		Short: short,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runAccountView(cmd.Context(), accountViewRequest{
				Command:       "account " + use,
				View:          view,
				RequiredScope: scope,
				Flags:         af,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	return cmd
}

type accountViewRequest struct {
	Command       string
	View          string
	RequiredScope string
	Flags         accountFlags
	Class         string
	ID            string
	EquippedOnly  bool
}

func (a *App) runAccountView(ctx context.Context, req accountViewRequest) error {
	if err := a.confirmRefresh(req.Flags, req.Command); err != nil {
		return err
	}
	_, tok, secrets, err := a.resolveProfileToken(req.RequiredScope)
	if err != nil {
		msg := err.Error()
		code := exitcode.AuthRequired
		if ee, ok := err.(*ExitError); ok {
			code = ee.Code
			msg = ee.Msg
		}
		_ = a.emit(output.NewError(req.Command, nil, output.DefaultFreshnessNull(), &output.ErrorBody{
			Code:      "auth_required",
			Message:   msg,
			Retryable: false,
		}))
		return &ExitError{Code: code, Msg: msg}
	}
	defer func() { tok = "" }()

	client := a.apiClient()
	opt := api.AccountGetOptions{Refresh: req.Flags.refresh}

	var res api.Result
	path := "/v1/account/profile"
	switch req.View {
	case "inventory":
		path = "/v1/account/inventory"
		res, err = client.GetInventory(ctx, tok, opt)
	case "profile", "currencies":
		path = "/v1/account/profile"
		res, err = client.GetProfile(ctx, tok, opt)
	default:
		p, scope, ok := api.PathForView(req.View)
		if !ok {
			return &ExitError{Code: exitcode.Usage, Msg: "unknown account view"}
		}
		if scope != req.RequiredScope {
			return &ExitError{Code: exitcode.Usage, Msg: "scope mismatch for account view"}
		}
		path = p
		res, err = client.GetAccountView(ctx, path, tok, opt)
	}
	if err != nil {
		msg := secrets.Redact(err.Error())
		code := exitcode.Network
		if strings.Contains(msg, "invalid JSON") {
			code = exitcode.Upstream
		}
		_ = a.emit(output.NewError(req.Command, nil, output.DefaultFreshnessNull(), &output.ErrorBody{
			Code:      "request_failed",
			Message:   msg,
			Retryable: false,
		}))
		return &ExitError{Code: code, Msg: msg}
	}

	retrieved := time.Now().UTC()
	if a.Deps.Clock != nil {
		retrieved = a.Deps.Clock.Now()
	} else if a.Deps.Now != nil {
		retrieved = a.Deps.Now()
	}

	source := &output.Source{
		Provider:       "BIG Games",
		Classification: "official",
		BaseURL:        api.ProductionAPIBase,
		Endpoint:       path,
		HTTPStatus:     res.HTTPStatus,
		RetrievedAt:    retrieved,
		FromLocalCache: false,
	}
	if client.BaseURL != "" && client.BaseURL != api.ProductionAPIBase {
		// Keep schema const for production base in public output; tests may still assert endpoint.
		source.BaseURL = api.ProductionAPIBase
	}

	if res.HTTPStatus == 401 {
		msg := res.ErrorMsg
		if msg == "" {
			msg = "unauthorized; token missing, expired, or revoked — run auth login"
		}
		msg = secrets.Redact(msg)
		st := res.HTTPStatus
		_ = a.emit(output.NewError(req.Command, source, freshnessFromResult(res, nil), &output.ErrorBody{
			Code:           "unauthorized",
			Message:        msg,
			Retryable:      false,
			UpstreamStatus: &st,
		}))
		return &ExitError{Code: exitcode.AuthRequired, Msg: msg}
	}
	if res.HTTPStatus == 403 {
		msg := res.ErrorMsg
		if msg == "" {
			msg = "forbidden; missing scope — re-authorize with the required scope"
		}
		st := res.HTTPStatus
		_ = a.emit(output.NewError(req.Command, source, freshnessFromResult(res, nil), &output.ErrorBody{
			Code:           "forbidden",
			Message:        secrets.Redact(msg),
			Retryable:      false,
			UpstreamStatus: &st,
		}))
		return &ExitError{Code: exitcode.AuthRequired, Msg: msg}
	}
	if res.HTTPStatus == 429 {
		st := res.HTTPStatus
		ra := api.ParseRetryAfterSeconds(res.Headers)
		_ = a.emit(output.NewError(req.Command, source, freshnessFromResult(res, nil), &output.ErrorBody{
			Code:              "rate_limited",
			Message:           "HTTP 429 rate limited; Retry-After reported without sleeping",
			Retryable:         true,
			UpstreamStatus:    &st,
			RetryAfterSeconds: ra,
			Details:           map[string]any{"rateLimit": res.RateLimit()},
		}))
		return &ExitError{Code: exitcode.RateLimit, Msg: "rate limited"}
	}
	if res.HTTPStatus < 200 || res.HTTPStatus >= 300 {
		st := res.HTTPStatus
		msg := res.ErrorMsg
		if msg == "" {
			msg = fmt.Sprintf("upstream HTTP %d", res.HTTPStatus)
		}
		_ = a.emit(output.NewError(req.Command, source, freshnessFromResult(res, nil), &output.ErrorBody{
			Code:           "upstream_error",
			Message:        secrets.Redact(msg),
			Retryable:      false,
			UpstreamStatus: &st,
		}))
		return &ExitError{Code: exitcode.Upstream, Msg: msg}
	}

	if a.Flags.Output == "raw" {
		if _, err := a.stdout().Write(res.Body); err != nil {
			return err
		}
		if len(res.Body) == 0 || res.Body[len(res.Body)-1] != '\n' {
			fmt.Fprintln(a.stdout())
		}
		return nil
	}

	switch req.View {
	case "inventory":
		filtered, rowCount, qtySum, ferr := api.FilterInventory(res.Data, req.Class, req.ID, req.EquippedOnly)
		if ferr != nil {
			return &ExitError{Code: exitcode.Upstream, Msg: ferr.Error()}
		}
		snap, cached := api.SnapshotFromInventory(res.Data)
		fresh := freshnessFromResult(res, snap)
		data := map[string]any{
			"inventory":      jsonutil.CloneRaw(filtered),
			"rowCount":       rowCount,
			"quantitySum":    qtySum,
			"rowCountNote":   "rowCount is filtered item rows, not total item quantity",
			"cached":         cached,
			"rateLimit":      res.RateLimit(),
			"quotaExhausted": refreshQuotaExhausted(res.Refresh),
		}
		return a.emit(output.NewSuccess(req.Command, source, fresh, data))
	case "profile":
		data := map[string]any{
			"profile":        jsonutil.CloneRaw(res.Data),
			"rateLimit":      res.RateLimit(),
			"quotaExhausted": refreshQuotaExhausted(res.Refresh),
		}
		return a.emit(output.NewSuccess(req.Command, source, freshnessFromResult(res, nil), data))
	case "currencies":
		ext, cerr := api.ExtractCurrencies(res.Data)
		if cerr != nil {
			return &ExitError{Code: exitcode.Upstream, Msg: cerr.Error()}
		}
		ext = api.FilterCurrenciesByID(ext, req.ID)
		data := map[string]any{
			"currencyFieldPresent": ext.CurrencyFieldPresent,
			"currencies":           ext.Currencies,
			"entryCount":           ext.EntryCount,
			"rateLimit":            res.RateLimit(),
			"quotaExhausted":       refreshQuotaExhausted(res.Refresh),
		}
		if refreshQuotaExhausted(res.Refresh) {
			a.notice("quotaExhausted: true — upstream returned latest snapshot with HTTP 200")
		}
		return a.emit(output.NewSuccess(req.Command, source, freshnessFromResult(res, nil), data))
	default:
		data := map[string]any{
			"data":           jsonutil.CloneRaw(res.Data),
			"rateLimit":      res.RateLimit(),
			"quotaExhausted": refreshQuotaExhausted(res.Refresh),
		}
		if refreshQuotaExhausted(res.Refresh) {
			a.notice("quotaExhausted: true — upstream returned latest snapshot with HTTP 200")
		}
		return a.emit(output.NewSuccess(req.Command, source, freshnessFromResult(res, nil), data))
	}
}

func freshnessFromResult(res api.Result, snapshotAt *time.Time) *output.Freshness {
	var refresh any
	if len(res.Refresh) > 0 && string(res.Refresh) != "null" {
		refresh = jsonutil.CloneRaw(res.Refresh)
	}
	return &output.Freshness{
		SnapshotAt: snapshotAt,
		IsStale:    nil,
		Meaning:    output.FreshnessMeaning,
		Refresh:    refresh,
	}
}

func refreshQuotaExhausted(raw json.RawMessage) bool {
	if len(raw) == 0 {
		return false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false
	}
	v, ok := m["quotaExhausted"]
	if !ok {
		return false
	}
	b, ok := v.(bool)
	return ok && b
}
