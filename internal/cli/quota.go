package cli

import (
	"encoding/json"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func (a *App) accountSource(res api.Result) *output.Source {
	retrieved := time.Now().UTC()
	if a.Deps.Clock != nil {
		retrieved = a.Deps.Clock.Now()
	} else if a.Deps.Now != nil {
		retrieved = a.Deps.Now()
	}
	return &output.Source{
		Provider:       "BIG Games",
		Classification: "official",
		BaseURL:        api.ProductionAPIBase,
		Endpoint:       res.Endpoint,
		HTTPStatus:     res.HTTPStatus,
		RetrievedAt:    retrieved,
		FromLocalCache: false,
	}
}

func newAccountQuotaCmd(app *App) *cobra.Command {
	var af accountFlags
	cmd := &cobra.Command{
		Use:   "quota",
		Short: "Summarize account refresh quota and rate-limit headers",
		Long: `Performs one authenticated GET /v1/account/profile (profile:read) and reports
only refresh quota fields plus rate-limit / Retry-After headers. Does not print
the profile body. Default call does not request refresh=true.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := app.confirmRefresh(af, "account quota"); err != nil {
				return err
			}
			_, tok, secrets, err := app.resolveProfileToken(api.ScopeProfile)
			if err != nil {
				msg := err.Error()
				code := exitcode.AuthRequired
				if ee, ok := err.(*ExitError); ok {
					code = ee.Code
					msg = ee.Msg
				}
				_ = app.emit(output.NewError("account quota", nil, output.DefaultFreshnessNull(), &output.ErrorBody{
					Code: "auth_required", Message: msg, Retryable: false,
				}))
				return &ExitError{Code: code, Msg: msg}
			}
			defer func() { tok = "" }()

			client := app.apiClient()
			res, err := client.GetProfile(cmd.Context(), tok, api.AccountGetOptions{Refresh: af.refresh})
			if err != nil {
				msg := secrets.Redact(err.Error())
				_ = app.emit(output.NewError("account quota", nil, output.DefaultFreshnessNull(), &output.ErrorBody{
					Code: "network_error", Message: msg, Retryable: true,
				}))
				return &ExitError{Code: exitcode.Network, Msg: msg}
			}
			if res.HTTPStatus == 429 {
				rl := res.RateLimit()
				_ = app.emit(output.NewError("account quota", app.accountSource(res), freshnessFromResult(res, nil), &output.ErrorBody{
					Code:      "rate_limited",
					Message:   "HTTP 429 rate limited; Retry-After reported without sleeping",
					Retryable: true,
					Details: map[string]any{
						"rateLimit":  rl,
						"retryAfter": rl.RetryAfter,
					},
				}))
				return &ExitError{Code: exitcode.RateLimit, Msg: "rate limited"}
			}
			if res.HTTPStatus == 401 || res.HTTPStatus == 403 {
				_ = app.emit(output.NewError("account quota", app.accountSource(res), freshnessFromResult(res, nil), &output.ErrorBody{
					Code: "auth_required", Message: "unauthorized or forbidden", Retryable: false,
				}))
				return &ExitError{Code: exitcode.AuthRequired, Msg: "unauthorized or forbidden"}
			}
			if res.HTTPStatus >= 400 {
				_ = app.emit(output.NewError("account quota", app.accountSource(res), freshnessFromResult(res, nil), &output.ErrorBody{
					Code: "upstream_error", Message: "upstream HTTP error", Retryable: res.HTTPStatus >= 500,
				}))
				return &ExitError{Code: exitcode.Upstream, Msg: "upstream HTTP error"}
			}

			summary := quotaSummary(res)
			return app.emit(output.NewSuccess("account quota", app.accountSource(res), freshnessFromResult(res, nil), summary))
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	return cmd
}

func quotaSummary(res api.Result) map[string]any {
	out := map[string]any{
		"httpStatus": res.HTTPStatus,
		"rateLimit":  res.RateLimit(),
	}
	rl := res.RateLimit()
	out["limit"] = emptyToNil(rl.Limit)
	out["remaining"] = emptyToNil(rl.Remaining)
	out["retryAfter"] = emptyToNil(rl.RetryAfter)
	if rl.QuotaExhaustedHdr != "" {
		out["quotaExhaustedHeader"] = rl.QuotaExhaustedHdr
	}

	if len(res.Refresh) == 0 || string(res.Refresh) == "null" {
		out["refreshPresent"] = false
		return out
	}
	out["refreshPresent"] = true
	dec := json.NewDecoder(strings.NewReader(string(res.Refresh)))
	dec.UseNumber()
	var refresh map[string]any
	if err := dec.Decode(&refresh); err != nil {
		out["refreshParseError"] = err.Error()
		return out
	}
	for _, k := range []string{
		"used", "limit", "resetsAt", "nextRefreshEligibleAt",
		"quotaExhausted", "consumedThisCall", "skipped",
	} {
		if v, ok := refresh[k]; ok {
			// Prefer refresh.limit over header limit when present.
			out[k] = v
		}
	}
	out["refresh"] = refresh
	return out
}

func emptyToNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}
