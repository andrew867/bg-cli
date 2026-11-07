package accountanalytics

import (
	"encoding/json"
	"fmt"
	"math/big"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

const portfolioMethodID = "accountanalytics.portfolio.v1"

// PortfolioLine is one inventory stack with optional variant-matched RAP.
type PortfolioLine struct {
	Class        string `json:"class"`
	ID           string `json:"id"`
	Variant      string `json:"variant"`
	PT           string `json:"pt,omitempty"`
	SH           string `json:"sh,omitempty"`
	Qty          string `json:"qty"`
	RAPMatched   bool   `json:"rapMatched"`
	RAPValue     string `json:"rapValue,omitempty"`
	RAPLineTotal string `json:"rapLineTotal,omitempty"`
	MethodID     string `json:"methodId"`
}

// PortfolioReport aggregates exact quantities and variant-matched RAP (never imputing missing RAP as zero).
type PortfolioReport struct {
	MethodID           string          `json:"methodId"`
	Lines              []PortfolioLine `json:"lines"`
	TotalQty           string          `json:"totalQty"`
	MatchedRAPSum      string          `json:"matchedRapSum"`
	MatchedLineCount   int             `json:"matchedLineCount"`
	UnmatchedLineCount int             `json:"unmatchedLineCount"`
	RAPCoveragePercent float64         `json:"rapCoveragePercent"`
}

// VariantRAPIndex maps class+id+variant signature to exact RAP tokens.
type VariantRAPIndex map[string]json.RawMessage

// BuildVariantRAPIndex indexes public RAP payloads with pt/sh-aware keys.
func BuildVariantRAPIndex(raw json.RawMessage) (VariantRAPIndex, error) {
	idx := VariantRAPIndex{}
	if len(raw) == 0 || string(raw) == "null" {
		return idx, nil
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) != nil {
		return nil, fmt.Errorf("variant rap: expected array")
	}
	for _, entry := range arr {
		var fields map[string]json.RawMessage
		if json.Unmarshal(entry, &fields) != nil {
			continue
		}
		var category string
		_ = json.Unmarshal(fields["category"], &category)
		var cd map[string]json.RawMessage
		_ = json.Unmarshal(fields["configData"], &cd)
		id := scalarJSON(cd["id"])
		if category == "" || id == "" {
			continue
		}
		pt := scalarJSON(cd["pt"])
		sh := scalarJSON(cd["sh"])
		val := fields["value"]
		if len(val) == 0 {
			continue
		}
		key := variantRAPKey(category, id, pt, sh, variantLabel(pt, sh))
		idx[key] = append(json.RawMessage(nil), val...)
	}
	return idx, nil
}

func variantRAPKey(category, id, pt, sh, label string) string {
	return strings.ToLower(category) + "\x00" + strings.ToLower(id) + "\x00" + label + "\x00" + pt + "\x00" + sh
}

func (idx VariantRAPIndex) lookup(class, id, pt, sh, label string) (json.RawMessage, bool) {
	if idx == nil {
		return nil, false
	}
	key := variantRAPKey(class, id, pt, sh, label)
	v, ok := idx[key]
	if ok && len(v) > 0 {
		return v, true
	}
	// Try alternate labels derived only from pt/sh when label normalization differs.
	alt := variantRAPKey(class, id, pt, sh, variantLabel(pt, sh))
	v, ok = idx[alt]
	return v, ok && len(v) > 0
}

// PortfolioReport builds a quantity/RAP report from inventory JSON and optional RAP payload.
func BuildPortfolioReport(inventory json.RawMessage, rapPayload json.RawMessage) (PortfolioReport, error) {
	rapIdx, err := BuildVariantRAPIndex(rapPayload)
	if err != nil {
		return PortfolioReport{}, err
	}
	items, err := inventoryItems(inventory)
	if err != nil {
		return PortfolioReport{}, err
	}
	totalQty := new(big.Int)
	rapSum := new(big.Int)
	matched, unmatched := 0, 0
	var lines []PortfolioLine
	for _, item := range items {
		class := scalarJSON(item["class"])
		id := scalarJSON(item["id"])
		v := output.DeriveItemVariant(item)
		qty := snapshot.ParseExactInt(item["count"])
		if !qty.ParseOK {
			qty = snapshot.ParseExactInt(item["_am"])
		}
		if qty.ParseOK && qty.Int != nil {
			totalQty.Add(totalQty, qty.Int)
		}
		line := PortfolioLine{
			Class: class, ID: id, Variant: v.Label, PT: v.PT, SH: v.SH,
			Qty: qty.Text, MethodID: portfolioMethodID,
		}
		if rapRaw, ok := rapIdx.lookup(class, id, v.PT, v.SH, v.Label); ok {
			rapVal := snapshot.ParseExactInt(rapRaw)
			if rapVal.ParseOK {
				line.RAPMatched = true
				line.RAPValue = rapVal.Text
				matched++
				if qty.ParseOK && rapVal.Int != nil && qty.Int != nil {
					lt := new(big.Int).Mul(qty.Int, rapVal.Int)
					line.RAPLineTotal = lt.String()
					rapSum.Add(rapSum, lt)
				}
			} else {
				unmatched++
			}
		} else {
			unmatched++
		}
		lines = append(lines, line)
	}
	sort.Slice(lines, func(i, j int) bool {
		if lines[i].Class != lines[j].Class {
			return lines[i].Class < lines[j].Class
		}
		if lines[i].ID != lines[j].ID {
			return lines[i].ID < lines[j].ID
		}
		return lines[i].Variant < lines[j].Variant
	})
	coverage := 0.0
	if len(lines) > 0 {
		coverage = float64(matched) / float64(len(lines)) * 100
	}
	return PortfolioReport{
		MethodID:           portfolioMethodID,
		Lines:              lines,
		TotalQty:           totalQty.String(),
		MatchedRAPSum:      rapSum.String(),
		MatchedLineCount:   matched,
		UnmatchedLineCount: unmatched,
		RAPCoveragePercent: coverage,
	}, nil
}

func inventoryItems(inventory json.RawMessage) ([]map[string]json.RawMessage, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(inventory, &root); err != nil {
		return nil, err
	}
	var items []json.RawMessage
	if err := json.Unmarshal(root["items"], &items); err != nil {
		return nil, fmt.Errorf("inventory items: %w", err)
	}
	out := make([]map[string]json.RawMessage, 0, len(items))
	for _, it := range items {
		var m map[string]json.RawMessage
		if json.Unmarshal(it, &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}
