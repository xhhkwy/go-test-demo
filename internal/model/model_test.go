package model

import (
	"encoding/json"
	"testing"
)

func TestUnknownContract(t *testing.T) {
	r := Unknown(Scan{IP: "1.2.3.4", Port: 22})
	data, err := json.Marshal(r)
	if err != nil {
		t.Fatal(err)
	}
	var fields map[string]any
	if err := json.Unmarshal(data, &fields); err != nil {
		t.Fatal(err)
	}
	if len(fields) != 7 || fields["protocol"] != "unknown" || fields["version"] != "" || fields["confidence"] != float64(0) {
		t.Fatalf("invalid result contract: %s", data)
	}
}
