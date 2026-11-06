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
)

func newAccountSnapshotCmd(app *App) *cobra.Command {
	var af accountFlags
	var outDir, prefix string
	var joinRAP bool
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Write dated inventory/profile/currency snapshot files plus a manifest",
		Long: `Fetches account inventory, profile, and extended-profile once each, then writes:

  {stamp}-{prefix}-inv.json
  {stamp}-{prefix}-prof.json
  {stamp}-{prefix}-extprof.json
  {stamp}-{prefix}-inventory.csv
  {stamp}-{prefix}-currencies.csv
  {stamp}-{prefix}-manifest.json

The local stamp uses the host timezone as yyyy-MM-dd-HHmmss (including seconds).
Default --dir is the current working directory. The manifest records API snapshot
timestamps (fetchedAt when present), file sizes, and summary totals useful for
comparing two runs (petQuantitySum, currencies, profile Statistics).

Stdout receives a small JSON summary only — payloads go to the files.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.runAccountSnapshot(cmd.Context(), snapshotOptions{
				Flags:   af,
				Dir:     outDir,
				Prefix:  prefix,
				JoinRAP: joinRAP,
			})
		},
	}
	cmd.Flags().BoolVar(&af.refresh, "refresh", false, "request refresh=true on each fetch (consumes quota; needs --yes if non-interactive)")
	cmd.Flags().BoolVar(&af.yes, "yes", false, "confirm quota-affecting --refresh when non-interactive")
	cmd.Flags().StringVar(&outDir, "dir", ".", "output directory (created if missing)")
	cmd.Flags().StringVar(&prefix, "prefix", "ps99", "filename prefix after the timestamp")
	cmd.Flags().BoolVar(&joinRAP, "join-rap", false, "join public market RAP into inventory CSV")
	return cmd
}

type snapshotOptions struct {
	Flags   accountFlags
	Dir     string
	Prefix  string
	JoinRAP bool
}

type snapshotFileResult struct {
	Name        string  `json:"name"`
	Path        string  `json:"path"`
	Bytes       int64   `json:"bytes"`
	OK          bool    `json:"ok"`
	Error       string  `json:"error,omitempty"`
	Kind        string  `json:"kind"`
	Endpoint    string  `json:"endpoint,omitempty"`
	HTTPStatus  int     `json:"httpStatus,omitempty"`
	SnapshotAt  *string `json:"snapshotAt,omitempty"`
	RetrievedAt string  `json:"retrievedAt,omitempty"`
}

type snapshotManifest struct {
	SchemaVersion string               `json:"schemaVersion"`
	Command       string               `json:"command"`
	OK            bool                 `json:"ok"`
	LocalStamp    string               `json:"localStamp"`
	LocalTimezone string               `json:"localTimezone"`
	ExportedAt    string               `json:"exportedAt"`
	Profile       string               `json:"profile"`
	OutputDir     string               `json:"outputDir"`
	Prefix        string               `json:"prefix"`
	Refresh       bool                 `json:"refresh"`
	JoinRAP       bool                 `json:"joinRap"`
	Excel         bool                 `json:"excel"`
	Files         []snapshotFileResult `json:"files"`
	Summary       map[string]any       `json:"summary"`
}

func (a *App) runAccountSnapshot(ctx context.Context, opt snapshotOptions) error {
	if err := a.confirmRefresh(opt.Flags, "account snapshot"); err != nil {
		return err
	}
	if strings.TrimSpace(opt.Prefix) == "" {
		return &ExitError{Code: exitcode.Usage, Msg: "--prefix must not be empty"}
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
	stamp := now.Format("2006-01-02-150405")

	profileAlias := a.Flags.Profile
	if profileAlias == "" {
		if p, _, _, err := a.resolveProfileToken(""); err == nil {
			profileAlias = p.Alias
		}
	}

	manifest := snapshotManifest{
		SchemaVersion: output.SchemaVersion,
		Command:       "account snapshot",
		OK:            true,
		LocalStamp:    stamp,
		LocalTimezone: now.Location().String(),
		ExportedAt:    now.Format(time.RFC3339),
		Profile:       profileAlias,
		OutputDir:     absDir,
		Prefix:        opt.Prefix,
		Refresh:       opt.Flags.refresh,
		JoinRAP:       opt.JoinRAP,
		Excel:         a.Flags.Excel,
		Files:         []snapshotFileResult{},
		Summary:       map[string]any{},
	}

	a.notice("snapshot %s → %s", stamp, absDir)

	// --- inventory (JSON + CSV from one fetch) ---
	invEnv, invRes, invErr := a.snapshotInventory(ctx, opt)
	if invErr != nil {
		manifest.OK = false
		a.notice("inventory fetch failed: %v", invErr)
		manifest.Files = append(manifest.Files, snapshotFileResult{
			Name: stamp + "-" + opt.Prefix + "-inv.json", OK: false, Error: invErr.Error(), Kind: "inventory-json",
		})
		manifest.Files = append(manifest.Files, snapshotFileResult{
			Name: stamp + "-" + opt.Prefix + "-inventory.csv", OK: false, Error: invErr.Error(), Kind: "inventory-csv",
		})
	} else {
		invName := stamp + "-" + opt.Prefix + "-inv.json"
		fr := a.writeSnapshotJSON(absDir, invName, invEnv)
		fr.Kind = "inventory-json"
		fr.Endpoint = invEnv.Source.Endpoint
		fr.HTTPStatus = invEnv.Source.HTTPStatus
		fr.RetrievedAt = invEnv.Source.RetrievedAt.UTC().Format(time.RFC3339)
		fr.SnapshotAt = formatSnapshotAt(invEnv.Freshness)
		manifest.Files = append(manifest.Files, fr)
		if !fr.OK {
			manifest.OK = false
		}

		csvName := stamp + "-" + opt.Prefix + "-inventory.csv"
		cfr := a.writeSnapshotFile(absDir, csvName, func(w io.Writer) error {
			invRaw, _ := invRes.data["inventory"].(json.RawMessage)
			return output.WriteInventoryCSV(w, invRaw, output.InventoryCSVOptions{
				Excel:    a.Flags.Excel,
				RAPIndex: a.pendingRAPIndex,
			})
		})
		a.pendingRAPIndex = nil
		cfr.Kind = "inventory-csv"
		cfr.Endpoint = invEnv.Source.Endpoint
		cfr.HTTPStatus = invEnv.Source.HTTPStatus
		cfr.RetrievedAt = invEnv.Source.RetrievedAt.UTC().Format(time.RFC3339)
		cfr.SnapshotAt = formatSnapshotAt(invEnv.Freshness)
		manifest.Files = append(manifest.Files, cfr)
		if !cfr.OK {
			manifest.OK = false
		}

		manifest.Summary["inventory"] = map[string]any{
			"rowCount":       invRes.data["rowCount"],
			"quantitySum":    invRes.data["quantitySum"],
			"petRowCount":    invRes.data["petRowCount"],
			"petQuantitySum": invRes.data["petQuantitySum"],
			"quotaExhausted": invRes.data["quotaExhausted"],
			"snapshotAt":     formatSnapshotAt(invEnv.Freshness),
		}
	}

	// --- profile (JSON) + currencies CSV from one fetch ---
	profEnv, profData, curMap, profErr := a.snapshotProfile(ctx, opt)
	if profErr != nil {
		manifest.OK = false
		a.notice("profile fetch failed: %v", profErr)
		manifest.Files = append(manifest.Files, snapshotFileResult{
			Name: stamp + "-" + opt.Prefix + "-prof.json", OK: false, Error: profErr.Error(), Kind: "profile-json",
		})
		manifest.Files = append(manifest.Files, snapshotFileResult{
			Name: stamp + "-" + opt.Prefix + "-currencies.csv", OK: false, Error: profErr.Error(), Kind: "currencies-csv",
		})
	} else {
		profName := stamp + "-" + opt.Prefix + "-prof.json"
		fr := a.writeSnapshotJSON(absDir, profName, profEnv)
		fr.Kind = "profile-json"
		fr.Endpoint = profEnv.Source.Endpoint
		fr.HTTPStatus = profEnv.Source.HTTPStatus
		fr.RetrievedAt = profEnv.Source.RetrievedAt.UTC().Format(time.RFC3339)
		manifest.Files = append(manifest.Files, fr)
		if !fr.OK {
			manifest.OK = false
		}

		curName := stamp + "-" + opt.Prefix + "-currencies.csv"
		cfr := a.writeSnapshotFile(absDir, curName, func(w io.Writer) error {
			return output.WriteCurrenciesCSV(w, curMap, output.CurrencyCSVOptions{Excel: a.Flags.Excel})
		})
		cfr.Kind = "currencies-csv"
		cfr.Endpoint = profEnv.Source.Endpoint
		cfr.HTTPStatus = profEnv.Source.HTTPStatus
		cfr.RetrievedAt = profEnv.Source.RetrievedAt.UTC().Format(time.RFC3339)
		manifest.Files = append(manifest.Files, cfr)
		if !cfr.OK {
			manifest.OK = false
		}

		manifest.Summary["profile"] = map[string]any{
			"quotaExhausted": profData["quotaExhausted"],
			"statistics":     extractProfileStatistics(profData),
			"currencies":     exactCurrencyAmounts(curMap),
		}
	}

	// --- extended profile ---
	extEnv, extErr := a.snapshotExtended(ctx, opt)
	if extErr != nil {
		manifest.OK = false
		a.notice("extended-profile fetch failed: %v", extErr)
		manifest.Files = append(manifest.Files, snapshotFileResult{
			Name: stamp + "-" + opt.Prefix + "-extprof.json", OK: false, Error: extErr.Error(), Kind: "extended-profile-json",
		})
	} else {
		extName := stamp + "-" + opt.Prefix + "-extprof.json"
		fr := a.writeSnapshotJSON(absDir, extName, extEnv)
		fr.Kind = "extended-profile-json"
		fr.Endpoint = extEnv.Source.Endpoint
		fr.HTTPStatus = extEnv.Source.HTTPStatus
		fr.RetrievedAt = extEnv.Source.RetrievedAt.UTC().Format(time.RFC3339)
		manifest.Files = append(manifest.Files, fr)
		if !fr.OK {
			manifest.OK = false
		}
	}

	manName := stamp + "-" + opt.Prefix + "-manifest.json"
	manPath := filepath.Join(absDir, manName)
	manFR := a.writeSnapshotFile(absDir, manName, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(manifest)
	})
	manFR.Kind = "manifest"
	// Re-append after writing so manifest file lists itself? Better include in files before write.
	// Rewrite manifest to include the manifest entry itself.
	manifest.Files = append(manifest.Files, snapshotFileResult{
		Name:  manName,
		Path:  manPath,
		Bytes: manFR.Bytes,
		OK:    manFR.OK,
		Error: manFR.Error,
		Kind:  "manifest",
	})
	if !manFR.OK {
		manifest.OK = false
	} else {
		// Update on-disk manifest with self entry and final ok flag.
		_ = a.writeSnapshotFile(absDir, manName, func(w io.Writer) error {
			enc := json.NewEncoder(w)
			enc.SetIndent("", "  ")
			enc.SetEscapeHTML(false)
			return enc.Encode(manifest)
		})
		if fi, err := os.Stat(manPath); err == nil {
			manifest.Files[len(manifest.Files)-1].Bytes = fi.Size()
		}
	}

	for _, f := range manifest.Files {
		if f.Kind == "manifest" {
			continue
		}
		if f.OK {
			a.notice("[OK] %s (%d bytes)", f.Name, f.Bytes)
		} else {
			a.notice("[FAIL] %s: %s", f.Name, f.Error)
		}
	}
	a.notice("snapshot export complete")

	summaryOut := map[string]any{
		"localStamp": stamp,
		"outputDir":  absDir,
		"manifest":   manName,
		"ok":         manifest.OK,
		"files":      manifest.Files,
		"summary":    manifest.Summary,
	}
	// Always emit a compact JSON summary on stdout (ignore --output csv/table/raw).
	w := output.Writer{Out: a.stdout(), Pretty: a.Flags.Pretty, Clock: a.Deps.Clock}
	if err := w.Write(output.NewSuccess("account snapshot", nil, output.DefaultFreshnessNull(), summaryOut)); err != nil {
		return err
	}
	if !manifest.OK {
		return &ExitError{Code: exitcode.Upstream, Msg: "snapshot completed with errors; see manifest"}
	}
	return nil
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

func (a *App) writeSnapshotJSON(dir, name string, env output.Envelope) snapshotFileResult {
	return a.writeSnapshotFile(dir, name, func(w io.Writer) error {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(env)
	})
}

func (a *App) writeSnapshotFile(dir, name string, write func(io.Writer) error) snapshotFileResult {
	path := filepath.Join(dir, name)
	fr := snapshotFileResult{Name: name, Path: path, OK: true}
	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		fr.OK = false
		fr.Error = err.Error()
		return fr
	}
	if err := write(f); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		fr.OK = false
		fr.Error = err.Error()
		return fr
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		fr.OK = false
		fr.Error = err.Error()
		return fr
	}
	_ = os.Remove(path) // Windows-friendly replace
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		fr.OK = false
		fr.Error = err.Error()
		return fr
	}
	if fi, err := os.Stat(path); err == nil {
		fr.Bytes = fi.Size()
	}
	return fr
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
