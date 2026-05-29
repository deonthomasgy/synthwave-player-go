package jsonutil

import (
	"encoding/json"
	"testing"
)

func TestStringSliceEncodesAsArray(t *testing.T) {
	b, err := json.Marshal(StringSlice(nil))
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "[]" {
		t.Fatalf("got %s, want []", b)
	}
}
