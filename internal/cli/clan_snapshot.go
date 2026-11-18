package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
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

func (a *App) attachClanSnapshot(ctx context.Context, absDir, stamp, collision, prefix, profile string, now time.Time, man *snapshot.Manifest, st clanSnapState, clanTag, userID string) {
	userID = a.configuredPS99UserID(userID)
	if len(st.ProfileBody) == 0 && !st.ProfileFetched {
		raw, err := a.fetchAccountData(ctx, "/v1/account/profile", api.ScopeProfile)
		if err != nil {
			a.notice("profile lookup for clan/currencies skipped: %v", err)
		} else {
			st.ProfileBody = raw
		}
	}
	tag := clanTag
	tagField := "--clan-tag"
	if tag == "" {
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
		if found, field, ok := clan.ExtractTag(st.ProfileBody, ext); ok {
			tag = found
			tagField = field
		}
	}
	_ = tagField

	currencies, currenciesKnown := clan.CurrenciesFromProfile(st.ProfileBody)

	var clanRaw json.RawMessage
	clanOK := false
	if tag != "" {
		if err := api.ValidateClanTag(tag); err != nil {
			a.notice("clan tag rejected: %v", err)
		} else {
			clanName := snapshot.FileName(stamp, collision, prefix, "clan.json")
			path := "/api/clan/" + url.PathEscape(tag)
			pr, err := a.getPublicCached(ctx, path, nil, api.BodyLimitDefault)
			if err != nil {
				man.OK = false
				a.notice("clan fetch failed: %v", err)
				man.Files = append(man.Files, failRec(clanName, "clan-json", err))
			} else if pr.Res.HTTPStatus < 200 || pr.Res.HTTPStatus >= 300 {
				man.OK = false
				err = fmt.Errorf("clan HTTP %d", pr.Res.HTTPStatus)
				a.notice("%s", err.Error())
				man.Files = append(man.Files, failRec(clanName, "clan-json", err))
			} else if len(pr.Res.Data) == 0 || string(pr.Res.Data) == "null" {
				man.OK = false
				err = fmt.Errorf("clan payload is empty")
				a.notice("%s", err.Error())
				man.Files = append(man.Files, failRec(clanName, "clan-json", err))
			} else {
				clanRaw = append(json.RawMessage(nil), pr.Res.Data...)
				clanOK = true
				source := publicSource(path, pr, now)
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
		a.notice("clan tag is absent from profile and extended profile; clan.json omitted")
	}

	battleName := snapshot.FileName(stamp, collision, prefix, "clanbattle.json")
	var battleRaw json.RawMessage
	battleFetched := false
	const battlePath = "/api/activeClanBattle"
	bpr, berr := a.getPublicCached(ctx, battlePath, nil, api.BodyLimitDefault)
	if berr != nil {
		man.OK = false
		a.notice("active clan battle fetch failed: %v", berr)
		man.Files = append(man.Files, failRec(battleName, "clan-battle-json", berr))
	} else if bpr.Res.HTTPStatus < 200 || bpr.Res.HTTPStatus >= 300 {
		man.OK = false
		err := fmt.Errorf("active clan battle HTTP %d", bpr.Res.HTTPStatus)
		a.notice("%s", err.Error())
		man.Files = append(man.Files, failRec(battleName, "clan-battle-json", err))
	} else {
		battleFetched = true
		if len(bpr.Res.Data) > 0 && string(bpr.Res.Data) != "null" {
			battleRaw = append(json.RawMessage(nil), bpr.Res.Data...)
		}
	}

	observed := clan.Observe(now, userID, tag, clanRaw, battleRaw, clanOK, battleFetched)
	if observed.ActiveBattle != nil && battleFetched {
		source := publicSource(battlePath, bpr, now)
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
	warnings := append([]string{}, scanned.Warnings...)
	if scanErr != nil {
		warnings = append(warnings, fmt.Sprintf("summary scan failed: %v", scanErr))
		a.notice("summary scan failed: %v", scanErr)
	}
	for _, w := range scanned.Warnings {
		a.notice("summary history: %s", w)
	}
	sessionID := scanned.SessionID
	if sessionID == "" {
		sessionID = clan.NewSessionID(prefix, stamp)
	}
	sum := clan.Build(clan.Input{
		Now:             now,
		LocalStamp:      stamp,
		CollisionSuffix: collision,
		Prefix:          prefix,
		Profile:         profile,
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
	man.Summary["userId"] = sum.UserID
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
