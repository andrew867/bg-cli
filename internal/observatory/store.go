package observatory

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const storeSchemaVersion = "observatory.store.v1"

// CollectionSnapshotMeta records provenance for one revisioned local catalog snapshot.
type CollectionSnapshotMeta struct {
	ID            string `json:"id"`
	FetchedAt     string `json:"fetchedAt"`
	OriginURL     string `json:"originUrl"`
	BodySHA256    string `json:"bodySha256"`
	Collection    string `json:"collection"`
	SchemaVersion string `json:"schemaVersion"`
}

// StoredCollection is raw JSON plus meta returned from LoadCollection.
type StoredCollection struct {
	Raw  []byte
	Meta CollectionSnapshotMeta
}

// SaveCollection writes rawJSON and meta.json under dir for the named collection.
func SaveCollection(dir, name string, rawJSON []byte, meta CollectionSnapshotMeta) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return fmt.Errorf("collection name is required")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if meta.BodySHA256 == "" {
		sum := sha256.Sum256(rawJSON)
		meta.BodySHA256 = hex.EncodeToString(sum[:])
	}
	if meta.Collection == "" {
		meta.Collection = name
	}
	if meta.SchemaVersion == "" {
		meta.SchemaVersion = storeSchemaVersion
	}
	if meta.FetchedAt == "" {
		meta.FetchedAt = time.Now().UTC().Format(time.RFC3339)
	}
	if meta.ID == "" {
		meta.ID = meta.Collection + "@" + meta.BodySHA256[:12]
	}
	base := filepath.Join(dir, sanitizeFilename(name))
	if err := os.WriteFile(base+".json", rawJSON, 0o644); err != nil {
		return err
	}
	mb, err := json.MarshalIndent(meta, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(base+".meta.json", mb, 0o644)
}

// LoadCollection reads the newest snapshot for name from dir (by meta fetchedAt).
func LoadCollection(dir, name string) (StoredCollection, error) {
	name = strings.TrimSpace(name)
	base := filepath.Join(dir, sanitizeFilename(name))
	raw, err := os.ReadFile(base + ".json")
	if err != nil {
		return StoredCollection{}, err
	}
	metaBytes, err := os.ReadFile(base + ".meta.json")
	if err != nil {
		return StoredCollection{}, err
	}
	var meta CollectionSnapshotMeta
	if err := json.Unmarshal(metaBytes, &meta); err != nil {
		return StoredCollection{}, err
	}
	return StoredCollection{Raw: raw, Meta: meta}, nil
}

// LocalCollectionEntry summarizes one on-disk collection.
type LocalCollectionEntry struct {
	Name string                 `json:"name"`
	Meta CollectionSnapshotMeta `json:"meta"`
}

// ListLocalCollections lists *.meta.json files in dir.
func ListLocalCollections(dir string) ([]LocalCollectionEntry, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []LocalCollectionEntry
	seen := map[string]bool{}
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".meta.json") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".meta.json")
		if seen[name] {
			continue
		}
		seen[name] = true
		mb, err := os.ReadFile(filepath.Join(dir, e.Name()))
		if err != nil {
			continue
		}
		var meta CollectionSnapshotMeta
		if json.Unmarshal(mb, &meta) != nil {
			continue
		}
		display := meta.Collection
		if display == "" {
			display = name
		}
		out = append(out, LocalCollectionEntry{Name: display, Meta: meta})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// SemanticChangeKind classifies a catalog diff entry.
type SemanticChangeKind string

const (
	ChangeWeight SemanticChangeKind = "weight"
	ChangeAsset  SemanticChangeKind = "asset"
	ChangeFlag   SemanticChangeKind = "flag"
	ChangeOther  SemanticChangeKind = "other"
)

// SemanticChange is one detected difference between collection snapshots.
type SemanticChange struct {
	Kind      SemanticChangeKind `json:"kind"`
	Path      string             `json:"path"`
	From      string             `json:"from,omitempty"`
	To        string             `json:"to,omitempty"`
	ConfigKey string             `json:"configKey,omitempty"`
}

// DiffResult is the output of DiffCollections.
type DiffResult struct {
	Changes          []SemanticChange `json:"changes"`
	ReorderOnlyPaths []string         `json:"reorderOnlyPaths,omitempty"`
	OK               bool             `json:"ok"`
}

// DiffCollections compares two raw collection JSON documents semantically.
func DiffCollections(fromRaw, toRaw []byte) (DiffResult, error) {
	var fromAny, toAny any
	if err := json.Unmarshal(fromRaw, &fromAny); err != nil {
		return DiffResult{}, fmt.Errorf("from: %w", err)
	}
	if err := json.Unmarshal(toRaw, &toAny); err != nil {
		return DiffResult{}, fmt.Errorf("to: %w", err)
	}
	res := DiffResult{OK: true}
	fromNorm := normalizeJSON(fromAny)
	toNorm := normalizeJSON(toAny)
	diffWalk("", fromNorm, toNorm, &res)
	if len(res.Changes) > 0 {
		res.OK = false
	}
	return res, nil
}

func diffWalk(path string, from, to any, res *DiffResult) {
	if jsonEqual(from, to) {
		return
	}
	fArr, fOk := from.([]any)
	tArr, tOk := to.([]any)
	if fOk && tOk {
		if arrayReorderOnly(fArr, tArr) {
			if path != "" {
				res.ReorderOnlyPaths = append(res.ReorderOnlyPaths, path)
			}
			return
		}
		maxLen := len(fArr)
		if len(tArr) > maxLen {
			maxLen = len(tArr)
		}
		for i := 0; i < maxLen; i++ {
			p := fmt.Sprintf("%s[%d]", path, i)
			var fi, ti any
			if i < len(fArr) {
				fi = fArr[i]
			}
			if i < len(tArr) {
				ti = tArr[i]
			}
			diffWalk(p, fi, ti, res)
		}
		return
	}
	fMap, fOk := from.(map[string]any)
	tMap, tOk := to.(map[string]any)
	if fOk && tOk {
		keys := map[string]bool{}
		for k := range fMap {
			keys[k] = true
		}
		for k := range tMap {
			keys[k] = true
		}
		sorted := make([]string, 0, len(keys))
		for k := range keys {
			sorted = append(sorted, k)
		}
		sort.Strings(sorted)
		for _, k := range sorted {
			p := k
			if path != "" {
				p = path + "." + k
			}
			diffWalk(p, fMap[k], tMap[k], res)
		}
		return
	}
	chg := SemanticChange{
		Path: path,
		From: fmt.Sprintf("%v", from),
		To:   fmt.Sprintf("%v", to),
		Kind: classifyChange(path, from, to),
	}
	res.Changes = append(res.Changes, chg)
}

func classifyChange(path string, _, _ any) SemanticChangeKind {
	lower := strings.ToLower(path)
	switch {
	case strings.Contains(lower, "weight"), strings.Contains(lower, "pets"):
		return ChangeWeight
	case strings.Contains(lower, "asset"), strings.Contains(lower, "icon"), strings.Contains(lower, "image"):
		return ChangeAsset
	case strings.Contains(lower, "flag"), strings.Contains(lower, "odds"):
		return ChangeFlag
	default:
		return ChangeOther
	}
}

func arrayReorderOnly(a, b []any) bool {
	if len(a) != len(b) {
		return false
	}
	mb := multisetOf(a)
	ma := multisetOf(b)
	return jsonEqual(ma, mb)
}

func multisetOf(arr []any) []any {
	cp := make([]any, len(arr))
	copy(cp, arr)
	sort.SliceStable(cp, func(i, j int) bool {
		return fmt.Sprintf("%v", cp[i]) < fmt.Sprintf("%v", cp[j])
	})
	return cp
}

func normalizeJSON(v any) any {
	switch x := v.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, val := range x {
			out[k] = normalizeJSON(val)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, val := range x {
			out[i] = normalizeJSON(val)
		}
		return out
	case float64:
		// json.Number would be string in UseNumber mode; keep float as-is
		return x
	case string:
		return x
	default:
		return x
	}
}

func jsonEqual(a, b any) bool {
	ab, _ := json.Marshal(a)
	bb, _ := json.Marshal(b)
	return string(ab) == string(bb)
}

// CoverageManifest summarizes which local collections are present.
type CoverageManifest struct {
	GeneratedAt  string                 `json:"generatedAt"`
	StoreDir     string                 `json:"storeDir"`
	Collections  []LocalCollectionEntry `json:"collections"`
	MissingNames []string               `json:"missingNames,omitempty"`
	Notes        []string               `json:"notes,omitempty"`
}

// BuildCoverageManifest compares dir contents to expected collection names.
func BuildCoverageManifest(dir string, expected []string) (CoverageManifest, error) {
	local, err := ListLocalCollections(dir)
	if err != nil {
		return CoverageManifest{}, err
	}
	have := map[string]bool{}
	for _, e := range local {
		have[e.Name] = true
	}
	var missing []string
	for _, name := range expected {
		if !have[name] {
			missing = append(missing, name)
		}
	}
	return CoverageManifest{
		GeneratedAt:  time.Now().UTC().Format(time.RFC3339),
		StoreDir:     dir,
		Collections:  local,
		MissingNames: missing,
		Notes: []string{
			"coverage is offline from the local catalog store only",
			"missing collection names are expectations, not proof the API lacks them",
		},
	}, nil
}

func sanitizeFilename(s string) string {
	s = strings.TrimSpace(s)
	repl := strings.NewReplacer("/", "_", "\\", "_", "..", "_")
	return repl.Replace(s)
}
