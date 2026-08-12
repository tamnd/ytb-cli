package ytb

import (
	"strings"
	"testing"
)

// A rendered count is a number and then a noun, and both of them change with the
// language. The two tables below are separated by where they came from, because
// that is the difference between a test that proves something and a test that
// agrees with the code.
//
// The observed table is strings YouTube served, captured with `ytb archive`
// against @BBCNews and dQw4w9WgXcQ. The Vietnamese ones are the whole reason
// counts.go exists: the old parser read "7.190.898.973 lượt xem" as 7.
//
// The CLDR table is the rest of the languages, and those strings are the short
// compact patterns rather than captures. Asking this network for the same channel
// with --hl de came back in English, so there was nothing to capture. That is
// worth knowing on its own and it is what the live suite's hl test is for; it is
// not a reason to leave the other twenty languages unparsed.

func TestParseCountTextObserved(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		why  string
	}{
		{"", 0, "nothing"},
		{"no numbers here", 0, "a sentence with no count in it"},
		{"views 12", 0, "the number has to lead"},
		{"121 views", 121, "the plain case"},
		{"116 songs", 116, "the noun is whatever the surface calls its rows"},
		{"1,234 views", 1_234, "an English group separator"},
		{"1.2M views", 1_200_000, "the compact form, glued to the number"},
		{"1.29K subscribers", 1_290, ""},
		{"2B plays", 2_000_000_000, ""},
		{"15.6M monthly audience", 15_600_000, ""},
		{"19.9M subscribers", 19_900_000, "@BBCNews in English"},
		{"32K videos", 32_000, "@BBCNews in English"},

		{"7.190.898.973 lượt xem", 7_190_898_973, "vi groups with dots, and this is the one that used to read 7"},
		{"20 Tr người đăng ký", 20_000_000, "vi triệu, joined with a non-breaking space"},
		{"32 N video", 32_000, "vi nghìn"},
		{"6,7 Tr video", 6_700_000, "vi writes the decimal with a comma, from #lofi"},
		{"1,3 Tr kênh", 1_300_000, "the other half of the same hashtag line"},
	}
	for _, c := range cases {
		if got := parseCountText(c.in); got != c.want {
			t.Errorf("parseCountText(%q) = %d, want %d: %s", c.in, got, c.want, c.why)
		}
	}
}

func TestParseCountTextCLDR(t *testing.T) {
	cases := []struct {
		in   string
		want int64
		lang string
	}{
		{"20 Mio. Abonnenten", 20_000_000, "de"},
		{"32 Tsd. Videos", 32_000, "de"},
		{"1,6 Mrd. Aufrufe", 1_600_000_000, "de"},
		{"20 млн подписчиков", 20_000_000, "ru"},
		{"32 тыс. видео", 32_000, "ru"},
		{"1,6 млрд просмотров", 1_600_000_000, "ru"},
		{"20 jt subscriber", 20_000_000, "id"},
		{"32 rb video", 32_000, "id"},
		{"20 mln subskrybentów", 20_000_000, "pl"},
		{"32 tys. filmów", 32_000, "pl"},
		{"20 mi de inscritos", 20_000_000, "pt"},
		{"32 mil vídeos", 32_000, "pt"},
		{"20 M d'abonnés", 20_000_000, "fr"},
		{"1,6 Md de vues", 1_600_000_000, "fr"},
		{"2000万 人の登録者", 20_000_000, "ja counts in ten thousands"},
		{"16億 回視聴", 1_600_000_000, "ja"},
		{"2000만명의 구독자", 20_000_000, "ko"},
		{"20 Mln iscritti", 20_000_000, "it"},
		{"32 mila video", 32_000, "it"},

		// French groups with a narrow no-break space, which is not a space to
		// strings.Fields, and that is what normaliseSpaces is for.
		{"7 190 898 973 vues", 7_190_898_973, "fr, the exact count written out"},
	}
	for _, c := range cases {
		if got := parseCountText(c.in); got != c.want {
			t.Errorf("parseCountText(%q) = %d, want %d (%s)", c.in, got, c.want, c.lang)
		}
	}
}

// The separators are read rather than deleted, and which one is the decimal point
// is decided by where it sits. This is the half of counts.go that has nothing to
// do with language.
func TestParseGroupedNumber(t *testing.T) {
	cases := map[string]float64{
		"1.234":         1234, // three digits after a lone dot is a group
		"1.2":           1.2,  // one digit after a lone dot is a decimal
		"1,234":         1234, // and the same the other way round
		"1,2":           1.2,  //
		"7.190.898.973": 7190898973,
		"1.234.567,89":  1234567.89, // both present, the comma is last
		"1,234,567.89":  1234567.89, // both present, the dot is last
		"1 234 567":     1234567,    // grouped with spaces
		"42":            42,
	}
	for in, want := range cases {
		got, ok := parseGroupedNumber(in)
		if !ok {
			t.Errorf("parseGroupedNumber(%q) failed", in)
			continue
		}
		if got != want {
			t.Errorf("parseGroupedNumber(%q) = %v, want %v", in, got, want)
		}
	}
}

// Every key in the table has to be in the form the lookup produces, which is
// lowercased with the trailing dot off. A key that is not can never match, and
// nothing about it would look wrong in a diff.
func TestCountMagnitudesAreLookedUpInTheirOwnForm(t *testing.T) {
	for suffix, mult := range countMagnitudes {
		if want := strings.TrimRight(strings.ToLower(suffix), ".,"); suffix != want {
			t.Errorf("%q is stored in a form the lookup never asks for, %q", suffix, want)
		}
		if mult < 1e3 {
			t.Errorf("%q multiplies by %v, which is not a magnitude", suffix, mult)
		}
	}
}
