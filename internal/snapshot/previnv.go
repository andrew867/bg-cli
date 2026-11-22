package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// PreviousRequest selects the inventory that belongs to one prior summary.
// The summary file is the history system's baseline, not the lexically
// previous inv.json. Every opened path stays inside Dir.
type PreviousRequest struct {
	Dir         string
	SummaryFile string
	Prefix      string
	SessionID   string
	AccountID   string
	Profile     string
}

// PreviousInventory is a prior inventory payload located through its summary
// and manifest. Reason is set when the payload cannot be used. Items is then
// nil and must not be treated as an empty inventory.
type PreviousInventory struct {
	Items           json.RawMessage
	File            string
	ExportedAt      string
	Statistics      map[string]string
	StatisticsKnown bool
	Reason          string
}

type summaryHead struct {
	SchemaVersion   string `json:"schemaVersion"`
	LocalStamp      string `json:"localStamp"`
	CollisionSuffix string `json:"collisionSuffix"`
	ExportedAt      string `json:"exportedAt"`
	SessionID       string `json:"sessionId"`
	Prefix          string `json:"prefix"`
	Profile         string `json:"profile"`
	AccountID       string `json:"accountId"`
	Current         struct {
		StatisticsKnown bool              `json:"statisticsKnown"`
		Statistics      map[string]string `json:"statistics"`
	} `json:"current"`
}

// LoadPreviousInventory reads one prior inventory through that snapshot's
// summary and manifest. It rejects absolute paths, traversal, and symlinks.
// A summary that has no inventory-diff artifact and no inventory aggregates
// is still usable: only the inventory payload named by its manifest is read.
func LoadPreviousInventory(req PreviousRequest) PreviousInventory {
	body, err := readSnapshotFile(req.Dir, req.SummaryFile)
	if err != nil {
		return PreviousInventory{Reason: "previous summary is unusable: " + err.Error()}
	}
	var head summaryHead
	if err := json.Unmarshal(body, &head); err != nil {
		return PreviousInventory{Reason: "previous summary is malformed"}
	}
	if !summarySchemaOK(head.SchemaVersion) {
		return PreviousInventory{Reason: "previous summary schema is incompatible"}
	}
	if strings.TrimSpace(head.SessionID) == "" {
		return PreviousInventory{Reason: "previous summary is missing sessionId"}
	}
	if !sameSnapshotAccount(req.AccountID, req.Profile, req.SessionID, head.AccountID, head.Profile, head.SessionID) {
		return PreviousInventory{Reason: "previous snapshot is a different account or session"}
	}
	if strings.TrimSpace(head.LocalStamp) == "" {
		return PreviousInventory{Reason: "previous summary is missing localStamp"}
	}
	prefix := strings.TrimSpace(head.Prefix)
	if prefix == "" {
		prefix = strings.TrimSpace(req.Prefix)
	}
	if prefix == "" {
		return PreviousInventory{Reason: "previous summary is missing prefix"}
	}
	out := PreviousInventory{ExportedAt: head.ExportedAt, StatisticsKnown: head.Current.StatisticsKnown}
	if head.Current.StatisticsKnown {
		out.Statistics = head.Current.Statistics
	}
	manName := manifestNameFromSummary(req.SummaryFile, head.LocalStamp, head.CollisionSuffix, prefix)
	manBody, err := readSnapshotFile(req.Dir, manName)
	if err != nil {
		out.Reason = "previous manifest is unusable: " + err.Error()
		return out
	}
	var man Manifest
	if err := json.Unmarshal(manBody, &man); err != nil {
		out.Reason = "previous manifest is malformed"
		return out
	}
	if man.Profile != "" && req.Profile != "" && !strings.EqualFold(man.Profile, req.Profile) {
		out.Reason = "previous snapshot is a different account or session"
		return out
	}
	var invName string
	for _, rec := range man.Files {
		if rec.Kind == "inventory-json" && rec.OK {
			invName = rec.Name
			if invName == "" {
				invName = rec.RelPath
			}
			break
		}
	}
	if invName == "" {
		out.Reason = "previous snapshot has no inventory payload"
		return out
	}
	invBody, err := readSnapshotFile(req.Dir, invName)
	if err != nil {
		out.Reason = "previous inventory is unusable: " + err.Error()
		return out
	}
	data, _, err := EnvelopeData(invBody)
	if err != nil {
		out.Reason = "previous inventory is malformed"
		return out
	}
	items := data["inventory"]
	if len(items) == 0 || string(items) == "null" {
		out.Reason = "previous inventory payload is missing"
		return out
	}
	out.Items = append(json.RawMessage(nil), items...)
	out.File = invName
	return out
}

// manifestNameFromSummary uses the summary file's own stem so a profile
// segment in the name is preserved. Older summaries still map
// "{stamp}-{prefix}-summary.json" to "{stamp}-{prefix}-manifest.json".
func manifestNameFromSummary(summaryFile, stamp, collision, prefix string) string {
	base := strings.TrimSpace(summaryFile)
	base = strings.ReplaceAll(base, "\\", "/")
	if i := strings.LastIndex(base, "/"); i >= 0 {
		base = base[i+1:]
	}
	const suffix = "-summary.json"
	if strings.HasSuffix(base, suffix) && base != suffix && !strings.Contains(base, "..") {
		return strings.TrimSuffix(base, suffix) + "-manifest.json"
	}
	return FileName(stamp, collision, prefix, "manifest.json")
}

func summarySchemaOK(v string) bool {
	return v == "1" || strings.HasPrefix(v, "1.")
}

func sameSnapshotAccount(expectAccount, expectProfile, expectSession, gotAccount, gotProfile, gotSession string) bool {
	ea, ga := strings.TrimSpace(expectAccount), strings.TrimSpace(gotAccount)
	if ea != "" && ga != "" && !strings.EqualFold(ea, ga) {
		return false
	}
	ep, gp := strings.TrimSpace(expectProfile), strings.TrimSpace(gotProfile)
	if ep != "" && gp != "" && !strings.EqualFold(ep, gp) {
		return false
	}
	if strings.TrimSpace(expectSession) != "" && strings.TrimSpace(gotSession) != "" && expectSession != gotSession {
		return false
	}
	return true
}

func readSnapshotFile(dir, name string) ([]byte, error) {
	if strings.TrimSpace(name) == "" || strings.ContainsAny(name, `/\`) || strings.Contains(name, "..") {
		return nil, fmt.Errorf("refusing snapshot path %q", name)
	}
	path, err := ResolveFilePath(dir, FileRecord{Name: name, RelPath: name})
	if err != nil {
		return nil, err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("refusing to follow symlink %s", name)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("%s is not a regular file", name)
	}
	return os.ReadFile(path)
}
