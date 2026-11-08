package observatory

import (
	"fmt"
	"sort"
	"strings"
)

// EvidenceClass grades how trustworthy a graph edge is.
type EvidenceClass string

const (
	EvidenceExplicitPublic  EvidenceClass = "explicit_public_field"
	EvidenceDerivedExplicit EvidenceClass = "derived_from_explicit_public_fields"
	EvidenceUserObserved    EvidenceClass = "user_observed"
	EvidenceNameHeuristic   EvidenceClass = "name_heuristic_unverified"
	EvidenceUnresolved      EvidenceClass = "unresolved"
)

// Edge links two entities with provenance.
type Edge struct {
	FromCollection   string        `json:"fromCollection"`
	FromKey          string        `json:"fromKey"`
	ToCollection     string        `json:"toCollection"`
	ToKey            string        `json:"toKey"`
	Relation         string        `json:"relation"`
	SourceCollection string        `json:"sourceCollection"`
	SourceKey        string        `json:"sourceKey"`
	SourceJSONPath   string        `json:"sourceJsonPath"`
	SnapshotID       string        `json:"snapshotId,omitempty"`
	Evidence         EvidenceClass `json:"evidenceClass"`
	Notes            string        `json:"notes,omitempty"`
}

// IsAuthoritative reports whether this edge may be shown as fact.
func (e Edge) IsAuthoritative() bool {
	return e.Evidence == EvidenceExplicitPublic || e.Evidence == EvidenceDerivedExplicit || e.Evidence == EvidenceUserObserved
}

// PromoteHeuristic returns a copy marked unresolved — never promote heuristics.
func PromoteHeuristic(e Edge) Edge {
	if e.Evidence != EvidenceNameHeuristic {
		return e
	}
	out := e
	out.Evidence = EvidenceUnresolved
	out.Notes = strings.TrimSpace(e.Notes + " heuristic join kept unresolved; not promoted to authoritative")
	return out
}

// UnresolvedItem is one dangling or ambiguous relationship.
type UnresolvedItem struct {
	Kind     string        `json:"kind"`
	Entity   string        `json:"entity"`
	Reason   string        `json:"reason"`
	Evidence EvidenceClass `json:"evidenceClass"`
}

// UnresolvedReport groups unresolved graph issues.
type UnresolvedReport struct {
	Items []UnresolvedItem `json:"items"`
	Notes []string         `json:"notes"`
}

// ReportUnresolved collects edges that are heuristic or unresolved.
func ReportUnresolved(edges []Edge) UnresolvedReport {
	var items []UnresolvedItem
	for _, e := range edges {
		switch e.Evidence {
		case EvidenceNameHeuristic, EvidenceUnresolved:
			items = append(items, UnresolvedItem{
				Kind:     e.Relation,
				Entity:   fmt.Sprintf("%s:%s → %s:%s", e.FromCollection, e.FromKey, e.ToCollection, e.ToKey),
				Reason:   firstNonEmpty(e.Notes, string(e.Evidence)),
				Evidence: e.Evidence,
			})
		default:
			if strings.Contains(strings.ToLower(e.Notes), "guess") {
				items = append(items, UnresolvedItem{
					Kind: e.Relation, Entity: e.FromKey, Reason: e.Notes, Evidence: EvidenceNameHeuristic,
				})
			}
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Entity < items[j].Entity })
	return UnresolvedReport{
		Items: items,
		Notes: []string{
			"name-based joins are never promoted to authoritative",
			"unresolved items require explicit public fields or user observations",
		},
	}
}

func firstNonEmpty(parts ...string) string {
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			return p
		}
	}
	return ""
}
