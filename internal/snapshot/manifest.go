package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const ManifestSchemaVersion = "1.1"

// FileRecord describes one payload file written by a snapshot export.
// The manifest file itself is referenced by ManifestFile and is NOT listed here
// with a self-referential size/digest.
type FileRecord struct {
	Name           string          `json:"name"`
	RelPath        string          `json:"relPath"`
	Kind           string          `json:"kind"`
	OK             bool            `json:"ok"`
	Error          string          `json:"error,omitempty"`
	Bytes          int64           `json:"bytes,omitempty"`
	SHA256         string          `json:"sha256,omitempty"`
	Endpoint       string          `json:"endpoint,omitempty"`
	HTTPStatus     int             `json:"httpStatus,omitempty"`
	RetrievedAt    string          `json:"retrievedAt,omitempty"`
	SnapshotAt     *string         `json:"snapshotAt,omitempty"`
	FromLocalCache *bool           `json:"fromLocalCache,omitempty"`
	QuotaExhausted *bool           `json:"quotaExhausted,omitempty"`
	Refresh        json.RawMessage `json:"refresh,omitempty"`
}

// Manifest is the on-disk snapshot index (schema 1.1).
type Manifest struct {
	SchemaVersion   string         `json:"schemaVersion"`
	Command         string         `json:"command"`
	OK              bool           `json:"ok"`
	LocalStamp      string         `json:"localStamp"`
	CollisionSuffix string         `json:"collisionSuffix,omitempty"`
	LocalTimezone   string         `json:"localTimezone"`
	ExportedAt      string         `json:"exportedAt"`
	Profile         string         `json:"profile,omitempty"`
	OutputDir       string         `json:"outputDir"`
	Prefix          string         `json:"prefix"`
	Refresh         bool           `json:"refresh"`
	JoinRAP         bool           `json:"joinRap"`
	Excel           bool           `json:"excel"`
	Note            string         `json:"note,omitempty"`
	Files           []FileRecord   `json:"files"`
	ManifestFile    string         `json:"manifestFile"`
	Summary         map[string]any `json:"summary,omitempty"`
}

// LoadManifest reads and validates a manifest JSON file.
func LoadManifest(path string) (*Manifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	if m.SchemaVersion == "" {
		return nil, fmt.Errorf("manifest missing schemaVersion")
	}
	if m.SchemaVersion != ManifestSchemaVersion && m.SchemaVersion != "1.0" {
		return nil, fmt.Errorf("unsupported manifest schemaVersion %q", m.SchemaVersion)
	}
	if len(m.Files) == 0 {
		return nil, fmt.Errorf("manifest has no files")
	}
	return &m, nil
}

// ResolveFilePath returns an absolute path for a file record, constrained under baseDir.
func ResolveFilePath(baseDir string, rec FileRecord) (string, error) {
	name := rec.RelPath
	if name == "" {
		name = rec.Name
	}
	if name == "" {
		return "", fmt.Errorf("file record missing name")
	}
	if filepath.IsAbs(name) {
		return "", fmt.Errorf("absolute paths are not allowed in manifest file entries: %s", name)
	}
	clean := filepath.Clean(name)
	if strings.HasPrefix(clean, "..") || strings.Contains(clean, string(filepath.Separator)+"..") {
		return "", fmt.Errorf("path traversal rejected: %s", name)
	}
	full := filepath.Join(baseDir, clean)
	absBase, err := filepath.Abs(baseDir)
	if err != nil {
		return "", err
	}
	absFull, err := filepath.Abs(full)
	if err != nil {
		return "", err
	}
	rel, err := filepath.Rel(absBase, absFull)
	if err != nil || strings.HasPrefix(rel, "..") {
		return "", fmt.Errorf("file escapes snapshot directory: %s", name)
	}
	return absFull, nil
}

// FormatTimeUTC formats t as RFC3339Nano UTC, or empty.
func FormatTimeUTC(t *time.Time) *string {
	if t == nil {
		return nil
	}
	s := t.UTC().Format(time.RFC3339Nano)
	return &s
}
