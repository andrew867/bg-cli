package accountanalytics

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/andrew867/bg-cli/internal/output"
	"github.com/andrew867/bg-cli/internal/snapshot"
)

const transactionsMethodID = "accountanalytics.transactions.v1"

// MovementCategory classifies normalized item flow.
type MovementCategory string

const (
	CategoryTrade      MovementCategory = "trade"
	CategoryBoothSale  MovementCategory = "booth_sale"
	CategoryBoothOther MovementCategory = "booth"
	CategoryMailGift   MovementCategory = "mail_gift"
	CategoryMailOther  MovementCategory = "mail"
)

// MovementDirection is inflow or outflow relative to the snapshot owner.
type MovementDirection string

const (
	DirectionOut MovementDirection = "out"
	DirectionIn  MovementDirection = "in"
)

// Movement is one normalized item transfer from trades, booth, or mail views.
type Movement struct {
	Source            string            `json:"source"`
	Timestamp         int64             `json:"timestamp,omitempty"`
	Category          MovementCategory  `json:"category"`
	Class             string            `json:"class,omitempty"`
	ItemID            string            `json:"itemId,omitempty"`
	Variant           string            `json:"variant,omitempty"`
	Qty               string            `json:"qty"`
	Direction         MovementDirection `json:"direction"`
	CounterpartyUID   string            `json:"counterpartyUid,omitempty"`
	CounterpartyName  string            `json:"counterpartyName,omitempty"`
	CounterpartyRedacted bool           `json:"counterpartyRedacted,omitempty"`
	MethodID          string            `json:"methodId"`
}

// TransactionSummary aggregates movements.
type TransactionSummary struct {
	MethodID          string         `json:"methodId"`
	MovementCount     int            `json:"movementCount"`
	BySource          map[string]int `json:"bySource"`
	ByCategory        map[string]int `json:"byCategory"`
	NetByItem         []NetItemRow   `json:"netByItem,omitempty"`
	LargestTransfers  []Movement     `json:"largestTransfers,omitempty"`
	Movements         []Movement     `json:"movements"`
}

// NetItemRow is signed net quantity for one item identity.
type NetItemRow struct {
	Class    string `json:"class,omitempty"`
	ItemID   string `json:"itemId"`
	Variant  string `json:"variant,omitempty"`
	NetQty   string `json:"netQty"`
	InCount  int    `json:"inCount"`
	OutCount int    `json:"outCount"`
}

// ParseMovements extracts normalized movements from a bundle's transaction views.
func ParseMovements(bundle *snapshot.Bundle) ([]Movement, error) {
	if bundle == nil {
		return nil, fmt.Errorf("nil bundle")
	}
	var out []Movement
	if raw, ok := bundle.ViewRaw["trades"]; ok {
		m, err := parseTrades(raw)
		if err != nil {
			return nil, fmt.Errorf("trades: %w", err)
		}
		out = append(out, m...)
	}
	if raw, ok := bundle.ViewRaw["booth"]; ok {
		m, err := parseBooth(raw)
		if err != nil {
			return nil, fmt.Errorf("booth: %w", err)
		}
		out = append(out, m...)
	}
	if raw, ok := bundle.ViewRaw["mail"]; ok {
		m, err := parseMail(raw)
		if err != nil {
			return nil, fmt.Errorf("mail: %w", err)
		}
		out = append(out, m...)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Timestamp != out[j].Timestamp {
			return out[i].Timestamp < out[j].Timestamp
		}
		return out[i].ItemID < out[j].ItemID
	})
	return out, nil
}

// SummarizeMovements builds count/net/largest reports (no sale prices).
func SummarizeMovements(movements []Movement, redactCounterparty bool) TransactionSummary {
	bySource := map[string]int{}
	byCat := map[string]int{}
	type netKey struct{ class, id, variant string }
	nets := map[netKey]*NetItemRow{}
	for i := range movements {
		m := &movements[i]
		m.MethodID = transactionsMethodID
		if redactCounterparty {
			if m.CounterpartyUID != "" || m.CounterpartyName != "" {
				m.CounterpartyRedacted = true
				m.CounterpartyUID = ""
				m.CounterpartyName = ""
			}
		}
		bySource[m.Source]++
		byCat[string(m.Category)]++
		nk := netKey{m.Class, m.ItemID, m.Variant}
		if nets[nk] == nil {
			nets[nk] = &NetItemRow{Class: m.Class, ItemID: m.ItemID, Variant: m.Variant, NetQty: "0"}
		}
		qty := snapshot.ParseExactIntString(m.Qty)
		cur := snapshot.ParseExactIntString(nets[nk].NetQty)
		switch m.Direction {
		case DirectionIn:
			nets[nk].InCount++
			if qty.ParseOK && cur.ParseOK {
				nets[nk].NetQty = snapshot.AddExact(cur, qty).Text
			}
		case DirectionOut:
			nets[nk].OutCount++
			if qty.ParseOK && cur.ParseOK {
				neg, _ := snapshot.SubExact(snapshot.ExactInt{Int: bigZero(), Text: "0", ParseOK: true, SourceText: "0"}, qty)
				nets[nk].NetQty = snapshot.AddExact(cur, neg).Text
			}
		}
	}
	netRows := make([]NetItemRow, 0, len(nets))
	for _, row := range nets {
		netRows = append(netRows, *row)
	}
	sort.Slice(netRows, func(i, j int) bool {
		return netRows[i].ItemID < netRows[j].ItemID
	})
	largest := append([]Movement(nil), movements...)
	sort.Slice(largest, func(i, j int) bool {
		ai := snapshot.ParseExactIntString(largest[i].Qty)
		aj := snapshot.ParseExactIntString(largest[j].Qty)
		if ai.ParseOK && aj.ParseOK && ai.Int != nil && aj.Int != nil {
			return ai.Int.Cmp(aj.Int) > 0
		}
		return largest[i].Qty > largest[j].Qty
	})
	if len(largest) > 10 {
		largest = largest[:10]
	}
	return TransactionSummary{
		MethodID:         transactionsMethodID,
		MovementCount:    len(movements),
		BySource:         bySource,
		ByCategory:       byCat,
		NetByItem:        netRows,
		LargestTransfers: largest,
		Movements:        movements,
	}
}

// AnalyzeTransactions parses and summarizes bundle transaction views.
func AnalyzeTransactions(bundle *snapshot.Bundle, redactCounterparty bool) (TransactionSummary, error) {
	mov, err := ParseMovements(bundle)
	if err != nil {
		return TransactionSummary{}, err
	}
	return SummarizeMovements(mov, redactCounterparty), nil
}

func parseTrades(raw json.RawMessage) ([]Movement, error) {
	entries, err := extractEntries(raw)
	if err != nil {
		return nil, err
	}
	var out []Movement
	for _, ent := range entries {
		ts, _ := parseTimestamp(ent)
		party := parseParty(ent["otherParty"])
		for _, side := range []struct {
			raw []json.RawMessage
			dir MovementDirection
		}{
			{parseItemList(ent["given"]), DirectionOut},
			{parseItemList(ent["received"]), DirectionIn},
		} {
			for _, it := range side.raw {
				var item map[string]json.RawMessage
				if json.Unmarshal(it, &item) != nil {
					continue
				}
				m := movementFromItem(item, "trades", CategoryTrade, side.dir, ts, party)
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func parseBooth(raw json.RawMessage) ([]Movement, error) {
	entries, err := extractEntries(raw)
	if err != nil {
		return nil, err
	}
	var out []Movement
	for _, ent := range entries {
		ts, _ := parseTimestamp(ent)
		party := parseParty(ent["otherParty"])
		cat := CategoryBoothOther
		var kind string
		_ = json.Unmarshal(ent["kind"], &kind)
		if strings.EqualFold(kind, "sale") {
			cat = CategoryBoothSale
		}
		for _, side := range []struct {
			raw []json.RawMessage
			dir MovementDirection
		}{
			{parseItemList(ent["given"]), DirectionOut},
			{parseItemList(ent["received"]), DirectionIn},
		} {
			for _, it := range side.raw {
				var item map[string]json.RawMessage
				if json.Unmarshal(it, &item) != nil {
					continue
				}
				m := movementFromItem(item, "booth", cat, side.dir, ts, party)
				out = append(out, m)
			}
		}
	}
	return out, nil
}

func parseMail(raw json.RawMessage) ([]Movement, error) {
	entries, err := extractEntries(raw)
	if err != nil {
		return nil, err
	}
	var out []Movement
	for _, ent := range entries {
		ts, _ := parseTimestamp(ent)
		var typ string
		_ = json.Unmarshal(ent["type"], &typ)
		cat := CategoryMailOther
		if strings.EqualFold(typ, "GiftItem") {
			cat = CategoryMailGift
		}
		sender := parseParty(ent["sender"])
		receiver := parseParty(ent["receiver"])
		itemRaw, ok := ent["item"]
		if !ok {
			continue
		}
		var item map[string]json.RawMessage
		if json.Unmarshal(itemRaw, &item) != nil {
			continue
		}
		dir := DirectionIn
		party := sender
		if strings.EqualFold(typ, "GiftItem") {
			dir = DirectionIn
			party = sender
		} else if receiver.UID != "" {
			party = receiver
		}
		m := movementFromItem(item, "mail", cat, dir, ts, party)
		out = append(out, m)
	}
	return out, nil
}

type partyInfo struct {
	UID  string
	Name string
}

func parseParty(raw json.RawMessage) partyInfo {
	if len(raw) == 0 {
		return partyInfo{}
	}
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return partyInfo{}
	}
	var uid int64
	_ = json.Unmarshal(m["uid"], &uid)
	var name string
	_ = json.Unmarshal(m["displayName"], &name)
	return partyInfo{UID: fmt.Sprintf("%d", uid), Name: name}
}

func parseTimestamp(ent map[string]json.RawMessage) (int64, bool) {
	if ent == nil {
		return 0, false
	}
	var ts int64
	if json.Unmarshal(ent["timestamp"], &ts) == nil {
		return ts, true
	}
	return 0, false
}

func extractEntries(raw json.RawMessage) ([]map[string]json.RawMessage, error) {
	var top map[string]json.RawMessage
	if json.Unmarshal(raw, &top) != nil {
		return nil, fmt.Errorf("invalid trades/booth/mail shape")
	}
	entriesRaw, ok := top["entries"]
	if !ok {
		if v, ok2 := top["data"]; ok2 {
			var inner map[string]json.RawMessage
			if json.Unmarshal(v, &inner) == nil {
				entriesRaw, ok = inner["entries"]
			}
		}
	}
	if !ok {
		return nil, nil
	}
	var entries []json.RawMessage
	if json.Unmarshal(entriesRaw, &entries) != nil {
		return nil, fmt.Errorf("entries array expected")
	}
	out := make([]map[string]json.RawMessage, 0, len(entries))
	for _, e := range entries {
		var m map[string]json.RawMessage
		if json.Unmarshal(e, &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}

func parseItemList(raw json.RawMessage) []json.RawMessage {
	if len(raw) == 0 {
		return nil
	}
	var items []json.RawMessage
	if json.Unmarshal(raw, &items) == nil {
		return items
	}
	return nil
}

func movementFromItem(item map[string]json.RawMessage, source string, cat MovementCategory, dir MovementDirection, ts int64, party partyInfo) Movement {
	var id, class string
	_ = json.Unmarshal(item["id"], &id)
	_ = json.Unmarshal(item["class"], &class)
	if class == "" {
		class = scalarJSON(item["collection"])
	}
	v := output.DeriveItemVariant(item)
	qty := snapshot.ParseExactInt(item["count"])
	if !qty.ParseOK {
		qty = snapshot.ParseExactInt(item["_am"])
	}
	if !qty.ParseOK {
		qty = snapshot.ExactInt{Int: bigZero(), Text: "1", ParseOK: true, SourceText: "1"}
		if qty.Int != nil {
			qty.Int.SetInt64(1)
		}
	}
	return Movement{
		Source:           source,
		Timestamp:        ts,
		Category:         cat,
		Class:            class,
		ItemID:           id,
		Variant:          v.Label,
		Qty:              qty.Text,
		Direction:        dir,
		CounterpartyUID:  party.UID,
		CounterpartyName: party.Name,
		MethodID:         transactionsMethodID,
	}
}
