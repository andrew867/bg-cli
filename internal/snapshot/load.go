package snapshot

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func kindToView(kind string) string {
	switch kind {
	case "item-index-json":
		return "itemIndex"
	case "trades-json":
		return "trades"
	case "booth-json":
		return "booth"
	case "mail-json":
		return "mail"
	default:
		return kind
	}
}

// LoadBundle loads a manifest and its associated payload JSON files offline.
func LoadBundle(manifestPath string) (*Bundle, error) {
	absMan, err := filepath.Abs(manifestPath)
	if err != nil {
		return nil, err
	}
	m, err := LoadManifest(absMan)
	if err != nil {
		return nil, err
	}
	baseDir := filepath.Dir(absMan)
	b := &Bundle{
		Manifest: m,
		BaseDir:  baseDir,
		ViewData: map[string]map[string]json.RawMessage{},
		ViewRaw:  map[string]json.RawMessage{},
		Warnings: []string{},
	}

	for _, rec := range m.Files {
		if !rec.OK {
			b.Warnings = append(b.Warnings, fmt.Sprintf("manifest marks %s as failed: %s", rec.Name, rec.Error))
			continue
		}
		path, err := ResolveFilePath(baseDir, rec)
		if err != nil {
			return nil, err
		}
		if err := VerifyFile(path, rec); err != nil {
			b.Warnings = append(b.Warnings, err.Error())
			// Still attempt load if file exists; integrity warning is enough unless missing.
			if _, serr := os.Stat(path); serr != nil {
				return nil, fmt.Errorf("%s: %w", rec.Name, serr)
			}
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		switch rec.Kind {
		case "inventory-json":
			data, _, err := EnvelopeData(raw)
			if err != nil {
				return nil, fmt.Errorf("inventory json: %w", err)
			}
			b.InventoryEnv = data
			b.Inventory = data["inventory"]
		case "profile-json":
			data, _, err := EnvelopeData(raw)
			if err != nil {
				return nil, fmt.Errorf("profile json: %w", err)
			}
			b.ProfileEnv = data
			b.ProfileData = data["profile"]
		case "extended-profile-json":
			data, _, err := EnvelopeData(raw)
			if err != nil {
				b.Warnings = append(b.Warnings, "extended-profile: "+err.Error())
				continue
			}
			b.ExtEnv = data
		case "item-index-json", "trades-json", "booth-json", "mail-json":
			data, _, err := EnvelopeData(raw)
			if err != nil {
				b.Warnings = append(b.Warnings, rec.Kind+": "+err.Error())
				continue
			}
			view := rec.View
			if view == "" {
				view = kindToView(rec.Kind)
			}
			b.ViewData[view] = data
			if v, ok := data[view]; ok {
				b.ViewRaw[view] = v
			} else if v, ok := data["data"]; ok {
				b.ViewRaw[view] = v
			}
		}
	}
	if b.Inventory == nil {
		b.Warnings = append(b.Warnings, "inventory payload missing or failed")
	}
	if b.ProfileData == nil {
		b.Warnings = append(b.Warnings, "profile payload missing or failed")
	}
	return b, nil
}
