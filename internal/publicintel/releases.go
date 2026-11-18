package publicintel

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/andrew867/bg-cli/internal/analysis"
	"github.com/andrew867/bg-cli/internal/jsonutil"
)

// ReleaseSummary is one row from GET /v1/releases (camelCase).
type ReleaseSummary struct {
	Version     ExactInt                   `json:"version"`
	PublishedAt *string                    `json:"publishedAt,omitempty"`
	ImportedAt  *string                    `json:"importedAt,omitempty"`
	Added       ExactInt                   `json:"added"`
	Changed     ExactInt                   `json:"changed"`
	Collections map[string]CollectionDelta `json:"collections,omitempty"`
}

// CollectionDelta is per-collection added/changed counts in the list endpoint.
type CollectionDelta struct {
	Added   ExactInt `json:"added"`
	Changed ExactInt `json:"changed"`
}

// ReleasesListReport is the parsed list page plus provenance.
type ReleasesListReport struct {
	OK           bool                  `json:"ok"`
	Releases     []ReleaseSummary      `json:"releases"`
	Total        ExactInt              `json:"total"`
	Page         ExactInt              `json:"page"`
	PageSize     ExactInt              `json:"pageSize"`
	Completeness analysis.Completeness `json:"completeness"`
	Warnings     []string              `json:"warnings,omitempty"`
	MethodID     string                `json:"methodId"`
}

// ReleaseDetailReport is GET /v1/releases/:version (camelCase throughout).
type ReleaseDetailReport struct {
	OK           bool                         `json:"ok"`
	Version      ExactInt                     `json:"version"`
	PublishedAt  *string                      `json:"publishedAt,omitempty"`
	ImportedAt   *string                      `json:"importedAt,omitempty"`
	Added        ExactInt                     `json:"added"`
	Changed      ExactInt                     `json:"changed"`
	Previous     *ExactInt                    `json:"previous,omitempty"`
	Next         *ExactInt                    `json:"next,omitempty"`
	Collections  map[string]ReleaseCollection `json:"collections,omitempty"`
	Completeness analysis.Completeness        `json:"completeness"`
	MethodID     string                       `json:"methodId"`
}

type ReleaseCollection struct {
	Added   []ReleaseItem `json:"added,omitempty"`
	Changed []ChangedItem `json:"changed,omitempty"`
}

type ReleaseItem struct {
	ConfigName  string  `json:"configName"`
	DisplayName string  `json:"displayName"`
	Category    *string `json:"category,omitempty"`
	Rarity      *string `json:"rarity,omitempty"`
	Data        string  `json:"data"`
	Icon        *string `json:"icon,omitempty"`
}

type ChangedItem struct {
	ReleaseItem
	Changes []FieldChange `json:"changes,omitempty"`
	More    ExactInt      `json:"more"`
}

type FieldChange struct {
	Path   []any `json:"path"`
	Before any   `json:"before,omitempty"`
	After  any   `json:"after,omitempty"`
}

// ItemChangeRow is one row from GET /v1/items/changes (camelCase, hasMore pagination).
type ItemChangeRow struct {
	Version     ExactInt `json:"version"`
	PublishedAt *string  `json:"publishedAt,omitempty"`
	ImportedAt  *string  `json:"importedAt,omitempty"`
	Collection  string   `json:"collection"`
	Change      string   `json:"change"`
	ReleaseItem
}

// ItemChangesPage is one page of the catch-up feed.
type ItemChangesPage struct {
	Since         map[string]any  `json:"since"`
	LatestVersion ExactInt        `json:"latestVersion"`
	Changes       []ItemChangeRow `json:"changes"`
	Total         ExactInt        `json:"total"`
	Page          ExactInt        `json:"page"`
	PageSize      ExactInt        `json:"pageSize"`
	HasMore       bool            `json:"hasMore"`
}

// ItemChangesReport aggregates validated pages.
type ItemChangesReport struct {
	OK           bool                  `json:"ok"`
	Pages        []ItemChangesPage     `json:"pages"`
	RowCount     ExactInt              `json:"rowCount"`
	Completeness analysis.Completeness `json:"completeness"`
	Warnings     []string              `json:"warnings,omitempty"`
	MethodID     string                `json:"methodId"`
}

// ChangesCheckpoint stores bot polling state; advance only after full pagination validates.
type ChangesCheckpoint struct {
	SinceVersion  ExactInt `json:"sinceVersion"`
	LatestVersion ExactInt `json:"latestVersion"`
	ValidatedRows ExactInt `json:"validatedRows"`
}

// ReleasesDiff compares two release list snapshots by version.
type ReleasesDiff struct {
	OK              bool           `json:"ok"`
	OnlyInA         []ExactInt     `json:"onlyInA"`
	OnlyInB         []ExactInt     `json:"onlyInB"`
	ChangedVersions []VersionDelta `json:"changedVersions"`
	MethodID        string         `json:"methodId"`
}

type VersionDelta struct {
	Version ExactInt `json:"version"`
	Added   ExactInt `json:"addedDelta"`
	Changed ExactInt `json:"changedDelta"`
}

// ParseReleasesList parses GET /v1/releases data (camelCase: releases, total, page, pageSize).
func ParseReleasesList(body []byte) (ReleasesListReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return ReleasesListReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return ReleasesListReport{}, err
	}
	rep := ReleasesListReport{
		OK:           true,
		MethodID:     "publicintel-releases-list.v1",
		Completeness: analysis.Complete,
	}
	if v, ok := raw["releases"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				rs, err := parseReleaseSummary(item)
				if err != nil {
					return rep, err
				}
				rep.Releases = append(rep.Releases, rs)
			}
		}
	}
	rep.Total = parseFieldExact(raw, "total")
	rep.Page = parseFieldExact(raw, "page")
	rep.PageSize = parseFieldExact(raw, "pageSize")
	if total, err := rep.Total.Int64(); err == nil && int64(len(rep.Releases)) < total {
		rep.Completeness = analysis.Partial
		rep.Warnings = append(rep.Warnings, "page holds fewer releases than data.total")
	}
	return rep, nil
}

func parseReleaseSummary(item json.RawMessage) (ReleaseSummary, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ReleaseSummary{}, err
	}
	rs := ReleaseSummary{
		Version:     parseFieldExact(m, "version"),
		Added:       parseFieldExact(m, "added"),
		Changed:     parseFieldExact(m, "changed"),
		Collections: map[string]CollectionDelta{},
	}
	if v, ok := m["publishedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rs.PublishedAt = &s
		}
	}
	if v, ok := m["importedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rs.ImportedAt = &s
		}
	}
	if v, ok := m["collections"]; ok {
		var cols map[string]json.RawMessage
		if jsonutil.Decode(v, &cols) == nil {
			for name, blob := range cols {
				var cm map[string]json.RawMessage
				if jsonutil.Decode(blob, &cm) != nil {
					continue
				}
				rs.Collections[name] = CollectionDelta{
					Added:   parseFieldExact(cm, "added"),
					Changed: parseFieldExact(cm, "changed"),
				}
			}
		}
	}
	return rs, nil
}

// ParseReleaseDetail parses GET /v1/releases/:version data.
func ParseReleaseDetail(body []byte) (ReleaseDetailReport, error) {
	data, err := unwrapData(body)
	if err != nil {
		return ReleaseDetailReport{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return ReleaseDetailReport{}, err
	}
	rep := ReleaseDetailReport{
		OK:           true,
		MethodID:     "publicintel-release-detail.v1",
		Completeness: analysis.Complete,
		Collections:  map[string]ReleaseCollection{},
		Version:      parseFieldExact(raw, "version"),
		Added:        parseFieldExact(raw, "added"),
		Changed:      parseFieldExact(raw, "changed"),
	}
	if v, ok := raw["publishedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.PublishedAt = &s
		}
	}
	if v, ok := raw["importedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			rep.ImportedAt = &s
		}
	}
	if v, ok := raw["previous"]; ok && string(v) != "null" {
		ei := parseExactIntJSON(v)
		rep.Previous = &ei
	}
	if v, ok := raw["next"]; ok && string(v) != "null" {
		ei := parseExactIntJSON(v)
		rep.Next = &ei
	}
	if v, ok := raw["collections"]; ok {
		var cols map[string]json.RawMessage
		if jsonutil.Decode(v, &cols) == nil {
			for name, blob := range cols {
				rc, err := parseReleaseCollection(blob)
				if err != nil {
					return rep, err
				}
				rep.Collections[name] = rc
			}
		}
	}
	return rep, nil
}

func parseReleaseCollection(blob json.RawMessage) (ReleaseCollection, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(blob, &m); err != nil {
		return ReleaseCollection{}, err
	}
	out := ReleaseCollection{}
	if v, ok := m["added"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				ri, err := parseReleaseItem(item)
				if err != nil {
					return out, err
				}
				out.Added = append(out.Added, ri)
			}
		}
	}
	if v, ok := m["changed"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				ci, err := parseChangedItem(item)
				if err != nil {
					return out, err
				}
				out.Changed = append(out.Changed, ci)
			}
		}
	}
	return out, nil
}

func parseReleaseItem(item json.RawMessage) (ReleaseItem, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ReleaseItem{}, err
	}
	ri := ReleaseItem{}
	if v, ok := m["configName"]; ok {
		_ = json.Unmarshal(v, &ri.ConfigName)
	}
	if v, ok := m["displayName"]; ok {
		_ = json.Unmarshal(v, &ri.DisplayName)
	}
	if v, ok := m["category"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			ri.Category = &s
		}
	}
	if v, ok := m["rarity"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			ri.Rarity = &s
		}
	}
	if v, ok := m["data"]; ok {
		_ = json.Unmarshal(v, &ri.Data)
	}
	if v, ok := m["icon"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			ri.Icon = &s
		}
	}
	return ri, nil
}

func parseChangedItem(item json.RawMessage) (ChangedItem, error) {
	ri, err := parseReleaseItem(item)
	if err != nil {
		return ChangedItem{}, err
	}
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ChangedItem{}, err
	}
	ci := ChangedItem{ReleaseItem: ri, More: parseFieldExact(m, "more")}
	if v, ok := m["changes"]; ok && string(v) != "null" {
		var arr []FieldChange
		if jsonutil.Decode(v, &arr) == nil {
			ci.Changes = arr
		}
	}
	return ci, nil
}

// ParseItemChanges parses one GET /v1/items/changes page (hasMore, not HasMore).
func ParseItemChanges(body []byte) (ItemChangesPage, error) {
	data, err := unwrapData(body)
	if err != nil {
		return ItemChangesPage{}, err
	}
	var raw map[string]json.RawMessage
	if err := jsonutil.Decode(data, &raw); err != nil {
		return ItemChangesPage{}, err
	}
	page := ItemChangesPage{
		LatestVersion: parseFieldExact(raw, "latestVersion"),
		Total:         parseFieldExact(raw, "total"),
		Page:          parseFieldExact(raw, "page"),
		PageSize:      parseFieldExact(raw, "pageSize"),
	}
	if v, ok := raw["since"]; ok {
		_ = jsonutil.Decode(v, &page.Since)
	}
	if v, ok := raw["hasMore"]; ok {
		_ = json.Unmarshal(v, &page.HasMore)
	}
	if v, ok := raw["changes"]; ok {
		var arr []json.RawMessage
		if jsonutil.Decode(v, &arr) == nil {
			for _, item := range arr {
				row, err := parseItemChangeRow(item)
				if err != nil {
					return page, err
				}
				page.Changes = append(page.Changes, row)
			}
		}
	}
	return page, nil
}

func parseItemChangeRow(item json.RawMessage) (ItemChangeRow, error) {
	var m map[string]json.RawMessage
	if err := jsonutil.Decode(item, &m); err != nil {
		return ItemChangeRow{}, err
	}
	ri, err := parseReleaseItem(item)
	if err != nil {
		return ItemChangeRow{}, err
	}
	row := ItemChangeRow{
		Version:     parseFieldExact(m, "version"),
		Collection:  stringField(m, "collection"),
		Change:      stringField(m, "change"),
		ReleaseItem: ri,
	}
	if v, ok := m["publishedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			row.PublishedAt = &s
		}
	}
	if v, ok := m["importedAt"]; ok && string(v) != "null" {
		var s string
		if json.Unmarshal(v, &s) == nil {
			row.ImportedAt = &s
		}
	}
	return row, nil
}

// BuildItemChangesReport validates a pagination run.
func BuildItemChangesReport(pages []ItemChangesPage) ItemChangesReport {
	rep := ItemChangesReport{
		OK:       true,
		Pages:    pages,
		MethodID: "publicintel-items-changes.v1",
		RowCount: ExactInt{Text: "0"},
	}
	var warnings []string
	if len(pages) == 0 {
		rep.Completeness = analysis.IncompletePagination
		rep.Warnings = []string{"no pages"}
		return rep
	}
	totalExpected := pages[0].Total
	seen := 0
	for i, p := range pages {
		seen += len(p.Changes)
		if !p.Total.ParseOK || !totalExpected.ParseOK || p.Total.Text != totalExpected.Text {
			warnings = append(warnings, fmt.Sprintf("page %d total mismatch", i+1))
		}
		if !p.LatestVersion.ParseOK || (i > 0 && p.LatestVersion.Text != pages[0].LatestVersion.Text) {
			warnings = append(warnings, fmt.Sprintf("page %d latestVersion mismatch", i+1))
		}
		if i < len(pages)-1 && !p.HasMore {
			warnings = append(warnings, fmt.Sprintf("page %d missing hasMore before final page", i+1))
		}
		if i == len(pages)-1 && p.HasMore {
			warnings = append(warnings, "final page still has hasMore=true")
		}
	}
	rep.RowCount = ExactInt{Text: fmt.Sprintf("%d", seen), ParseOK: true, Value: int64Ptr(int64(seen))}
	if len(warnings) > 0 {
		rep.Completeness = analysis.IncompletePagination
		rep.Warnings = warnings
		rep.OK = false
	} else if te, err := totalExpected.Int64(); err == nil && int64(seen) != te {
		rep.Completeness = analysis.IncompletePagination
		rep.Warnings = append(rep.Warnings, "row count does not match total")
		rep.OK = false
	} else {
		rep.Completeness = analysis.Complete
	}
	return rep
}

// ValidateItemChangesPages checks pagination integrity without merging.
func ValidateItemChangesPages(pages []ItemChangesPage) []string {
	return BuildItemChangesReport(pages).Warnings
}

// AdvanceChangesCheckpoint advances only when every page passed validation.
func AdvanceChangesCheckpoint(cp ChangesCheckpoint, pages []ItemChangesPage) (ChangesCheckpoint, bool) {
	rep := BuildItemChangesReport(pages)
	if !rep.OK || rep.Completeness != analysis.Complete {
		return cp, false
	}
	if len(pages) == 0 {
		return cp, false
	}
	last := pages[len(pages)-1]
	out := ChangesCheckpoint{
		SinceVersion:  last.LatestVersion,
		LatestVersion: last.LatestVersion,
		ValidatedRows: rep.RowCount,
	}
	return out, true
}

// DiffReleases compares two list reports by version number.
func DiffReleases(a, b ReleasesListReport) ReleasesDiff {
	byA := map[string]ReleaseSummary{}
	byB := map[string]ReleaseSummary{}
	for _, r := range a.Releases {
		byA[r.Version.Text] = r
	}
	for _, r := range b.Releases {
		byB[r.Version.Text] = r
	}
	diff := ReleasesDiff{OK: true, MethodID: "publicintel-releases-diff.v1"}
	for k, r := range byA {
		if _, ok := byB[k]; !ok {
			diff.OnlyInA = append(diff.OnlyInA, r.Version)
		}
	}
	for k, r := range byB {
		if _, ok := byA[k]; !ok {
			diff.OnlyInB = append(diff.OnlyInB, r.Version)
		}
	}
	for k, ra := range byA {
		rb, ok := byB[k]
		if !ok {
			continue
		}
		if ra.Added.Text != rb.Added.Text || ra.Changed.Text != rb.Changed.Text {
			ad := ExactInt{Text: "0", ParseOK: true, Value: int64Ptr(0)}
			cd := ExactInt{Text: "0", ParseOK: true, Value: int64Ptr(0)}
			if av, err := ra.Added.Int64(); err == nil {
				if bv, err2 := rb.Added.Int64(); err2 == nil {
					d := bv - av
					ad = ExactInt{Text: fmt.Sprintf("%d", d), ParseOK: true, Value: &d}
				}
			}
			if av, err := ra.Changed.Int64(); err == nil {
				if bv, err2 := rb.Changed.Int64(); err2 == nil {
					d := bv - av
					cd = ExactInt{Text: fmt.Sprintf("%d", d), ParseOK: true, Value: &d}
				}
			}
			diff.ChangedVersions = append(diff.ChangedVersions, VersionDelta{
				Version: ra.Version, Added: ad, Changed: cd,
			})
		}
	}
	sort.Slice(diff.OnlyInA, func(i, j int) bool { return diff.OnlyInA[i].Text < diff.OnlyInA[j].Text })
	sort.Slice(diff.OnlyInB, func(i, j int) bool { return diff.OnlyInB[i].Text < diff.OnlyInB[j].Text })
	sort.Slice(diff.ChangedVersions, func(i, j int) bool {
		return diff.ChangedVersions[i].Version.Text < diff.ChangedVersions[j].Version.Text
	})
	return diff
}

func parseFieldExact(m map[string]json.RawMessage, key string) ExactInt {
	if v, ok := m[key]; ok {
		return parseExactIntJSON(v)
	}
	return ExactInt{}
}

func parseExactIntJSON(raw json.RawMessage) ExactInt {
	return ParseExactIntJSON(raw)
}

func stringField(m map[string]json.RawMessage, key string) string {
	if v, ok := m[key]; ok {
		var s string
		if json.Unmarshal(v, &s) == nil {
			return s
		}
	}
	return ""
}

func int64Ptr(v int64) *int64 { return &v }
