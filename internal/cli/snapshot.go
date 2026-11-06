package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/api"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/jsonutil"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func newAccountSnapshotCmd(app *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write dated inventory/profile/currency snapshot files plus a manifest",
		Long: `Fetches account inventory, profile, and extended-profile once each, then writes
payload files and a non-recursive manifest (payload sizes + SHA-256 only).

Same-second collisions get a -0002 style suffix. Use snapshot diff offline to compare runs.`,
	}
	cmd.AddCommand(newAccountSnapshotExportCmd(app))
	cmd.AddCommand(newAccountSnapshotDiffCmd(app))
	// Default RunE when invoked as `account snapshot` (no subcommand) → export.
	var af accountFlags
	var outDir, prefix, note string
	var joinRAP bool
	cmd.RunE = func(c *cobra.Command, args []string) error {
		return app.runAccountSnapshot(c.Context(), snapshotOptions{
			Flags: af, Dir: outDir, Prefix: prefix, JoinRAP: joinRAP, Note: note,
		})
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true on each fetch (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	cmd.Flags().StringVar(&outDir, "dir", ".", "output directory (created if missing)")
	cmd.Flags().StringVar(&prefix, "prefix", "ps99", "filename prefix after the timestamp")
	cmd.Flags().BoolVar(&joinRAP, "join-rap", false, "join public market RAP into inventory CSV")
	cmd.Flags().StringVar(&note, "note", "", "optional user-supplied experiment context stored in the manifest")
	return cmd
}

func newAccountSnapshotExportCmd(app *App) *cobra.Command {
	var af accountFlags
	var outDir, prefix, note string
	var joinRAP bool
	cmd := &cobra.Command{
		Use:   "export",
		Short: "Write a dated snapshot (same as account snapshot)",
		RunE: func(c *cobra.Command, args []string) error {
			return app.runAccountSnapshot(c.Context(), snapshotOptions{
				Flags: af, Dir: outDir, Prefix: prefix, JoinRAP: joinRAP, Note: note,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true on each fetch")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm --refresh when non-interactive")
	cmd.Flags().StringVar(&outDir, "dir", ".", "output directory")
	cmd.Flags().StringVar(&prefix, "prefix", "ps99", "filename prefix")
	cmd.Flags().BoolVar(&joinRAP, "join-rap", false, "join public market RAP into inventory CSV")
	cmd.Flags().StringVar(&note, "note", "", "optional experiment note")
	return cmd
}

type snapshotOptions struct {
	Flags   accountFlags
	Dir     string
	Prefix  string
	JoinRAP bool
	Note    string
}

func (a *App) runAccountSnapshot(ctx context.Context, opt snapshotOptions) error {
	if err := a.confirmRefresh(opt.Flags, "account snapshot"); err != nil {
		return err
	}
	if err := snapshot.ValidatePrefix(opt.Prefix); err != nil {
		return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
	}
	dir := opt.Dir
	if dir == "" {
		dir = "."
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
	}
	if err := os.MkdirAll(absDir, 0o755); err != nil {
		return &ExitError{Code: exitcode.Usage, Msg: fmt.Sprintf("create output dir: %v", err)}
	}

	now := a.nowLocal()
	baseStamp := now.Format("2006-01-02-150405")
	stamp, collision, err := snapshot.AllocateStamp(absDir, baseStamp, opt.Prefix)
	if err != nil {
		return &ExitError{Code: exitcode.Usage, Msg: err.Error()}
	}

	profileAlias := a.Flags.Profile
	if profileAlias == "" {
		if p, _, _, err := a.resolveProfileToken(""); err == nil {
			profileAlias = p.Alias
		}
	}

	man := snapshot.Manifest{
		SchemaVersion:   snapshot.ManifestSchemaVersion,
		Command:         "account snapshot",
		OK:              true,
		LocalStamp:      stamp,
		CollisionSuffix: collision,
		LocalTimezone:   now.Location().String(),
		ExportedAt:      now.Format(time.RFC3339),
		Profile:         profileAlias,
		OutputDir:       absDir,
		Prefix:          opt.Prefix,
		Refresh:         opt.Flags.refresh,
		JoinRAP:         opt.JoinRAP,
		Excel:           a.Flags.Excel,
		Note:            opt.Note,
		Files:           []snapshot.FileRecord{},
		Summary:         map[string]any{},
	}

	displayStamp := stamp
	if collision != "" {
		displayStamp = stamp + "-" + collision
	}
	a.notice("snapshot %s → %s", displayStamp, absDir)

	// Inventory
	invEnv, invBuilt, invErr := a.snapshotInventory(ctx, opt)
	invJSONName := snapshot.FileName(stamp, collision, opt.Prefix, "inv.json")
	invCSVName := snapshot.FileName(stamp, collision, opt.Prefix, "inventory.csv")
	if invErr != nil {
		man.OK = false
		a.notice("inventory fetch failed: %v", invErr)
		man.Files = append(man.Files,
			failRec(invJSONName, "inventory-json", invErr),
			failRec(invCSVName, "inventory-csv", invErr),
		)
	} else {
		rec := a.writePayload(absDir, invJSONName, "inventory-json", invEnv, invEnv)
		man.Files = append(man.Files, rec)
		if !rec.OK {
			man.OK = false
		}
		csvRec := a.writePayloadRaw(absDir, invCSVName, "inventory-csv", func(w io.Writer) error {
			invRaw, _ := invBuilt.data["inventory"].(json.RawMessage)
			return output.WriteInventoryCSV(w, invRaw, output.InventoryCSVOptions{
				Excel: a.Flags.Excel, RAPIndex: a.pendingRAPIndex,
			})
		}, invEnv)
		a.pendingRAPIndex = nil
		man.Files = append(man.Files, csvRec)
		if !csvRec.OK {
			man.OK = false
		}
		man.Summary["inventory"] = map[string]any{
			"rowCount":       invBuilt.data["rowCount"],
			"quantitySum":    invBuilt.data["quantitySum"],
			"petRowCount":    invBuilt.data["petRowCount"],
			"petQuantitySum": invBuilt.data["petQuantitySum"],
			"quotaExhausted": invBuilt.data["quotaExhausted"],
			"snapshotAt":     formatSnapshotAt(invEnv.Freshness),
		}
	}

	// Profile + currencies
	profEnv, profData, curMap, profErr := a.snapshotProfile(ctx, opt)
	profName := snapshot.FileName(stamp, collision, opt.Prefix, "prof.json")
	curName := snapshot.FileName(stamp, collision, opt.Prefix, "currencies.csv")
	if profErr != nil {
		man.OK = false
		a.notice("profile fetch failed: %v", profErr)
		man.Files = append(man.Files,
			failRec(profName, "profile-json", profErr),
			failRec(curName, "currencies-csv", profErr),
		)
	} else {
		rec := a.writePayload(absDir, profName, "profile-json", profEnv, profEnv)
		man.Files = append(man.Files, rec)
		if !rec.OK {
			man.OK = false
		}
		csvRec := a.writePayloadRaw(absDir, curName, "currencies-csv", func(w io.Writer) error {
			return output.WriteCurrenciesCSV(w, curMap, output.CurrencyCSVOptions{Excel: a.Flags.Excel})
		}, profEnv)
		man.Files = append(man.Files, csvRec)
		if !csvRec.OK {
			man.OK = false
		}
		man.Summary["profile"] = map[string]any{
			"quotaExhausted": profData["quotaExhausted"],
			"statistics":     extractProfileStatistics(profData),
			"currencies":     exactCurrencyAmounts(curMap),
		}
	}

	// Extended
	extEnv, extErr := a.snapshotExtended(ctx, opt)
	extName := snapshot.FileName(stamp, collision, opt.Prefix, "extprof.json")
	if extErr != nil {
		man.OK = false
		a.notice("extended-profile fetch failed: %v", extErr)
		man.Files = append(man.Files, failRec(extName, "extended-profile-json", extErr))
	} else {
		rec := a.writePayload(absDir, extName, "extended-profile-json", extEnv, extEnv)
		man.Files = append(man.Files, rec)
		if !rec.OK {
			man.OK = false
		}
	}

	manName := snapshot.FileName(stamp, collision, opt.Prefix, "manifest.json")
	man.ManifestFile = manName
	// Write manifest once — no self size/digest entry.
	manPath := filepath.Join(absDir, manName)
	if _, err := snapshot.AtomicWrite(manPath, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(man)
	}); err != nil {
		man.OK = false
		a.notice("manifest write failed: %v", err)
	}

	for _, f := range man.Files {
		if f.OK {
			a.notice("[OK] %s (%d bytes sha256=%s)", f.Name, f.Bytes, shortSHA(f.SHA256))
		} else {
			a.notice("[FAIL] %s: %s", f.Name, f.Error)
		}
	}
	a.notice("snapshot export complete (%s)", manName)

	summaryOut := map[string]any{
		"localStamp":      stamp,
		"collisionSuffix": collision,
		"outputDir":       absDir,
		"manifest":        manName,
		"ok":              man.OK,
		"files":           man.Files,
		"summary":         man.Summary,
		"note":            opt.Note,
	}
	w := output.Writer{Out: a.stdout(), Pretty: a.Flags.Pretty, Clock: a.Deps.Clock}
	if err := w.Write(output.NewSuccess("account snapshot", nil, output.DefaultFreshnessNull(), summaryOut)); err != nil {
		return err
	}
	if !man.OK {
		return &ExitError{Code: exitcode.Upstream, Msg: "snapshot completed with errors; see manifest"}
	}
	return nil
}

func failRec(name, kind string, err error) snapshot.FileRecord {
	return snapshot.FileRecord{Name: name, RelPath: name, Kind: kind, OK: false, Error: err.Error()}
}

func shortSHA(s string) string {
	if len(s) < 12 {
		return s
	}
	return s[:12]
}

func (a *App) writePayload(dir, name, kind string, env output.Envelope, meta output.Envelope) snapshot.FileRecord {
	return a.writePayloadRaw(dir, name, kind, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(env)
	}, meta)
}

func (a *App) writePayloadRaw(dir, name, kind string, write func(io.Writer) error, meta output.Envelope) snapshot.FileRecord {
	path := filepath.Join(dir, name)
	rec := snapshot.FileRecord{Name: name, RelPath: name, Kind: kind, OK: true}
	if meta.Source != nil {
		rec.Endpoint = meta.Source.Endpoint
		rec.HTTPStatus = meta.Source.HTTPStatus
		rec.RetrievedAt = meta.Source.RetrievedAt.UTC().Format(time.RFC3339)
		fc := meta.Source.FromLocalCache
		rec.FromLocalCache = &fc
	}
	rec.SnapshotAt = formatSnapshotAt(meta.Freshness)
	if meta.Freshness != nil && meta.Freshness.Refresh != nil {
		if raw, ok := meta.Freshness.Refresh.(json.RawMessage); ok {
			rec.Refresh = append(json.RawMessage(nil), raw...)
			qe := refreshQuotaExhausted(raw)
			rec.QuotaExhausted = &qe
		}
	}
	wr, err := snapshot.AtomicWrite(path, write)
	if err != nil {
		rec.OK = false
		rec.Error = err.Error()
		return rec
	}
	rec.Bytes = wr.Bytes
	rec.SHA256 = wr.SHA256
	return rec
}

type snapshotBuilt struct {
	data map[string]any
}

func (a *App) snapshotInventory(ctx context.Context, opt snapshotOptions) (output.Envelope, snapshotBuilt, error) {
	_, tok, secrets, err := a.resolveProfileToken(api.ScopeInventory)
	if err != nil {
		return output.Envelope{}, snapshotBuilt{}, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetInventory(ctx, tok, api.AccountGetOptions{Refresh: opt.Flags.refresh})
	if err != nil {
		return output.Envelope{}, snapshotBuilt{}, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return output.Envelope{}, snapshotBuilt{}, err
	}
	invRes, ferr := api.FilterInventory(res.Data, "", "", false)
	if ferr != nil {
		return output.Envelope{}, snapshotBuilt{}, ferr
	}
	snap, cached := api.SnapshotFromInventory(res.Data)
	source := a.accountSource(res)
	source.Endpoint = "/v1/account/inventory"
	data := map[string]any{
		"inventory":       jsonutil.CloneRaw(invRes.Filtered),
		"rowCount":        invRes.RowCount,
		"quantitySum":     invRes.QuantitySum,
		"quantitySumNote": "sum of count across all filtered rows (may include non-pet stacks such as currencies); not an object/row total",
		"petRowCount":     invRes.PetRowCount,
		"petQuantitySum":  invRes.PetQuantitySum,
		"rowCountNote":    "rowCount is filtered item rows, not total item quantity",
		"cached":          cached,
		"rateLimit":       res.RateLimit(),
		"quotaExhausted":  refreshQuotaExhausted(res.Refresh),
	}
	if opt.JoinRAP {
		idx, matched, jerr := a.loadRAPJoin(ctx, invRes.Filtered)
		if jerr != nil {
			a.notice("rap join warning: %v", jerr)
			data["rapJoin"] = map[string]any{"ok": false, "error": jerr.Error()}
		} else {
			data["rapJoin"] = map[string]any{"ok": true, "indexSize": len(idx), "matchedRows": matched, "source": "/api/rap"}
			a.pendingRAPIndex = idx
		}
	}
	env := output.NewSuccess("account inventory", source, freshnessFromResult(res, snap), data)
	return env, snapshotBuilt{data: data}, nil
}

func (a *App) snapshotProfile(ctx context.Context, opt snapshotOptions) (output.Envelope, map[string]any, map[string]json.RawMessage, error) {
	_, tok, secrets, err := a.resolveProfileToken(api.ScopeProfile)
	if err != nil {
		return output.Envelope{}, nil, nil, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetProfile(ctx, tok, api.AccountGetOptions{Refresh: opt.Flags.refresh})
	if err != nil {
		return output.Envelope{}, nil, nil, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return output.Envelope{}, nil, nil, err
	}
	source := a.accountSource(res)
	source.Endpoint = "/v1/account/profile"
	data := map[string]any{
		"profile":        jsonutil.CloneRaw(res.Data),
		"rateLimit":      res.RateLimit(),
		"quotaExhausted": refreshQuotaExhausted(res.Refresh),
	}
	env := output.NewSuccess("account profile", source, freshnessFromResult(res, nil), data)
	ext, cerr := api.ExtractCurrencies(res.Data)
	if cerr != nil {
		return env, data, map[string]json.RawMessage{}, cerr
	}
	return env, data, ext.Currencies, nil
}

func (a *App) snapshotExtended(ctx context.Context, opt snapshotOptions) (output.Envelope, error) {
	_, tok, secrets, err := a.resolveProfileToken(api.ScopeExtendedProfile)
	if err != nil {
		return output.Envelope{}, err
	}
	defer func() { tok = "" }()
	res, err := a.apiClient().GetAccountView(ctx, "/v1/account/extendedProfile", tok, api.AccountGetOptions{Refresh: opt.Flags.refresh})
	if err != nil {
		return output.Envelope{}, fmt.Errorf("%s", secrets.Redact(err.Error()))
	}
	if err := mapAccountHTTP(res); err != nil {
		return output.Envelope{}, err
	}
	source := a.accountSource(res)
	source.Endpoint = "/v1/account/extendedProfile"
	data := map[string]any{
		"data":           jsonutil.CloneRaw(res.Data),
		"rateLimit":      res.RateLimit(),
		"quotaExhausted": refreshQuotaExhausted(res.Refresh),
	}
	return output.NewSuccess("account extended-profile", source, freshnessFromResult(res, nil), data), nil
}

func mapAccountHTTP(res api.Result) error {
	switch {
	case res.HTTPStatus == 401 || res.HTTPStatus == 403:
		return &ExitError{Code: exitcode.AuthRequired, Msg: "unauthorized or forbidden"}
	case res.HTTPStatus == 429:
		return &ExitError{Code: exitcode.RateLimit, Msg: "rate limited"}
	case res.HTTPStatus < 200 || res.HTTPStatus >= 300:
		return &ExitError{Code: exitcode.Upstream, Msg: fmt.Sprintf("upstream HTTP %d", res.HTTPStatus)}
	default:
		return nil
	}
}

func (a *App) nowLocal() time.Time {
	if a.Deps.Clock != nil {
		return a.Deps.Clock.Now().In(time.Local)
	}
	if a.Deps.Now != nil {
		return a.Deps.Now().In(time.Local)
	}
	return time.Now()
}

func formatSnapshotAt(f *output.Freshness) *string {
	if f == nil || f.SnapshotAt == nil {
		return nil
	}
	s := f.SnapshotAt.UTC().Format(time.RFC3339Nano)
	return &s
}

func exactCurrencyAmounts(currencies map[string]json.RawMessage) map[string]string {
	out := map[string]string{}
	for k, raw := range currencies {
		var fields map[string]json.RawMessage
		_ = json.Unmarshal(raw, &fields)
		if am := strings.TrimSpace(string(fields["_am"])); am != "" && am != "null" {
			out[k] = am
		}
	}
	return out
}

func extractProfileStatistics(profData map[string]any) map[string]string {
	out := map[string]string{}
	raw, ok := profData["profile"].(json.RawMessage)
	if !ok {
		return out
	}
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return out
	}
	var stats map[string]json.RawMessage
	if json.Unmarshal(top["Statistics"], &stats) != nil {
		return out
	}
	for k, v := range stats {
		s := strings.TrimSpace(string(v))
		if s != "" && s != "null" {
			if len(s) >= 2 && s[0] == '"' {
				var str string
				if json.Unmarshal(v, &str) == nil {
					out[k] = str
					continue
				}
			}
			out[k] = s
		}
	}
	return out
}
