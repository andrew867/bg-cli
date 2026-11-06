package output

import (
	"encoding/json"
	"strings"
)

// ItemVariant is the derived pet/item variant view for CSV and tables.
type ItemVariant struct {
	PT      string // exact rawData/top-level pt token when present
	SH      string // exact rawData/top-level sh token when present
	Golden  bool
	Rainbow bool
	Shiny   bool
	Label   string // Regular|Golden|Rainbow|Shiny|Golden Shiny|Rainbow Shiny|"" for non-pets
}

// DeriveItemVariant reads top-level and rawData variant fields.
// Convention: pt=1 Golden, pt=2 Rainbow, sh=true Shiny; absent fields mean Regular for pets.
func DeriveItemVariant(item map[string]json.RawMessage) ItemVariant {
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(item["rawData"], &raw)
	if raw == nil {
		raw = map[string]json.RawMessage{}
	}

	pt := firstScalar(item["pt"], raw["pt"])
	sh := firstScalar(item["sh"], raw["sh"])

	golden := boolToken(item["golden"]) || pt == "1"
	rainbow := boolToken(item["rainbow"]) || pt == "2"
	shiny := boolToken(item["shiny"]) || sh == "true"

	class := ""
	_ = json.Unmarshal(item["class"], &class)
	label := ""
	if strings.EqualFold(class, "Pet") {
		switch {
		case rainbow && shiny:
			label = "Rainbow Shiny"
		case golden && shiny:
			label = "Golden Shiny"
		case rainbow:
			label = "Rainbow"
		case golden:
			label = "Golden"
		case shiny:
			label = "Shiny"
		default:
			label = "Regular"
		}
	}

	return ItemVariant{
		PT:      pt,
		SH:      sh,
		Golden:  golden,
		Rainbow: rainbow,
		Shiny:   shiny,
		Label:   label,
	}
}

func firstScalar(vals ...json.RawMessage) string {
	for _, v := range vals {
		if s := scalarCSV(v); s != "" {
			return s
		}
	}
	return ""
}

func boolToken(raw json.RawMessage) bool {
	s := strings.ToLower(strings.TrimSpace(scalarCSV(raw)))
	return s == "true"
}
