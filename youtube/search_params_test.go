package youtube

import (
	"encoding/base64"
	"testing"
)

func TestSearchFiltersIsEmpty(t *testing.T) {
	if !(SearchFilters{}).IsEmpty() {
		t.Fatal("zero SearchFilters should be empty")
	}
	if (SearchFilters{HD: true}).IsEmpty() {
		t.Fatal("SearchFilters{HD:true} should not be empty")
	}
	if (SearchFilters{Sort: "date"}).IsEmpty() {
		t.Fatal("SearchFilters{Sort:date} should not be empty")
	}
}

func TestSearchFiltersEncodeEmpty(t *testing.T) {
	if got := (SearchFilters{}).Encode(); got != "" {
		t.Fatalf("empty filters Encode() = %q, want \"\"", got)
	}
}

func TestSearchFiltersEncodeDecodes(t *testing.T) {
	// A non-empty filter must produce a valid base64url protobuf blob.
	f := SearchFilters{Sort: "date", Type: "video", Duration: "long", HD: true}
	enc := f.Encode()
	if enc == "" {
		t.Fatal("non-empty filters should encode to a non-empty sp value")
	}
	if _, err := base64.URLEncoding.DecodeString(enc); err != nil {
		t.Fatalf("Encode() produced invalid base64url: %v", err)
	}
}

func TestSearchFiltersEncodeStable(t *testing.T) {
	// Same filters encode to the same value across calls.
	f := SearchFilters{Type: "channel"}
	if a, b := f.Encode(), f.Encode(); a != b {
		t.Fatalf("Encode() not stable: %q vs %q", a, b)
	}
}
