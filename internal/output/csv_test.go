package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteInventoryCSVExactNumbers(t *testing.T) {
	const huge = "9007199254740993"
	inv := json.RawMessage(`{"items":[{"class":"Pet","id":"Huge","count":` + huge + `,"displayName":"Huge","rap":` + huge + `,"rapApproximate":false,"exists":` + huge + `,"rarity":"Basic","rarityNumber":1,"collection":"Pets","rawData":{"id":"Huge","_am":` + huge + `}}]}`)
	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, inv, InventoryCSVOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "variant,golden,rainbow,shiny,pt,sh,stackKey") {
		t.Fatalf("missing variant columns:\n%s", out)
	}
	if !strings.Contains(out, huge) {
		t.Fatalf("exact count missing:\n%s", out)
	}
	if strings.Contains(out, "9007199254740992") {
		t.Fatal("count rounded to float64")
	}
	if !strings.Contains(out, ",Regular,false,false,false,,,") {
		t.Fatalf("expected Regular variant:\n%s", out)
	}
}

func TestWriteInventoryCSVVariants(t *testing.T) {
	inv := json.RawMessage(`{"items":[
		{"class":"Pet","id":"Butler Penguin","count":4,"displayName":"Butler Penguin","stackKey":"{\"id\":\"Butler Penguin\",\"pt\":1,\"_am\":4}","rawData":{"id":"Butler Penguin","pt":1,"_am":4},"rap":400,"rapApproximate":false,"exists":1,"rarity":"Basic","rarityNumber":1,"collection":"Pets"},
		{"class":"Pet","id":"Butler Penguin","count":2,"displayName":"Butler Penguin","stackKey":"{\"id\":\"Butler Penguin\",\"pt\":2,\"_am\":2}","rawData":{"id":"Butler Penguin","pt":2,"_am":2},"rap":2000,"rapApproximate":false,"exists":1,"rarity":"Basic","rarityNumber":1,"collection":"Pets"}
	]}`)
	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, inv, InventoryCSVOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, ",Golden,true,false,false,1,,") {
		t.Fatalf("golden row missing:\n%s", out)
	}
	if !strings.Contains(out, ",Rainbow,false,true,false,2,,") {
		t.Fatalf("rainbow row missing:\n%s", out)
	}
}

func TestWriteInventoryCSVJoinRAP(t *testing.T) {
	inv := json.RawMessage(`{"items":[{"class":"Pet","id":"Test Cat","count":3,"displayName":"Test Cat","rap":1,"rapApproximate":false,"exists":2,"rarity":"Basic","rarityNumber":1,"collection":"Pets","rawData":{"id":"Test Cat","_am":3}}]}`)
	idx, err := BuildRAPIndex(json.RawMessage(`[{"category":"Pet","configData":{"id":"Test Cat","sh":true},"value":1250}]`))
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, inv, InventoryCSVOptions{RAPIndex: idx}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "marketRap,marketRapMatched") {
		t.Fatalf("missing join columns:\n%s", out)
	}
	if !strings.Contains(out, ",1250,true") {
		t.Fatalf("expected market RAP join:\n%s", out)
	}
}

func TestWriteCurrenciesCSVExactAM(t *testing.T) {
	const huge = "9007199254740993"
	currencies := map[string]json.RawMessage{
		"Diamonds": json.RawMessage(`{"id":"Diamonds","_am":` + huge + `}`),
	}
	var buf bytes.Buffer
	if err := WriteCurrenciesCSV(&buf, currencies, CurrencyCSVOptions{}); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "key,id,_am,amount\n") {
		t.Fatalf("header: %q", out)
	}
	if !strings.Contains(out, "Diamonds,Diamonds,"+huge+","+huge) {
		t.Fatalf("row missing exact _am/amount:\n%s", out)
	}
}

func TestWriteCurrenciesCSVExcelBOM(t *testing.T) {
	var buf bytes.Buffer
	if err := WriteCurrenciesCSV(&buf, map[string]json.RawMessage{}, CurrencyCSVOptions{Excel: true}); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(buf.String(), "\ufeffkey,id,_am,amount\n") {
		t.Fatalf("expected BOM prefix: %q", buf.String())
	}
}

func TestWriteQuotaCSV(t *testing.T) {
	var buf bytes.Buffer
	err := WriteQuotaCSV(&buf, map[string]any{
		"used": json.Number("3"), "limit": json.Number("48"),
		"remaining": "45", "retryAfter": "", "quotaExhausted": false,
	}, false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "3,48,45,") {
		t.Fatalf("%s", buf.String())
	}
}
