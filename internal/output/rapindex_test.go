package output

import (
	"encoding/json"
	"testing"
)

func TestBuildRAPIndexArray(t *testing.T) {
	raw := json.RawMessage(`[{"category":"Pet","configData":{"id":"Test Cat"},"value":1250},{"category":"Pet","configData":{"id":"Huge"},"value":9007199254740993}]`)
	idx, err := BuildRAPIndex(raw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := idx.Lookup("pet", "test cat")
	if !ok || string(v) != "1250" {
		t.Fatalf("lookup Test Cat: %v %s", ok, v)
	}
	v, ok = idx.Lookup("Pet", "Huge")
	if !ok || string(v) != "9007199254740993" {
		t.Fatalf("large value altered: %v %s", ok, v)
	}
}

func TestBuildRAPIndexNestedMap(t *testing.T) {
	raw := json.RawMessage(`{"Pet":{"Test Cat":42}}`)
	idx, err := BuildRAPIndex(raw)
	if err != nil {
		t.Fatal(err)
	}
	v, ok := idx.Lookup("Pet", "Test Cat")
	if !ok || string(v) != "42" {
		t.Fatalf("%v %s", ok, v)
	}
}
