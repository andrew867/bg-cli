package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/exitcode"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
	"github.com/andrew867/bg-cli/internal/snapview"
)

func (a *App) presentSnapshot(man snapshot.Manifest, sum clan.Summary, priors []clan.Prior, opt snapshotOptions, absDir, manName string) error {
	var bundle *snapview.Bundle
	if opt.Zip {
		packed := a.packSnapshotZip(absDir, man, opt)
		bundle = &packed
	}
	summaryOut := map[string]any{
		"localStamp":      man.LocalStamp,
		"collisionSuffix": man.CollisionSuffix,
		"outputDir":       absDir,
		"manifest":        manName,
		"ok":              man.OK,
		"files":           man.Files,
		"summary":         man.Summary,
		"note":            opt.Note,
	}
	if bundle != nil {
		summaryOut["zip"] = map[string]any{
			"ok":       bundle.OK,
			"error":    bundle.Error,
			"name":     bundle.Name,
			"path":     bundle.Path,
			"bytes":    bundle.Bytes,
			"sha256":   bundle.SHA256,
			"members":  bundle.MemberNames,
			"missing":  bundle.Missing,
			"complete": bundle.Complete,
		}
	}
	if opt.MachineJSON {
		w := output.Writer{Out: a.stdout(), Pretty: a.Flags.Pretty, Clock: a.Deps.Clock}
		if err := w.Write(output.NewSuccess("account snapshot", nil, output.DefaultFreshnessNull(), summaryOut)); err != nil {
			return err
		}
	} else {
		text := snapview.Render(snapview.View{
			Summary:           sum,
			Manifest:          man,
			Priors:            priors,
			ShowAllCurrencies: opt.ShowAllCurrencies,
			Verbose:           opt.Verbose,
			Color:             a.snapshotColor(),
			Bundle:            bundle,
			Location:          time.Local,
		})
		if _, err := io.WriteString(a.stdout(), text); err != nil {
			return err
		}
	}
	if !man.OK {
		return &ExitError{Code: exitcode.Upstream, Msg: "snapshot completed with errors; see manifest"}
	}
	return nil
}

func (a *App) packSnapshotZip(absDir string, man snapshot.Manifest, opt snapshotOptions) snapview.Bundle {
	dest, err := snapshotZipDest(absDir, man, opt.ZipPath)
	if err != nil {
		a.notice("ZIP failed: %v", err)
		return snapview.Bundle{Error: err.Error()}
	}
	members, missing, err := snapshot.MembersFromManifest(absDir, man)
	if err != nil {
		a.notice("ZIP failed: %v", err)
		return snapview.Bundle{Error: err.Error()}
	}
	rep, err := snapshot.Pack(dest, members, nil)
	if err != nil {
		a.notice("ZIP failed: %v", err)
		return snapview.Bundle{Error: err.Error(), Missing: missing}
	}
	rep.Missing = append(rep.Missing, missing...)
	rep.Complete = rep.Complete && len(missing) == 0
	if !rep.Complete {
		a.notice("ZIP is incomplete; missing %s", strings.Join(rep.Missing, ", "))
	}
	return snapview.Bundle{
		OK:          true,
		Name:        rep.Name,
		Dir:         absDir,
		Path:        rep.Path,
		Bytes:       rep.Bytes,
		SHA256:      rep.SHA256,
		Members:     len(rep.Members),
		MemberNames: rep.Members,
		Missing:     rep.Missing,
		Complete:    rep.Complete,
	}
}

func snapshotZipDest(absDir string, man snapshot.Manifest, zipPath string) (string, error) {
	zipPath = strings.TrimSpace(zipPath)
	var dest string
	if zipPath == "" {
		base := strings.TrimSuffix(man.ManifestFile, "-manifest.json")
		if base == "" || base == man.ManifestFile {
			base = man.LocalStamp
			if man.CollisionSuffix != "" {
				base += "-" + man.CollisionSuffix
			}
			base += "-" + man.Prefix
		}
		dest = filepath.Join(absDir, base+".zip")
	} else if filepath.IsAbs(zipPath) {
		dest = filepath.Clean(zipPath)
	} else {
		dest = filepath.Clean(filepath.Join(absDir, zipPath))
		rel, err := filepath.Rel(absDir, dest)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("--zip-path escapes the output directory")
		}
	}
	if err := snapshot.ValidateZipPath(dest); err != nil {
		return "", err
	}
	return dest, nil
}

func (a *App) snapshotColor() bool {
	if a.Deps.IsTTY == nil || !a.Deps.IsTTY() {
		return false
	}
	env := os.Getenv
	if a.Deps.Env != nil {
		env = a.Deps.Env
	}
	if env("NO_COLOR") != "" || env("TERM") == "dumb" {
		return false
	}
	return true
}
