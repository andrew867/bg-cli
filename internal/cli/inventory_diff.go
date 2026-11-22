package cli

import (
	"encoding/json"
	"io"
	"strings"

	"github.com/andrew867/bg-cli/internal/clan"
	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

func (a *App) writeInventoryDiff(absDir, stamp, collision, prefix, fileProfile string, man *snapshot.Manifest, sum clan.Summary, currentInv json.RawMessage) *snapshot.InventoryDiff {
	opt := snapshot.InventoryDiffOptions{
		CurrentAt:     sum.ExportedAt,
		CurStats:      sum.Current.Statistics,
		CurStatsKnown: sum.Current.StatisticsKnown,
	}
	if len(currentInv) > 0 {
		opt.CurrentFile = snapshot.ProfileFileName(stamp, collision, fileProfile, prefix, "inv.json")
	}
	var prev json.RawMessage
	win := sum.Windows.Previous
	switch {
	case !win.Valid || strings.TrimSpace(win.BaselineFile) == "":
		opt.UnavailableReason = win.Reason
		if opt.UnavailableReason == "" {
			opt.UnavailableReason = "no previous snapshot"
		}
	default:
		loaded := snapshot.LoadPreviousInventory(snapshot.PreviousRequest{
			Dir:             absDir,
			SummaryFile:     win.BaselineFile,
			Prefix:          prefix,
			SessionID:       sum.SessionID,
			AccountID:       sum.AccountID,
			LinkedAccountID: sum.LinkedAccountID,
			PS99UserID:      sum.PS99UserID,
			CredentialEpoch: sum.CredentialEpoch,
			Profile:         sum.Profile,
		})
		if loaded.Reason != "" {
			opt.UnavailableReason = loaded.Reason
			opt.BaselineAt = loaded.ExportedAt
		} else {
			prev = loaded.Items
			opt.BaselineFile = loaded.File
			opt.BaselineAt = loaded.ExportedAt
			opt.PrevStats = loaded.Statistics
			opt.PrevStatsKnown = loaded.StatisticsKnown
		}
	}
	diff := snapshot.DiffInventories(prev, currentInv, opt)
	for _, w := range diff.Warnings {
		a.notice("inventory diff: %s", w)
	}
	if diff.Status != snapshot.DiffStatusOK && diff.Reason != "" {
		a.notice("inventory diff: %s", diff.Reason)
	}
	name := snapshot.ProfileFileName(stamp, collision, fileProfile, prefix, "inventory-diff.json")
	rec := a.writePayloadRaw(absDir, name, "inventory-diff", func(w io.Writer) error {
		return snapshot.WriteInventoryDiff(w, diff)
	}, output.Envelope{})
	if !rec.OK {
		a.notice("inventory diff write failed: %s", rec.Error)
		return &diff
	}
	// The diff is derived. It is not an Inventory source, so it has no view.
	// inv.json and inventory.csv keep View "inventory".
	rec.View = ""
	rec.ContentStatus = snapshot.ContentOK
	man.Files = append(man.Files, rec)
	return &diff
}
