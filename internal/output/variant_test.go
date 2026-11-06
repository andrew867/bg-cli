package output

import (
	"encoding/json"
	"testing"
)

func TestDeriveItemVariantFromRawData(t *testing.T) {
	cases := []struct {
		name    string
		raw     string
		label   string
		golden  bool
		rainbow bool
		shiny   bool
	}{
		{"regular", `{"class":"Pet","id":"X","rawData":{"id":"X","_am":1}}`, "Regular", false, false, false},
		{"golden", `{"class":"Pet","id":"X","rawData":{"id":"X","pt":1,"_am":1}}`, "Golden", true, false, false},
		{"rainbow", `{"class":"Pet","id":"X","rawData":{"id":"X","pt":2,"_am":1}}`, "Rainbow", false, true, false},
		{"shiny", `{"class":"Pet","id":"X","rawData":{"id":"X","sh":true,"_am":1}}`, "Shiny", false, false, true},
		{"rainbow shiny", `{"class":"Pet","id":"X","rawData":{"id":"X","pt":2,"sh":true,"_am":1}}`, "Rainbow Shiny", false, true, true},
		{"currency", `{"class":"Currency","id":"Diamonds","rawData":{"id":"Diamonds","_am":9}}`, "", false, false, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var item map[string]json.RawMessage
			if err := json.Unmarshal([]byte(tc.raw), &item); err != nil {
				t.Fatal(err)
			}
			v := DeriveItemVariant(item)
			if v.Label != tc.label || v.Golden != tc.golden || v.Rainbow != tc.rainbow || v.Shiny != tc.shiny {
				t.Fatalf("%+v", v)
			}
		})
	}
}
