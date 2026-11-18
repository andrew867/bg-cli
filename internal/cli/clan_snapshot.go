package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

type clanSnapState struct {
	ProfileBody     json.RawMessage
	ExtendedBody    json.RawMessage
	ProfileFetched  bool
	ExtendedFetched bool
}

func (a *App) attachClanSnapshot(ctx context.Context, absDir, stamp, collision, prefix, profile string, now time.Time, man *snapshot.Manifest, st clanSnapState, opt snapshotOptions) {
	userID, userOK := a.configuredPS99UserID(opt.UserID)
	var warnings []string
	if !userOK {
		msg := "player user id is not configured; contribution and rank omitted"
		warnings = append(warnings, msg)
		a.notice("%s", msg)
	}
	if len(st.ProfileBody) == 0 && !st.ProfileFetched {
		raw, err := a.fetchAccountData(ctx, "/v1/account/profile", api.ScopeProfile)
		if err != nil {
			a.notice("profile lookup for clan/currencies skipped: %v", err)
		} else {
			st.ProfileBody = raw
		}
	}
	accountID := a.configuredAccountID(st.ProfileBody)
	discovered := ""
	if strings.TrimSpace(opt.ClanTag) == "" {
		var ext json.RawMessage
		if len(st.ExtendedBody) == 0 && !st.ExtendedFetched {
			raw, err := a.fetchAccountData(ctx, "/v1/account/extendedProfile", api.ScopeExtendedProfile)
			if err != nil {
				a.notice("extended profile lookup for clan tag skipped: %v", err)
			} else {
				ext = raw
			}
		} else {
			ext = st.ExtendedBody
		}
		if found, _, ok := clan.ExtractTag(st.ProfileBody, ext); ok {
			discovered = found
		}
	}
	tag, _, _ := clan.ResolveClanTag(opt.ClanTag, discovered, a.configuredPS99ClanTag())

	currencies, currenciesKnown := clan.CurrenciesFromProfile(st.ProfileBody)
	telemetry := map[string]any{}

	var clanRaw json.RawMessage
	clanOK := false
	var clanErr error
	if tag != "" {
		if err := api.ValidateClanTag(tag); err != nil {
			clanErr = err
			a.notice("clan tag rejected: %v", err)
		} else {
			clanName := snapshot.FileName(stamp, collision, prefix, "clan.json")
			clanPath := "/api/clan/" + url.PathEscape(tag)
			pr, err := a.getPublicFresh(ctx, clanPath, nil, api.BodyLimitDefault)
			switch {
			case err != nil:
				clanErr = err
				a.notice("clan fetch failed: %v", err)
				man.Files = append(man.Files, failRec(clanName, "clan-json", err))
			case pr.Res.HTTPStatus < 200 || pr.Res.HTTPStatus >= 300:
				clanErr = fmt.Errorf("clan HTTP %d", pr.Res.HTTPStatus)
				a.notice("%s", clanErr.Error())
				man.Files = append(man.Files, failRec(clanName, "clan-json", clanErr))
			case len(pr.Res.Data) == 0 || string(pr.Res.Data) == "null":
				clanErr = fmt.Errorf("clan payload is empty")
				a.notice("%s", clanErr.Error())
				man.Files = append(man.Files, failRec(clanName, "clan-json", clanErr))
			default:
				clanRaw = append(json.RawMessage(nil), pr.Res.Data...)
				clanOK = true
				source := publicSource(clanPath, pr, now)
				env := output.NewSuccess("account snapshot clan", source, output.DefaultFreshnessNull(), map[string]any{
					"tag":  tag,
					"clan": clanRaw,
				})
				rec := a.writePayload(absDir, clanName, "clan-json", env, env)
				if rec.OK {
					rec.ContentStatus = snapshot.ContentOK
				}
				man.Files = append(man.Files, rec)
				if !rec.OK {
					man.OK = false
					clanOK = false
				}
			}
		}
	} else {
		clanErr = fmt.Errorf("clan tag is absent from profile, extended profile, and config ps99ClanTag")
		a.notice("clan tag is absent from profile, extended profile, and config ps99ClanTag; clan.json omitted")
	}
	switch {
	case clanErr != nil:
		telemetry["clan"] = "unavailable"
		telemetry["clanError"] = clanErr.Error()
		warnings = append(warnings, "clan telemetry unavailable: "+clanErr.Error())
		if opt.RequireClan {
			man.OK = false
		}
	case !clanOK:
		telemetry["clan"] = "unavailable"
		telemetry["clanError"] = "clan file was not saved"
		warnings = append(warnings, "clan telemetry unavailable: clan file was not saved")
	default:
		telemetry["clan"] = "ok"
	}

	battleName := snapshot.FileName(stamp, collision, prefix, "clanbattle.json")
	var battleRaw json.RawMessage
	battleFetched := false
	var battleErr error
	var battlePR publicResult
	const battlePath = "/api/activeClanBattle"
	bpr, berr := a.getPublicFresh(ctx, battlePath, nil, api.BodyLimitDefault)
	battlePR = bpr
	switch {
	case berr != nil:
		battleErr = berr
		a.notice("active clan battle fetch failed: %v", berr)
		man.Files = append(man.Files, failRec(battleName, "clan-battle-json", berr))
	case bpr.Res.HTTPStatus < 200 || bpr.Res.HTTPStatus >= 300:
		battleErr = fmt.Errorf("active clan battle HTTP %d", bpr.Res.HTTPStatus)
		a.notice("%s", battleErr.Error())
		man.Files = append(man.Files, failRec(battleName, "clan-battle-json", battleErr))
	default:
		battleFetched = true
		if len(bpr.Res.Data) > 0 && string(bpr.Res.Data) != "null" {
			battleRaw = append(json.RawMessage(nil), bpr.Res.Data...)
		}
	}
	if battleErr != nil {
		telemetry["activeBattle"] = "unavailable"
		telemetry["activeBattleError"] = battleErr.Error()
		warnings = append(warnings, "active battle telemetry unavailable: "+battleErr.Error())
		if opt.RequireClan {
			man.OK = false
		}
	} else {
		telemetry["activeBattle"] = "ok"
	}

	observed := clan.Observe(now, userID, tag, clanRaw, battleRaw, clanOK, battleFetched)
	if observed.ActiveBattle != nil && battleFetched {
		source := publicSource(battlePath, battlePR, now)
		env := output.NewSuccess("account snapshot clan battle", source, output.DefaultFreshnessNull(), map[string]any{
			"active": true,
			"battle": battleRaw,
		})
		rec := a.writePayload(absDir, battleName, "clan-battle-json", env, env)
		if rec.OK {
			rec.ContentStatus = snapshot.ContentOK
		}
		man.Files = append(man.Files, rec)
		if !rec.OK {
			man.OK = false
		}
	}

	sumName := snapshot.FileName(stamp, collision, prefix, "summary.json")
	scanned, scanErr := clan.Scan(absDir, prefix, now, sumName)
	warnings = append(warnings, scanned.Warnings...)
	if scanErr != nil {
		warnings = append(warnings, fmt.Sprintf("summary scan failed: %v", scanErr))
		a.notice("summary scan failed: %v", scanErr)
	}
	for _, w := range scanned.Warnings {
		a.notice("summary history: %s", w)
	}
	sessionID := opt.SessionID
	switch {
	case sessionID != "":
	case opt.NewSession:
		sessionID = clan.NewSessionID(prefix, stamp)
		if collision != "" {
			sessionID += "-" + collision
		}
	default:
		sessionID = clan.MatchingSession(scanned.Priors, accountID, profile)
		if sessionID == "" {
			sessionID = clan.NewSessionID(prefix, stamp)
		}
	}
	sum := clan.Build(clan.Input{
		Now:             now,
		LocalStamp:      stamp,
		CollisionSuffix: collision,
		Prefix:          prefix,
		Profile:         profile,
		AccountID:       accountID,
		UserID:          userID,
		SessionID:       sessionID,
		Currencies:      currencies,
		CurrenciesKnown: currenciesKnown,
		Clan:            observed,
		Priors:          scanned.Priors,
		Warnings:        warnings,
	})
	rec := a.writePayloadRaw(absDir, sumName, "snapshot-summary", func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(sum)
	}, output.Envelope{})
	if rec.OK {
		rec.ContentStatus = snapshot.ContentOK
	}
	man.Files = append(man.Files, rec)
	if !rec.OK {
		man.OK = false
	}
	man.Summary["sessionId"] = sum.SessionID
	if sum.AccountID != "" {
		man.Summary["accountId"] = sum.AccountID
	}
	if sum.UserID != "" {
		man.Summary["userId"] = sum.UserID
	}
	man.Summary["clanTelemetry"] = telemetry
	man.Summary["clan"] = sum.Current.Clan
	man.Summary["windows"] = sum.Windows
	fmt.Fprint(a.stderr(), clan.FormatConsole(sum))
}

func publicSource(endpoint string, pr publicResult, now time.Time) *output.Source {
	return &output.Source{
		Provider:       "BIG Games",
		Classification: "official",
		BaseURL:        api.ProductionAPIBase,
		Endpoint:       endpoint,
		HTTPStatus:     pr.Res.HTTPStatus,
		RetrievedAt:    now,
		FromLocalCache: pr.FromLocalCache,
	}
}
