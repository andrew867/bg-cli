package snapshot

import (
	"fmt"
	"os"
	"path/filepath"
)

// IntegrityReport is the result of verifying a manifest against on-disk bytes.
type IntegrityReport struct {
	OK       bool     `json:"ok"`
	Manifest string   `json:"manifest"`
	Checked  int      `json:"checked"`
	Failures []string `json:"failures,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	MethodID string   `json:"methodId"`
}

// ValidateIntegrity verifies every OK file record's size and SHA-256.
// Failed records in the manifest are reported as warnings, not hard failures,
// unless requireAllRequested is true and any requested view is missing/failed.
func ValidateIntegrity(manifestPath string, requireAllRequested bool) (IntegrityReport, error) {
	m, err := LoadManifest(manifestPath)
	if err != nil {
		return IntegrityReport{}, err
	}
	base := filepath.Dir(manifestPath)
	if _, err := os.Stat(manifestPath); err != nil {
		return IntegrityReport{}, err
	}
	rep := IntegrityReport{
		OK: true, Manifest: manifestPath, MethodID: "snapshot-integrity.v1",
	}
	for _, rec := range m.Files {
		if !rec.OK {
			rep.Warnings = append(rep.Warnings, fmt.Sprintf("%s marked failed: %s", rec.Name, rec.Error))
			if requireAllRequested && rec.RequestStatus == RequestRequested {
				rep.OK = false
				rep.Failures = append(rep.Failures, fmt.Sprintf("requested view %s failed: %s", rec.View, rec.Error))
			}
			continue
		}
		path, err := ResolveFilePath(base, rec)
		if err != nil {
			rep.OK = false
			rep.Failures = append(rep.Failures, err.Error())
			continue
		}
		rep.Checked++
		if err := VerifyFile(path, rec); err != nil {
			rep.OK = false
			rep.Failures = append(rep.Failures, err.Error())
		}
	}
	return rep, nil
}
