package ytb

import (
	"reflect"
	"strings"
	"testing"
)

// id_test.go covers the record form of an id, and the one rule about it that a
// future change could break quietly.

// TestVLNeverReachesARecordOrAURI asserts the routing prefix stays on the wire.
//
// VL in front of a playlist id is how the browse endpoint is addressed, and that
// is all it is. It is not part of the id, YouTube does not print it anywhere a
// person can see, and two records for the same playlist, one keyed VLUU... and one
// keyed UU..., are two nodes in the graph for one playlist. The check is over the
// field with the kit:"id" tag because that field is what the URI and the store key
// are built from, so a leak there is a leak into both.
//
// Browse is the exception and the only one, because it is named for the wire and
// exists to be handed to a request.
func TestVLNeverReachesARecordOrAURI(t *testing.T) {
	inputs := []string{
		"UCuAXFkgsw1L7xaCfnd5JJOw",
		"UUuAXFkgsw1L7xaCfnd5JJOw",
		"UULFuAXFkgsw1L7xaCfnd5JJOw",
		// The wire form as input, which is the case that matters: somebody pasted a
		// browseId off a response and the record must still be keyed on the playlist.
		"VLUUuAXFkgsw1L7xaCfnd5JJOw",
		"VLPLlaN88a7y2_qHDbY9eQbuNTAuEJUSEeuu",
		"PLlaN88a7y2_qHDbY9eQbuNTAuEJUSEeuu",
		"dQw4w9WgXcQ",
	}
	for _, in := range inputs {
		info := ClassifyID(in)
		if info == nil {
			t.Fatalf("ClassifyID(%q) = nil, want a record", in)
		}
		rv := reflect.ValueOf(*info)
		rt := rv.Type()
		for i := range rt.NumField() {
			f := rt.Field(i)
			switch f.Name {
			// Browse is the wire form and the whole point of the exception. Input is
			// what the caller typed, which is not the tool's to rewrite. Note and
			// Unviewable are sentences, and the sentence about VL says the letters.
			case "Browse", "Input", "Note", "Unviewable":
				continue
			}
			s, ok := rv.Field(i).Interface().(string)
			if !ok {
				continue
			}
			if strings.Contains(s, "VL") {
				t.Errorf("ClassifyID(%q).%s = %q, which carries the wire prefix", in, f.Name, s)
			}
		}
		if id := idFieldOf(t, info); strings.HasPrefix(id, "VL") {
			t.Errorf("ClassifyID(%q) is keyed %q, so its URI and store row would be too", in, id)
		}
	}
}

// TestVLIsOfferedForTheWire asserts the browse form is still there when it is
// asked for, because the point is that the prefix has one home rather than none.
func TestVLIsOfferedForTheWire(t *testing.T) {
	info := ClassifyID("UCuAXFkgsw1L7xaCfnd5JJOw")
	if info == nil {
		t.Fatal("ClassifyID on a channel returned nil")
	}
	if want := "VLUUuAXFkgsw1L7xaCfnd5JJOw"; info.Browse != want {
		t.Errorf("browse = %q, want %q", info.Browse, want)
	}
	if !strings.HasPrefix(info.Uploads, "UU") {
		t.Errorf("uploads = %q, want the bare playlist id", info.Uploads)
	}
}

// idFieldOf returns the value of the field tagged kit:"id", which is the field the
// kit mints a URI from and keys a store row on.
func idFieldOf(t *testing.T, rec any) string {
	t.Helper()
	rt := reflect.TypeOf(rec)
	for rt.Kind() == reflect.Pointer {
		rt = rt.Elem()
	}
	rv := reflect.ValueOf(rec).Elem()
	for i := range rt.NumField() {
		if rt.Field(i).Tag.Get("kit") == "id" {
			return rv.Field(i).String()
		}
	}
	t.Fatalf("%s has no kit:\"id\" field, so nothing keys its records", rt.Name())
	return ""
}

// TestAMixIsNotOfferedAsAURL asserts the record for a mix says why there is
// nothing to read instead of handing back a link that answers 200 and refuses.
func TestAMixIsNotOfferedAsAURL(t *testing.T) {
	info := ClassifyID("RDdQw4w9WgXcQ")
	if info == nil {
		t.Fatal("ClassifyID on a mix returned nil")
	}
	if info.URL != "" {
		t.Errorf("url = %q, want empty: browsing a mix returns a refusal", info.URL)
	}
	if info.Unviewable == "" {
		t.Error("unviewable is empty, so nothing tells the reader why there is no URL")
	}
	// The terminal views draw from the note column, so the mix's one sentence has to
	// be there as well or a mix explains itself only in json.
	if info.Note != info.Unviewable {
		t.Errorf("note = %q, want the unviewable sentence", info.Note)
	}
}

// TestAnUnknownShapeIsAUsageError asserts a string that matches nothing is refused
// rather than guessed at.
func TestAnUnknownShapeIsAUsageError(t *testing.T) {
	for _, in := range []string{"", "hello", "not an id at all", "@ab"} {
		if info := ClassifyID(in); info != nil {
			t.Errorf("ClassifyID(%q) = %+v, want nil", in, info)
		}
	}
}
