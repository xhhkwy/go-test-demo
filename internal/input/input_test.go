package input

import (
	"errors"
	"strings"
	"testing"
)

func TestBatchStopsAtLimit(t *testing.T) {
	// Stop after the configured count without retaining an unbounded RawMessage
	// array. The rest may be malformed; capacity is already exhausted.
	s := `[{"ip":"::1","port":1,"banner":""},` + strings.Repeat("null,", 10000) + "]"
	_, err := Parse(strings.NewReader(s), 1<<20, 1, 64)
	var detail *Error
	if !errors.As(err, &detail) || !detail.TooLarge {
		t.Fatalf("expected capacity error: %v", err)
	}
}

func TestMalformedJSONHasNoRecordIndex(t *testing.T) {
	for _, body := range []string{"[", `[{"ip":"::1","port":22,"banner":"\x00"}]`, `[{"ip":"::1","port":22,"banner":"unfinished`, "[] []"} {
		_, err := Parse(strings.NewReader(body), 1024, 10, 64)
		var detail *Error
		if !errors.As(err, &detail) || detail.Index != nil || detail.TooLarge {
			t.Fatalf("syntax response for %q: %v", body, err)
		}
	}
}
