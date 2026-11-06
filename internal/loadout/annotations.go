package loadout

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

const AnnotationSchemaVersion = "1.0"

// PetAnnotation is a manual observation for fields missing from the account API.
type PetAnnotation struct {
	ID                               string   `json:"id"`
	UID                              string   `json:"uid,omitempty"`
	InstanceKey                      string   `json:"instanceKey,omitempty"`
	Level                            *int     `json:"level,omitempty"`
	DisplayedLevelDamageBonusPercent *float64 `json:"displayedLevelDamageBonusPercent,omitempty"`
	Charms                           []string `json:"charms,omitempty"`
	Variant                          string   `json:"variant,omitempty"`
	Source                           string   `json:"source,omitempty"`
	Notes                            string   `json:"notes,omitempty"`
}

// AnnotationFile is optional local JSON filling inventory gaps.
type AnnotationFile struct {
	SchemaVersion  string          `json:"schemaVersion"`
	World          string          `json:"world,omitempty"`
	Pets           []PetAnnotation `json:"pets"`
	BestEventPetID string          `json:"bestEventPetId,omitempty"`
	EventTeamLimit *int            `json:"eventTeamLimit,omitempty"`
	EquipBestOrder []string        `json:"equipBestOrder,omitempty"` // observed game order (ids or instance keys)
	Notes          []string        `json:"notes,omitempty"`
}

// LoadAnnotations reads an annotation file. Empty path returns empty annotations.
func LoadAnnotations(path string) (AnnotationFile, error) {
	if strings.TrimSpace(path) == "" {
		return AnnotationFile{SchemaVersion: AnnotationSchemaVersion}, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return AnnotationFile{}, err
	}
	var f AnnotationFile
	if err := json.Unmarshal(b, &f); err != nil {
		return AnnotationFile{}, fmt.Errorf("parse annotations: %w", err)
	}
	if f.SchemaVersion == "" {
		return AnnotationFile{}, fmt.Errorf("annotations missing schemaVersion")
	}
	return f, nil
}

// MatchAnnotation finds the best annotation for an equipped slot.
// Prefer instanceKey/slot keys. UID alone is treated as a stack identity and is only
// used when a single annotation matches that stack key — shared UIDs across multiple
// equipped Butler Penguins must not collapse distinct slots.
func (f AnnotationFile) MatchAnnotation(slot EquippedSlot) (PetAnnotation, bool) {
	if slot.Slot != "" {
		for _, p := range f.Pets {
			if p.InstanceKey != "" && (p.InstanceKey == slot.Slot || p.InstanceKey == "slot:"+slot.Slot) {
				return p, true
			}
		}
	}
	if slot.UID != "" {
		matches := []PetAnnotation{}
		for _, p := range f.Pets {
			if p.UID != "" && p.UID == slot.UID {
				matches = append(matches, p)
			}
			if p.InstanceKey != "" && p.InstanceKey == slot.UID {
				matches = append(matches, p)
			}
		}
		if len(matches) == 1 {
			return matches[0], true
		}
		if len(matches) > 1 {
			return PetAnnotation{ID: slot.ID, Notes: "ambiguous: uid is stack-identity and matches multiple annotations; use instanceKey/slot"}, false
		}
	}
	var byID []PetAnnotation
	for _, p := range f.Pets {
		if strings.EqualFold(p.ID, slot.ID) {
			byID = append(byID, p)
		}
	}
	if len(byID) == 1 {
		return byID[0], true
	}
	if len(byID) > 1 {
		return PetAnnotation{ID: slot.ID, Notes: "ambiguous: multiple annotations for pet id; uid is not a unique instance key — use slot/instanceKey"}, false
	}
	return PetAnnotation{}, false
}
