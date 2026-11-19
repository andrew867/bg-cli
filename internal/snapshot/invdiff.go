package snapshot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
)

const (
	// InventoryDiffSchema is the inventory-diff.json document version.
	InventoryDiffSchema = "1.0"

	DiffStatusOK          = "ok"
	DiffStatusUnavailable = "unavailable"

	// Classifications describe a net quantity change. They do not name a cause.
	ClassAddedToInventory     = "added to inventory"
	ClassRemovedFromInventory = "removed from inventory"
	ClassNetInventoryChange   = "net inventory change"
)

// IdentityExcludedFields are stripped before a stable identity is formed.
// _am and count are quantities. stackKey embeds that quantity, so it is not
// an identity. The remaining names are market or display metadata. A change
// to only those fields is not an inventory change. Unknown rawData fields,
// including createdAt, stay in the identity.
var IdentityExcludedFields = []string{
	"_am",
	"count",
	"stackKey",
	"rap",
	"rapApproximate",
	"exists",
	"icon",
	"goldenIcon",
	"displayName",
	"rarity",
	"rarityNumber",
}

func excludedIdentityField(key string) bool {
	switch key {
	case "_am", "count", "stackKey", "rap", "rapApproximate", "exists", "icon", "goldenIcon", "displayName", "rarity", "rarityNumber":
		return true
	default:
		return false
	}
}

// InventoryDiff is the exact previous-to-current inventory comparison.
// Quantities and deltas are decimal integer strings. Missing data is
// status "unavailable" and is never stored as zero.
type InventoryDiff struct {
	SchemaVersion     string                `json:"schemaVersion"`
	Status            string                `json:"status"`
	Reason            string                `json:"reason,omitempty"`
	Baseline          *InventoryRef         `json:"baseline,omitempty"`
	Current           *InventoryRef         `json:"current,omitempty"`
	Aggregates        InventoryAggregateSet `json:"aggregates"`
	Counts            *ChangeCounts         `json:"counts,omitempty"`
	PetSpeciesChanges []SpeciesChange       `json:"petSpeciesChanges"`
	ItemChanges       []ItemChange          `json:"itemChanges"`
	Diagnostics       *InventoryDiagnostics `json:"diagnostics,omitempty"`
	Warnings          []string              `json:"warnings"`
}

// InventoryRef names one side of the comparison.
type InventoryRef struct {
	File       string `json:"file,omitempty"`
	ExportedAt string `json:"exportedAt,omitempty"`
}

// InventoryAggregateSet holds pet and inventory totals for both sides.
// Golden, Rainbow, and Shiny overlap: a Golden Shiny pet counts in Golden
// and in Shiny, and once in Pet quantity. Those three totals must not be
// added together and compared with the pet total.
type InventoryAggregateSet struct {
	PetQuantity   CountPair `json:"petQuantity"`
	PetRows       CountPair `json:"petRows"`
	Golden        CountPair `json:"golden"`
	Rainbow       CountPair `json:"rainbow"`
	Shiny         CountPair `json:"shiny"`
	Huge          CountPair `json:"huge"`
	InventoryRows CountPair `json:"inventoryRows"`
}

// CountPair is an exact integer comparison. Delta is current minus previous.
type CountPair struct {
	Previous string `json:"previous,omitempty"`
	Current  string `json:"current,omitempty"`
	Delta    string `json:"delta,omitempty"`
	Status   string `json:"status,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// ChangeCounts counts identities whose quantity differs.
// Changed includes Added and Removed. Net changes are Changed - Added - Removed.
type ChangeCounts struct {
	Changed int `json:"changed"`
	Added   int `json:"added"`
	Removed int `json:"removed"`
}

// SpeciesChange is the net pet quantity change for one pet id.
// Variant and unique-instance fields are ignored here. The identity list
// still keeps them.
type SpeciesChange struct {
	ID             string `json:"id"`
	DisplayName    string `json:"displayName,omitempty"`
	Previous       string `json:"previous"`
	Current        string `json:"current"`
	Delta          string `json:"delta"`
	Classification string `json:"classification"`
}

// ItemChange is one stable identity whose quantity differs.
type ItemChange struct {
	Class          string `json:"class"`
	Collection     string `json:"collection,omitempty"`
	ID             string `json:"id"`
	DisplayName    string `json:"displayName,omitempty"`
	Identity       string `json:"identity"`
	Previous       string `json:"previous"`
	Current        string `json:"current"`
	Delta          string `json:"delta"`
	Classification string `json:"classification"`
	Variant        string `json:"variant,omitempty"`
	Golden         bool   `json:"golden"`
	Rainbow        bool   `json:"rainbow"`
	Shiny          bool   `json:"shiny"`
	PT             string `json:"pt,omitempty"`
	SH             string `json:"sh,omitempty"`
	Huge           *bool  `json:"huge,omitempty"`
}

// InventoryDiagnostics compares net pet inventory deltas with profile
// "Opened" counters for the same previous interval. A mismatch is not an
// error and is not treated as corruption.
type InventoryDiagnostics struct {
	PetQuantityDelta   string `json:"petQuantityDelta,omitempty"`
	EggsOpenedDelta    string `json:"eggsOpenedDelta,omitempty"`
	PetMinusEggs       string `json:"petMinusEggs,omitempty"`
	GoldenDelta        string `json:"goldenDelta,omitempty"`
	GoldenOpenedDelta  string `json:"goldenOpenedDelta,omitempty"`
	GoldenMinusOpened  string `json:"goldenMinusOpened,omitempty"`
	RainbowDelta       string `json:"rainbowDelta,omitempty"`
	RainbowOpenedDelta string `json:"rainbowOpenedDelta,omitempty"`
	RainbowMinusOpened string `json:"rainbowMinusOpened,omitempty"`
	ShinyDelta         string `json:"shinyDelta,omitempty"`
	ShinyOpenedDelta   string `json:"shinyOpenedDelta,omitempty"`
	ShinyMinusOpened   string `json:"shinyMinusOpened,omitempty"`
}

// InventoryDiffOptions carries file names and profile counters already on hand.
// The comparison itself does not read the network or other snapshot files.
type InventoryDiffOptions struct {
	BaselineFile      string
	BaselineAt        string
	CurrentFile       string
	CurrentAt         string
	PrevStats         map[string]string
	CurStats          map[string]string
	PrevStatsKnown    bool
	CurStatsKnown     bool
	UnavailableReason string
}

// InventoryMeasure is the current inventory's stored aggregate.
// Huge is set only when at least one pet row has an explicit huge boolean.
type InventoryMeasure struct {
	OK            bool
	Reason        string
	PetQuantity   string
	PetRows       string
	Golden        string
	Rainbow       string
	Shiny         string
	Huge          string
	HugeKnown     bool
	InventoryRows string
}

// MeasureInventory totals one inventory object. A missing or malformed
// payload is not reported as zero.
func MeasureInventory(inv json.RawMessage) InventoryMeasure {
	idx, reason := indexInventory(inv, "current")
	if reason != "" {
		return InventoryMeasure{Reason: reason}
	}
	m := InventoryMeasure{
		OK:            true,
		PetQuantity:   idx.petQuantity.String(),
		PetRows:       idx.petRows.String(),
		Golden:        idx.golden.String(),
		Rainbow:       idx.rainbow.String(),
		Shiny:         idx.shiny.String(),
		InventoryRows: idx.inventoryRows.String(),
		HugeKnown:     idx.hugeKnown,
	}
	if idx.hugeKnown {
		m.Huge = idx.huge.String()
	}
	return m
}

// DiffInventories compares two inventory objects ({"items":[...]}).
// Identity is class + collection + canonical raw data with quantity and
// market fields removed. Rows that share an identity have their quantities
// summed before the comparison. Field order does not matter.
func DiffInventories(prev, current json.RawMessage, opt InventoryDiffOptions) InventoryDiff {
	diff := newInventoryDiff()
	applyRefs(&diff, opt)
	if len(bytes.TrimSpace(current)) == 0 || string(bytes.TrimSpace(current)) == "null" {
		diff.Status = DiffStatusUnavailable
		diff.Reason = "current inventory is unavailable"
		diff.Aggregates = unavailableAggregates("current inventory is unavailable")
		return diff
	}
	cur, curReason := indexInventory(current, "current")
	if curReason != "" {
		diff.Status = DiffStatusUnavailable
		diff.Reason = curReason
		diff.Aggregates = unavailableAggregates(curReason)
		return diff
	}
	if opt.UnavailableReason != "" || len(bytes.TrimSpace(prev)) == 0 || string(bytes.TrimSpace(prev)) == "null" {
		reason := opt.UnavailableReason
		if reason == "" {
			reason = "no previous snapshot"
		}
		return currentOnlyDiff(diff, cur, reason)
	}
	old, oldReason := indexInventory(prev, "previous")
	if oldReason != "" {
		out := currentOnlyDiff(diff, cur, oldReason)
		out.Warnings = mergeWarnings(cur.warnings, nil)
		return out
	}
	return compareIndexes(diff, old, cur, opt)
}

// WriteInventoryDiff writes the diff as deterministic indented JSON.
func WriteInventoryDiff(w io.Writer, diff InventoryDiff) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.SetEscapeHTML(false)
	return enc.Encode(diff)
}

func newInventoryDiff() InventoryDiff {
	return InventoryDiff{
		SchemaVersion:     InventoryDiffSchema,
		Status:            DiffStatusOK,
		PetSpeciesChanges: []SpeciesChange{},
		ItemChanges:       []ItemChange{},
		Warnings:          []string{},
	}
}

func applyRefs(diff *InventoryDiff, opt InventoryDiffOptions) {
	if opt.CurrentFile != "" || opt.CurrentAt != "" {
		diff.Current = &InventoryRef{File: opt.CurrentFile, ExportedAt: opt.CurrentAt}
	}
	if opt.BaselineFile != "" || opt.BaselineAt != "" {
		diff.Baseline = &InventoryRef{File: opt.BaselineFile, ExportedAt: opt.BaselineAt}
	}
}

func unavailableAggregates(reason string) InventoryAggregateSet {
	p := CountPair{Status: DiffStatusUnavailable, Reason: reason}
	return InventoryAggregateSet{
		PetQuantity: p, PetRows: p, Golden: p, Rainbow: p, Shiny: p, Huge: p, InventoryRows: p,
	}
}

func currentOnlyPair(cur *big.Int, reason string) CountPair {
	return CountPair{Current: cur.String(), Status: DiffStatusUnavailable, Reason: reason}
}

func hugeCurrent(cur *inventoryIndex, reason string) CountPair {
	if cur == nil || !cur.hugeKnown {
		return CountPair{Status: DiffStatusUnavailable, Reason: "huge classification is absent"}
	}
	return CountPair{Current: cur.huge.String(), Status: DiffStatusUnavailable, Reason: reason}
}

func currentOnlyDiff(base InventoryDiff, cur *inventoryIndex, reason string) InventoryDiff {
	base.Status = DiffStatusUnavailable
	base.Reason = reason
	base.Aggregates = InventoryAggregateSet{
		PetQuantity:   currentOnlyPair(cur.petQuantity, reason),
		PetRows:       currentOnlyPair(cur.petRows, reason),
		Golden:        currentOnlyPair(cur.golden, reason),
		Rainbow:       currentOnlyPair(cur.rainbow, reason),
		Shiny:         currentOnlyPair(cur.shiny, reason),
		Huge:          hugeCurrent(cur, reason),
		InventoryRows: currentOnlyPair(cur.inventoryRows, reason),
	}
	base.Warnings = mergeWarnings(cur.warnings, nil)
	base.PetSpeciesChanges = []SpeciesChange{}
	base.ItemChanges = []ItemChange{}
	base.Counts = nil
	return base
}

func fullPair(prev, cur *big.Int) CountPair {
	delta := new(big.Int).Sub(cur, prev)
	return CountPair{Previous: prev.String(), Current: cur.String(), Delta: delta.String()}
}

func hugePair(prev, cur *inventoryIndex) CountPair {
	if prev.hugeKnown && cur.hugeKnown {
		return fullPair(prev.huge, cur.huge)
	}
	p := CountPair{Status: DiffStatusUnavailable, Reason: "huge classification is absent"}
	if prev.hugeKnown {
		p.Previous = prev.huge.String()
	}
	if cur.hugeKnown {
		p.Current = cur.huge.String()
	}
	return p
}

func compareIndexes(base InventoryDiff, prev, cur *inventoryIndex, opt InventoryDiffOptions) InventoryDiff {
	base.Status = DiffStatusOK
	base.Aggregates = InventoryAggregateSet{
		PetQuantity:   fullPair(prev.petQuantity, cur.petQuantity),
		PetRows:       fullPair(prev.petRows, cur.petRows),
		Golden:        fullPair(prev.golden, cur.golden),
		Rainbow:       fullPair(prev.rainbow, cur.rainbow),
		Shiny:         fullPair(prev.shiny, cur.shiny),
		Huge:          hugePair(prev, cur),
		InventoryRows: fullPair(prev.inventoryRows, cur.inventoryRows),
	}
	base.Warnings = mergeWarnings(prev.warnings, cur.warnings)
	base.ItemChanges = identityChanges(prev, cur)
	base.PetSpeciesChanges = speciesChanges(prev, cur)
	added, removed := 0, 0
	for _, ch := range base.ItemChanges {
		switch ch.Classification {
		case ClassAddedToInventory:
			added++
		case ClassRemovedFromInventory:
			removed++
		}
	}
	base.Counts = &ChangeCounts{Changed: len(base.ItemChanges), Added: added, Removed: removed}
	fillDiagnostics(&base, opt)
	return base
}

func identityChanges(prev, cur *inventoryIndex) []ItemChange {
	keys := make([]string, 0, len(prev.items)+len(cur.items))
	seen := map[string]struct{}{}
	for k := range prev.items {
		seen[k] = struct{}{}
		keys = append(keys, k)
	}
	for k := range cur.items {
		if _, ok := seen[k]; ok {
			continue
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	changes := make([]ItemChange, 0)
	for _, key := range keys {
		old := prev.items[key]
		neu := cur.items[key]
		prevQ := qtyOrZero(old)
		curQ := qtyOrZero(neu)
		if prevQ.Cmp(curQ) == 0 {
			continue
		}
		meta := neu
		if meta == nil {
			meta = old
		}
		delta := new(big.Int).Sub(curQ, prevQ)
		changes = append(changes, ItemChange{
			Class:          meta.class,
			Collection:     meta.collection,
			ID:             meta.id,
			DisplayName:    meta.displayName,
			Identity:       meta.identity,
			Previous:       prevQ.String(),
			Current:        curQ.String(),
			Delta:          delta.String(),
			Classification: classifyQty(prevQ, curQ),
			Variant:        meta.variant.Label,
			Golden:         meta.variant.Golden,
			Rainbow:        meta.variant.Rainbow,
			Shiny:          meta.variant.Shiny,
			PT:             meta.variant.PT,
			SH:             meta.variant.SH,
			Huge:           meta.huge,
		})
	}
	sort.Slice(changes, func(i, j int) bool {
		if changes[i].Class != changes[j].Class {
			return changes[i].Class < changes[j].Class
		}
		if changes[i].ID != changes[j].ID {
			return changes[i].ID < changes[j].ID
		}
		return changes[i].Identity < changes[j].Identity
	})
	return changes
}

func speciesChanges(prev, cur *inventoryIndex) []SpeciesChange {
	type acc struct {
		id   string
		name string
		prev *big.Int
		cur  *big.Int
	}
	byID := map[string]*acc{}
	add := func(idx *inventoryIndex, current bool) {
		for _, it := range idx.items {
			if !strings.EqualFold(it.class, "Pet") {
				continue
			}
			s := byID[it.id]
			if s == nil {
				s = &acc{id: it.id, prev: new(big.Int), cur: new(big.Int)}
				byID[it.id] = s
			}
			if current {
				s.cur.Add(s.cur, it.qty)
				if it.displayName != "" {
					s.name = it.displayName
				}
			} else {
				s.prev.Add(s.prev, it.qty)
				if s.name == "" {
					s.name = it.displayName
				}
			}
		}
	}
	add(prev, false)
	add(cur, true)
	out := make([]SpeciesChange, 0)
	for _, s := range byID {
		if s.prev.Cmp(s.cur) == 0 {
			continue
		}
		delta := new(big.Int).Sub(s.cur, s.prev)
		name := s.name
		if name == "" {
			name = s.id
		}
		out = append(out, SpeciesChange{
			ID:             s.id,
			DisplayName:    name,
			Previous:       s.prev.String(),
			Current:        s.cur.String(),
			Delta:          delta.String(),
			Classification: classifyQty(s.prev, s.cur),
		})
	}
	sort.Slice(out, func(i, j int) bool {
		di := ParseExactIntString(out[i].Delta).Int
		dj := ParseExactIntString(out[j].Delta).Int
		if di == nil {
			di = new(big.Int)
		}
		if dj == nil {
			dj = new(big.Int)
		}
		cmp := new(big.Int).Abs(di).Cmp(new(big.Int).Abs(dj))
		if cmp != 0 {
			return cmp > 0
		}
		if out[i].DisplayName != out[j].DisplayName {
			return out[i].DisplayName < out[j].DisplayName
		}
		return out[i].ID < out[j].ID
	})
	return out
}

func classifyQty(prev, cur *big.Int) string {
	if prev.Sign() == 0 && cur.Sign() > 0 {
		return ClassAddedToInventory
	}
	if prev.Sign() > 0 && cur.Sign() == 0 {
		return ClassRemovedFromInventory
	}
	return ClassNetInventoryChange
}

func qtyOrZero(it *indexedItem) *big.Int {
	if it == nil || it.qty == nil {
		return new(big.Int)
	}
	return it.qty
}

func fillDiagnostics(diff *InventoryDiff, opt InventoryDiffOptions) {
	diag := &InventoryDiagnostics{}
	set := false
	if eggs, ok := statDelta(opt, "Eggs Opened"); ok && diff.Aggregates.PetQuantity.Delta != "" {
		if gap, ok := subText(diff.Aggregates.PetQuantity.Delta, eggs); ok {
			diag.PetQuantityDelta = diff.Aggregates.PetQuantity.Delta
			diag.EggsOpenedDelta = eggs
			diag.PetMinusEggs = gap
			set = true
		}
	}
	if opened, ok := statDelta(opt, "Golden Pets Opened"); ok && diff.Aggregates.Golden.Delta != "" {
		if gap, ok := subText(diff.Aggregates.Golden.Delta, opened); ok {
			diag.GoldenDelta = diff.Aggregates.Golden.Delta
			diag.GoldenOpenedDelta = opened
			diag.GoldenMinusOpened = gap
			set = true
		}
	}
	if opened, ok := statDelta(opt, "Rainbow Pets Opened"); ok && diff.Aggregates.Rainbow.Delta != "" {
		if gap, ok := subText(diff.Aggregates.Rainbow.Delta, opened); ok {
			diag.RainbowDelta = diff.Aggregates.Rainbow.Delta
			diag.RainbowOpenedDelta = opened
			diag.RainbowMinusOpened = gap
			set = true
		}
	}
	if opened, ok := statDelta(opt, "Shiny Pets Opened"); ok && diff.Aggregates.Shiny.Delta != "" {
		if gap, ok := subText(diff.Aggregates.Shiny.Delta, opened); ok {
			diag.ShinyDelta = diff.Aggregates.Shiny.Delta
			diag.ShinyOpenedDelta = opened
			diag.ShinyMinusOpened = gap
			set = true
		}
	}
	if set {
		diff.Diagnostics = diag
	}
}

func statDelta(opt InventoryDiffOptions, name string) (string, bool) {
	if !opt.PrevStatsKnown || !opt.CurStatsKnown {
		return "", false
	}
	prev, ok1 := lookupExactStat(opt.PrevStats, name)
	cur, ok2 := lookupExactStat(opt.CurStats, name)
	if !ok1 || !ok2 {
		return "", false
	}
	return subText(cur, prev)
}

func lookupExactStat(stats map[string]string, name string) (string, bool) {
	if stats == nil {
		return "", false
	}
	if v, ok := stats[name]; ok {
		return exactDecimal(v)
	}
	for k, v := range stats {
		if strings.EqualFold(k, name) {
			return exactDecimal(v)
		}
	}
	return "", false
}

func exactDecimal(v string) (string, bool) {
	e := ParseExactIntString(v)
	if !e.ParseOK || e.Approximate || e.Int == nil {
		return "", false
	}
	return e.Text, true
}

func subText(cur, prev string) (string, bool) {
	c, ok1 := exactDecimal(cur)
	p, ok2 := exactDecimal(prev)
	if !ok1 || !ok2 {
		return "", false
	}
	delta := new(big.Int).Sub(ParseExactIntString(c).Int, ParseExactIntString(p).Int)
	return delta.String(), true
}

type inventoryIndex struct {
	items         map[string]*indexedItem
	petQuantity   *big.Int
	petRows       *big.Int
	golden        *big.Int
	rainbow       *big.Int
	shiny         *big.Int
	huge          *big.Int
	hugeKnown     bool
	inventoryRows *big.Int
	warnings      []string
}

type indexedItem struct {
	class       string
	collection  string
	id          string
	displayName string
	identity    string
	variant     output.ItemVariant
	huge        *bool
	qty         *big.Int
}

func newInventoryIndex() *inventoryIndex {
	return &inventoryIndex{
		items:         map[string]*indexedItem{},
		petQuantity:   new(big.Int),
		petRows:       new(big.Int),
		golden:        new(big.Int),
		rainbow:       new(big.Int),
		shiny:         new(big.Int),
		huge:          new(big.Int),
		inventoryRows: new(big.Int),
	}
}

func indexInventory(inv json.RawMessage, side string) (*inventoryIndex, string) {
	trimmed := bytes.TrimSpace(inv)
	if len(trimmed) == 0 || string(trimmed) == "null" {
		return nil, side + " inventory is unavailable"
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &root); err != nil || root == nil {
		return nil, side + " inventory is malformed"
	}
	idx := newInventoryIndex()
	itemsRaw, ok := root["items"]
	if !ok || len(bytes.TrimSpace(itemsRaw)) == 0 || string(bytes.TrimSpace(itemsRaw)) == "null" {
		return idx, ""
	}
	var items []json.RawMessage
	if err := json.Unmarshal(itemsRaw, &items); err != nil {
		return nil, side + " inventory is malformed"
	}
	for _, raw := range items {
		idx.add(raw)
	}
	sort.Strings(idx.warnings)
	return idx, ""
}

func (idx *inventoryIndex) add(raw json.RawMessage) {
	var item map[string]json.RawMessage
	if err := json.Unmarshal(raw, &item); err != nil || item == nil {
		idx.warnings = append(idx.warnings, "skipped a row because its identity could not be normalized")
		return
	}
	class, collection, id, display, identity, src, err := buildIdentity(item)
	if err != nil || identity == "" {
		idx.warnings = append(idx.warnings, "skipped a row because its identity could not be normalized")
		return
	}
	qty, ok := itemQuantity(item, src)
	if !ok {
		label := id
		if label == "" {
			label = class
		}
		if label == "" {
			label = "row"
		}
		idx.warnings = append(idx.warnings, "skipped a row whose quantity is not an exact integer: "+label)
		return
	}
	huge := hugeFlag(item, src)
	variant := output.DeriveItemVariant(item)
	if prev, exists := idx.items[identity]; exists {
		prev.qty.Add(prev.qty, qty)
		if prev.displayName == "" {
			prev.displayName = display
		}
		if prev.huge == nil {
			prev.huge = huge
		}
	} else {
		idx.items[identity] = &indexedItem{
			class:       class,
			collection:  collection,
			id:          id,
			displayName: display,
			identity:    identity,
			variant:     variant,
			huge:        huge,
			qty:         qty,
		}
	}
	idx.inventoryRows.Add(idx.inventoryRows, big.NewInt(1))
	if !strings.EqualFold(class, "Pet") {
		return
	}
	idx.petQuantity.Add(idx.petQuantity, qty)
	idx.petRows.Add(idx.petRows, big.NewInt(1))
	if variant.Golden {
		idx.golden.Add(idx.golden, qty)
	}
	if variant.Rainbow {
		idx.rainbow.Add(idx.rainbow, qty)
	}
	if variant.Shiny {
		idx.shiny.Add(idx.shiny, qty)
	}
	if huge != nil {
		idx.hugeKnown = true
		if *huge {
			idx.huge.Add(idx.huge, qty)
		}
	}
}

func buildIdentity(item map[string]json.RawMessage) (class, collection, id, display, identity string, src map[string]json.RawMessage, err error) {
	raw, hasRaw, err := asObject(item["rawData"])
	if err != nil {
		return "", "", "", "", "", nil, err
	}
	src = map[string]json.RawMessage{}
	if hasRaw {
		for k, v := range raw {
			if excludedIdentityField(k) {
				continue
			}
			src[k] = v
		}
		for _, k := range []string{"id", "pt", "sh", "tn", "createdAt", "huge"} {
			if _, ok := src[k]; ok {
				continue
			}
			v := bytes.TrimSpace(item[k])
			if len(v) == 0 || string(v) == "null" || excludedIdentityField(k) {
				continue
			}
			src[k] = item[k]
		}
	} else {
		for k, v := range item {
			if excludedIdentityField(k) {
				continue
			}
			switch k {
			case "class", "collection", "displayName", "rawData":
				continue
			}
			src[k] = v
		}
	}
	class = jsonTokenString(item["class"])
	if class == "" {
		class = jsonTokenString(src["class"])
	}
	collection = jsonTokenString(item["collection"])
	if collection == "" {
		collection = jsonTokenString(src["collection"])
	}
	id = jsonTokenString(item["id"])
	if id == "" {
		id = jsonTokenString(src["id"])
	}
	display = jsonTokenString(item["displayName"])
	if display == "" {
		display = id
	}
	identity, err = canonicalIdentity(class, collection, src)
	return class, collection, id, display, identity, src, err
}

func canonicalIdentity(class, collection string, raw map[string]json.RawMessage) (string, error) {
	rawCanon, err := canonicalObject(raw)
	if err != nil {
		return "", err
	}
	doc := map[string]json.RawMessage{
		"class":      jsonString(class),
		"collection": jsonString(collection),
		"raw":        rawCanon,
	}
	out, err := canonicalObject(doc)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func jsonString(s string) json.RawMessage {
	b, err := json.Marshal(s)
	if err != nil {
		return json.RawMessage(`""`)
	}
	return b
}

func canonicalObject(obj map[string]json.RawMessage) (json.RawMessage, error) {
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var buf bytes.Buffer
	buf.WriteByte('{')
	for i, k := range keys {
		if i > 0 {
			buf.WriteByte(',')
		}
		kb, err := json.Marshal(k)
		if err != nil {
			return nil, err
		}
		buf.Write(kb)
		buf.WriteByte(':')
		cv, err := canonicalValue(obj[k])
		if err != nil {
			return nil, err
		}
		buf.Write(cv)
	}
	buf.WriteByte('}')
	return buf.Bytes(), nil
}

func canonicalValue(raw json.RawMessage) (json.RawMessage, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return json.RawMessage("null"), nil
	}
	switch raw[0] {
	case '{':
		obj, _, err := asObject(raw)
		if err != nil {
			return nil, err
		}
		return canonicalObject(obj)
	case '[':
		var arr []json.RawMessage
		if err := json.Unmarshal(raw, &arr); err != nil {
			return nil, err
		}
		var buf bytes.Buffer
		buf.WriteByte('[')
		for i, el := range arr {
			if i > 0 {
				buf.WriteByte(',')
			}
			cv, err := canonicalValue(el)
			if err != nil {
				return nil, err
			}
			buf.Write(cv)
		}
		buf.WriteByte(']')
		return buf.Bytes(), nil
	default:
		var buf bytes.Buffer
		if err := json.Compact(&buf, raw); err != nil {
			return nil, err
		}
		return buf.Bytes(), nil
	}
}

func asObject(raw json.RawMessage) (map[string]json.RawMessage, bool, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return nil, false, nil
	}
	if raw[0] != '{' {
		return nil, false, fmt.Errorf("rawData is not an object")
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, false, err
	}
	if obj == nil {
		obj = map[string]json.RawMessage{}
	}
	return obj, true, nil
}

func jsonTokenString(raw json.RawMessage) string {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	if raw[0] == '"' {
		var s string
		if json.Unmarshal(raw, &s) == nil {
			return s
		}
	}
	return string(raw)
}

func itemQuantity(item, raw map[string]json.RawMessage) (*big.Int, bool) {
	for _, candidate := range []json.RawMessage{item["count"], item["_am"], raw["_am"], raw["count"]} {
		trimmed := bytes.TrimSpace(candidate)
		if len(trimmed) == 0 || string(trimmed) == "null" {
			continue
		}
		return exactQuantity(candidate)
	}
	return nil, false
}

func exactQuantity(raw json.RawMessage) (*big.Int, bool) {
	e := ParseExactInt(raw)
	if !e.ParseOK || e.Approximate || e.Int == nil {
		return nil, false
	}
	return new(big.Int).Set(e.Int), true
}

func hugeFlag(item, raw map[string]json.RawMessage) *bool {
	if b, ok := explicitBool(raw["huge"]); ok {
		return &b
	}
	if b, ok := explicitBool(item["huge"]); ok {
		return &b
	}
	return nil
}

func explicitBool(raw json.RawMessage) (bool, bool) {
	switch string(bytes.TrimSpace(raw)) {
	case "true":
		return true, true
	case "false":
		return false, true
	}
	s := strings.TrimSpace(string(raw))
	if len(s) >= 2 && s[0] == '"' {
		var text string
		if json.Unmarshal(raw, &text) == nil {
			switch strings.ToLower(strings.TrimSpace(text)) {
			case "true":
				return true, true
			case "false":
				return false, true
			}
		}
	}
	return false, false
}

func diffInventoriesFromBundles(from, to *Bundle, fromPV, toPV ProfileView) *InventoryDiff {
	var prev, cur json.RawMessage
	var fromAt, toAt string
	if from != nil {
		prev = from.Inventory
		if from.Manifest != nil {
			fromAt = from.Manifest.ExportedAt
		}
	}
	if to != nil {
		cur = to.Inventory
		if to.Manifest != nil {
			toAt = to.Manifest.ExportedAt
		}
	}
	opt := InventoryDiffOptions{
		BaselineFile:   inventoryFileName(from),
		CurrentFile:    inventoryFileName(to),
		BaselineAt:     fromAt,
		CurrentAt:      toAt,
		PrevStats:      exactStatMap(fromPV.Statistics, fromPV.HasStatistics),
		CurStats:       exactStatMap(toPV.Statistics, toPV.HasStatistics),
		PrevStatsKnown: fromPV.HasStatistics,
		CurStatsKnown:  toPV.HasStatistics,
	}
	if len(bytes.TrimSpace(prev)) == 0 {
		opt.UnavailableReason = "previous inventory is unavailable"
	}
	diff := DiffInventories(prev, cur, opt)
	return &diff
}

func inventoryFileName(b *Bundle) string {
	if b == nil || b.Manifest == nil {
		return ""
	}
	for _, f := range b.Manifest.Files {
		if f.Kind == "inventory-json" && f.OK {
			if f.Name != "" {
				return f.Name
			}
			return f.RelPath
		}
	}
	return ""
}

func exactStatMap(stats map[string]ExactInt, known bool) map[string]string {
	if !known || len(stats) == 0 {
		return nil
	}
	out := make(map[string]string, len(stats))
	for k, v := range stats {
		if v.ParseOK && !v.Approximate && v.Int != nil {
			out[k] = v.Text
		}
	}
	return out
}

func mergeWarnings(a, b []string) []string {
	out := append(append([]string{}, a...), b...)
	sort.Strings(out)
	compact := make([]string, 0, len(out))
	last := ""
	for i, w := range out {
		if i > 0 && w == last {
			continue
		}
		compact = append(compact, w)
		last = w
	}
	if compact == nil {
		return []string{}
	}
	return compact
}
