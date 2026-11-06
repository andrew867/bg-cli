package cli

import (
	"os"
	"runtime"
	"time"

	"github.com/spf13/cobra"

	"github.com/andrew867/bg-cli/internal/auth"
	"github.com/andrew867/bg-cli/internal/config"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
)

func newDoctorCmd(app *App) *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Check local config, credential store, and profile scopes",
		Long: `Reports whether bg-cli can use the OS credential store, which profiles exist,
and which account command scopes each profile has granted. Does not print secrets
and does not call the network.`,
		RunE: func(cmd *cobra.Command, args []string) error {
			checks := []map[string]any{}
			ok := true

			checks = append(checks, map[string]any{
				"name":      "version",
				"ok":        true,
				"detail":    Version,
				"goos":      runtime.GOOS,
				"goarch":    runtime.GOARCH,
				"goVersion": runtime.Version(),
			})

			cfgDir, err := config.Dir()
			cfgPath, _ := config.Path()
			if err != nil {
				ok = false
				checks = append(checks, map[string]any{"name": "config_dir", "ok": false, "detail": err.Error()})
			} else {
				checks = append(checks, map[string]any{
					"name":   "config_dir",
					"ok":     true,
					"path":   cfgDir,
					"file":   cfgPath,
					"exists": fileExists(cfgPath),
				})
			}

			store := app.tokenStore()
			keyringOK := true
			keyringDetail := "credential store available"
			probe := "doctorprobe"
			if err := store.Set(probe, "probe-token-not-a-secret"); err != nil {
				keyringOK = false
				keyringDetail = err.Error()
				ok = false
			} else {
				defer func() { _ = store.Delete(probe) }()
				if _, gerr := store.Get(probe); gerr != nil {
					keyringOK = false
					keyringDetail = gerr.Error()
					ok = false
				}
			}
			checks = append(checks, map[string]any{
				"name":   "credential_store",
				"ok":     keyringOK,
				"detail": keyringDetail,
			})

			cfg, err := config.Load()
			profiles := []map[string]any{}
			if err != nil {
				ok = false
				checks = append(checks, map[string]any{"name": "config_load", "ok": false, "detail": err.Error()})
			} else {
				checks = append(checks, map[string]any{
					"name":           "config_load",
					"ok":             true,
					"defaultProfile": cfg.DefaultProfile,
					"profileCount":   len(cfg.Profiles),
				})
				for _, p := range cfg.Profiles {
					tokPresent := false
					if _, err := store.Get(p.Alias); err == nil {
						tokPresent = true
					}
					missing := missingFrom(auth.DefaultScopes, p.Scopes)
					expired := time.Now().UTC().After(p.ExpiresAt)
					profiles = append(profiles, map[string]any{
						"alias":                p.Alias,
						"clientId":             p.ClientID,
						"tokenPresent":         tokPresent,
						"expired":              expired,
						"expiresAt":            p.ExpiresAt.UTC().Format(time.RFC3339),
						"grantedScopes":        p.Scopes,
						"missingDefaultScopes": missing,
						"redirectUri":          p.RedirectURI,
					})
					if !tokPresent || expired || len(missing) > 0 {
						ok = false
					}
				}
			}

			data := map[string]any{
				"healthy":  ok,
				"checks":   checks,
				"profiles": profiles,
				"hints": []string{
					"Enable all seven account scopes on the developer app; auth login requests them by default.",
					"Re-run auth login after upgrading if account commands report missing scopes.",
					"Use account quota to see refresh used/limit and Retry-After without dumping profile bodies.",
					"Use --output csv with account inventory or account currencies for spreadsheet export.",
					"Add --join-rap on inventory CSV to attach public market RAP; --excel writes a UTF-8 BOM.",
				},
			}
			env := output.NewSuccess("doctor", nil, output.DefaultFreshnessNull(), data)
			if err := app.emit(env); err != nil {
				return err
			}
			if !ok {
				return &ExitError{Code: exitcode.Usage, Msg: "doctor found issues; see JSON output"}
			}
			return nil
		},
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func missingFrom(want, have []string) []string {
	set := map[string]struct{}{}
	for _, h := range have {
		set[h] = struct{}{}
	}
	var missing []string
	for _, w := range want {
		if _, ok := set[w]; !ok {
			missing = append(missing, w)
		}
	}
	return missing
}
