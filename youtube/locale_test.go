package youtube

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// locale_test.go is the Vietnamese fixture, and it is here because a parser that
// reads YouTube in English is a parser that has been tested in English.
//
// Doc 01 section 1.3: every InnerTube request pins hl and gl, and the answer
// comes back translated. Not just the labels. The subscriber line becomes
// "1,6 Tr người đăng ký", the tab strip becomes "Trang chủ", and a parser that
// found the videos tab by looking for the word "Videos" finds nothing and reports
// a channel with no videos tab rather than failing.
//
// So the same channel is captured twice, once in each language, and the two
// records have to agree on everything that is not rendered text. What is left
// after the _text fields are taken out is the record: the id, the counts as
// numbers, the tab slugs, the flags, the images. Those come from the data and not
// from the translation, and a field that moves between the two is a field being
// read off a label.

// localeTextual is the fields that are allowed to differ, each with the reason.
// A field that differs and is not in here is the bug this test exists to find.
//
// Anything ending in _text is excused without an entry, because that is what the
// suffix is for, and so is anything under via, which is prose for a human and
// quotes the label it read.
var localeTextual = map[string]string{
	"fetched_at":   "two captures, two clocks",
	"tabs[].title": "the tab's rendered label, which is exactly what slug exists to avoid reading",
	"joined_text":  "the about panel's own sentence, and the panel is not in either fixture",
	"country":      "the about panel states the country in words, translated",
}

// localeApprox is the counts that are derived from a rounded label, with how far
// apart the two languages may land.
//
// YouTube does its own rounding before it translates, and it does not round to
// the same precision in every language: the same channel is "19.9M subscribers"
// in English and "20 Tr người đăng ký" in Vietnamese. Neither is wrong and there
// is no exact number on the page to prefer. What can be checked is that they
// describe the same channel, which is what a tolerance says.
var localeApprox = map[string]float64{
	"subscriber_count": 0.05,
}

// TestTheSameChannelInTwoLanguages parses the English and the Vietnamese capture
// of @BBCNews and compares them field by field.
func TestTheSameChannelInTwoLanguages(t *testing.T) {
	const url = "https://www.youtube.com/@BBCNews"
	en := ParseChannelRecord(loadChannelPage(t, "channel_page_verified.json"), url)
	vi := ParseChannelRecord(loadChannelPage(t, "channel_page_vi.json"), url)
	if en == nil || vi == nil {
		t.Fatal("one of the two captures parsed to nothing")
	}

	// The Vietnamese one has to actually be in Vietnamese, or this test passes by
	// comparing two English pages and proves nothing at all.
	if !strings.Contains(vi.SubscriberCountText, "người đăng ký") {
		t.Fatalf("the vi fixture is not in Vietnamese: subscriber text = %q", vi.SubscriberCountText)
	}
	if strings.Contains(en.SubscriberCountText, "người đăng ký") {
		t.Fatalf("the en fixture is not in English: subscriber text = %q", en.SubscriberCountText)
	}

	left, right := flattenRecord(t, en), flattenRecord(t, vi)
	for _, path := range union(left, right) {
		key := generalise(path)
		if strings.HasSuffix(key, "_text") || strings.HasPrefix(key, "via.") {
			continue
		}
		if _, textual := localeTextual[key]; textual {
			continue
		}
		if tolerance, approx := localeApprox[key]; approx {
			if !within(left[path], right[path], tolerance) {
				t.Errorf("%s is further apart than %.0f%% between the two languages: en %s, vi %s",
					path, tolerance*100, left[path], right[path])
			}
			continue
		}
		if left[path] != right[path] {
			t.Errorf("%s differs between the two languages:\n  en: %s\n  vi: %s\nA field that moves with the language is a field read off a label.",
				path, left[path], right[path])
		}
	}
}

func within(a, b string, tolerance float64) bool {
	x, errA := strconv.ParseFloat(a, 64)
	y, errB := strconv.ParseFloat(b, 64)
	if errA != nil || errB != nil || x == 0 {
		return a == b
	}
	return math.Abs(x-y)/x <= tolerance
}

// generalise turns tabs[3].title into tabs[].title, so one excuse covers a strip
// of eight tabs rather than eight identical excuses that break when a channel
// grows a ninth.
func generalise(path string) string {
	var out strings.Builder
	depth := 0
	for _, r := range path {
		switch {
		case r == '[':
			depth++
			out.WriteRune(r)
		case r == ']':
			depth--
			out.WriteRune(r)
		case depth > 0:
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}

// flattenRecord marshals a record and flattens it to one line per leaf, which is
// what makes the failure name the field rather than print two records and leave
// the reader to find the difference.
func flattenRecord(t *testing.T, rec any) map[string]string {
	t.Helper()
	raw, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var tree any
	if err := json.Unmarshal(raw, &tree); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]string{}
	flatten("", tree, out)
	return out
}

func flatten(prefix string, node any, out map[string]string) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			path := key
			if prefix != "" {
				path = prefix + "." + key
			}
			flatten(path, child, out)
		}
	case []any:
		for i, child := range v {
			flatten(fmt.Sprintf("%s[%d]", prefix, i), child, out)
		}
	default:
		out[prefix] = fmt.Sprint(node)
	}
}

// union is every path either record has, so a field that one language carries and
// the other drops entirely is a difference and not a pair of empty strings.
func union(a, b map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range []map[string]string{a, b} {
		for path := range m {
			if !seen[path] {
				seen[path] = true
				out = append(out, path)
			}
		}
	}
	sort.Strings(out)
	return out
}
