package output

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func TestWriteInventoryCSVExactNumbers(t *testing.T) {
	const huge = "9007199254740993"
	inv := json.RawMessage(`{"items":[{"class":"Pet","id":"Huge","count":` + huge + `,"displayName":"Huge","rap":` + huge + `,"rapApproximate":false,"exists":` + huge + `,"rarity":"Basic","rarityNumber":1,"collection":"Pets"}]}`)
	var buf bytes.Buffer
	if err := WriteInventoryCSV(&buf, inv); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "class,id,count,displayName,rap,rapApproximate,exists,rarity,rarityNumber,collection") {
		t.Fatalf("missing header:\n%s", out)
	}
	if !strings.Contains(out, huge) {
		t.Fatalf("exact count missing:\n%s", out)
	}
	if strings.Contains(out, "9007199254740992") {
		t.Fatal("count rounded to float64")
	}
	if !strings.Contains(out, "Pet,Huge,"+huge+",Huge,"+huge+",false,"+huge+",Basic,1,Pets") {
		t.Fatalf("unexpected row:\n%s", out)
	}
}

func TestWriteCurrenciesCSVExactAM(t *testing.T) {
	const huge = "9007199254740993"
	currencies := map[string]json.RawMessage{
		"Diamonds": json.RawMessage(`{"id":"Diamonds","_am":` + huge + `}`),
	}
	var buf bytes.Buffer
	if err := WriteCurrenciesCSV(&buf, currencies); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, "key,id,_am\n") {
		t.Fatalf("header: %q", out)
	}
	if !strings.Contains(out, "Diamonds,Diamonds,"+huge) {
		t.Fatalf("row missing exact _am:\n%s", out)
	}
}
