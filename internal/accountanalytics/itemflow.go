package accountanalytics

import (
	"fmt"
	"strings"
)

const MethodIDItemFlow = "accountanalytics.item-flow.v1"

// FlowCategory classifies an inventory movement explanation.
type FlowCategory string

const (
	FlowHatch      FlowCategory = "hatch"
	FlowGained     FlowCategory = "gained"
	FlowConsumed   FlowCategory = "consumed"
	FlowTradeIn    FlowCategory = "trade_in"
	FlowTradeOut   FlowCategory = "trade_out"
	FlowCraftIn    FlowCategory = "craft_in"
	FlowCraftOut   FlowCategory = "craft_out"
	FlowForgeOut   FlowCategory = "forge_consumption"
	FlowConversion FlowCategory = "conversion"
	FlowUnknown    FlowCategory = "unknown"
)

// FlowEvent is one annotated or inferred item movement.
type FlowEvent struct {
	ItemID     string       `json:"itemId"`
	Variant    string       `json:"variant,omitempty"`
	Quantity   string       `json:"quantity"`
	Category   FlowCategory `json:"category"`
	Direction  string       `json:"direction"` // in|out|unknown
	StableID   string       `json:"stableId,omitempty"`
	CreatedAt  string       `json:"createdAt,omitempty"`
	Annotation string       `json:"annotation,omitempty"`
	Evidence   string       `json:"evidence,omitempty"`
	Verified   bool         `json:"verified"`
	Warnings   []string     `json:"warnings,omitempty"`
}

// ItemFlowReport summarizes movements with double-count and unverified guards.
type ItemFlowReport struct {
	MethodID string      `json:"methodId"`
	Events   []FlowEvent `json:"events"`
	Notes    []string    `json:"notes"`
}

// BuildAnnotatedItemFlow constructs a ledger from user annotations + optional parsed movements.
// Rule: unexplained reduction is NOT automatically a trade.
func BuildAnnotatedItemFlow(annotations []FlowEvent, txMovements []Movement) ItemFlowReport {
	rep := ItemFlowReport{
		MethodID: MethodIDItemFlow,
		Notes: []string{
			"unexplained inventory reduction must not be labeled trade without matching event evidence",
			"inventory increase is not equivalent to a hatch without PetHatchCount / egg evidence",
			"avoid double counting the same transfer across annotation and transaction views",
		},
	}
	seen := map[string]bool{}
	for _, a := range annotations {
		key := strings.Join([]string{string(a.Category), a.ItemID, a.Quantity, a.StableID, a.CreatedAt}, "|")
		if seen[key] {
			a.Warnings = append(a.Warnings, "duplicate annotation suppressed")
			continue
		}
		seen[key] = true
		if a.Category == "" {
			a.Category = FlowUnknown
		}
		if !a.Verified && a.Category == FlowTradeOut {
			a.Warnings = append(a.Warnings, "trade_out without verified counterparty/event remains annotation-only")
		}
		rep.Events = append(rep.Events, a)
	}
	for _, m := range txMovements {
		cat := FlowUnknown
		dir := "unknown"
		switch m.Direction {
		case DirectionIn:
			dir = "in"
			switch m.Category {
			case CategoryTrade:
				cat = FlowTradeIn
			case CategoryMailGift, CategoryMailOther:
				cat = FlowGained
			case CategoryBoothSale, CategoryBoothOther:
				cat = FlowGained
			}
		case DirectionOut:
			dir = "out"
			switch m.Category {
			case CategoryTrade:
				cat = FlowTradeOut
			case CategoryBoothSale, CategoryBoothOther:
				cat = FlowConsumed
			}
		}
		created := ""
		if m.Timestamp != 0 {
			created = fmt.Sprintf("%d", m.Timestamp)
		}
		ev := FlowEvent{
			ItemID: m.ItemID, Variant: m.Variant, Quantity: m.Qty,
			Category: cat, Direction: dir, CreatedAt: created,
			Evidence: "transactions:" + m.Source, Verified: m.Timestamp != 0 && m.ItemID != "",
		}
		key := strings.Join([]string{string(ev.Category), ev.ItemID, ev.Quantity, ev.CreatedAt, ev.Evidence}, "|")
		if seen[key] {
			continue
		}
		seen[key] = true
		rep.Events = append(rep.Events, ev)
	}
	return rep
}
