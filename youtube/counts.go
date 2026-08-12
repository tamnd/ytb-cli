package youtube

import (
	"regexp"
	"strconv"
	"strings"
)

// counts.go turns a rendered count into a number, in whatever language the page
// came back in.
//
// This used to be six lines that stripped commas and looked for k, m or b, and
// it was right in English and wrong everywhere else. Two things break it.
//
// The separator. Vietnamese renders the exact lifetime view count of @BBCNews as
// "7.190.898.973 lượt xem", and a parser that treats the dot as a decimal point
// reads the first group and returns 7. That is not a near miss, it is nine orders
// of magnitude, and it looks like an answer. German, Spanish, Indonesian and
// Turkish group the same way.
//
// The magnitude word. The compact forms are CLDR's, so "20M subscribers" is
// "20 Tr người đăng ký" in Vietnamese, "20 Mio. Abonnenten" in German and
// "20 млн подписчиков" in Russian. The table below is those forms. It is a table
// and not a language switch because these parsers are pure functions with no
// client and no hl to read, which is deliberate: doc 06 section 5 wants the
// golden suite calling the parsers directly.
//
// One collision is not resolvable this way and it is written down rather than
// hidden: Turkish "B" is bin, a thousand, and English "B" is a billion. The
// English reading wins because en is the default and every other language spells
// its thousand differently. A Turkish count in the billions is the price, and
// nothing in YouTube is in the billions except views.

// countMagnitudes is the compact suffix of a rendered count, lowercased and with
// any trailing dot removed, mapped to what it multiplies by.
//
// Comments name the languages each form was taken from. CLDR calls these the
// short compact decimal patterns and YouTube renders exactly them.
var countMagnitudes = map[string]float64{
	// 10^3
	"k":    1e3, // en, fr, nl, tr(k is also used), sv
	"n":    1e3, // vi, nghìn
	"tys":  1e3, // pl, tysiąc
	"tsd":  1e3, // de, Tausend
	"mil":  1e3, // es, pt
	"mila": 1e3, // it
	"rb":   1e3, // id, ribu
	"тыс":  1e3, // ru
	"тис":  1e3, // uk
	"хил":  1e3, // bg
	"tis":  1e3, // cs, sk
	"e":    1e3, // fi, tuhatta
	"tn":   1e3, // et

	// 10^6
	"m":      1e6, // en, fr, es
	"mio":    1e6, // de
	"mln":    1e6, // pl, nl, ru transliterated
	"mn":     1e6, // tr
	"млн":    1e6, // ru, uk
	"tr":     1e6, // vi, triệu
	"jt":     1e6, // id, juta
	"mi":     1e6, // pt
	"ล้าน":   1e6, // th
	"مليون":  1e6, // ar
	"میلیون": 1e6, // fa

	// 10^9
	"b":       1e9, // en. Also tr bin, a thousand, and en wins. See above.
	"mrd":     1e9, // de
	"md":      1e9, // fr
	"mld":     1e9, // it, pl, nl
	"млрд":    1e9, // ru, uk
	"t":       1e9, // vi, tỷ
	"bi":      1e9, // pt
	"مليار":   1e9, // ar
	"میلیارد": 1e9, // fa
}

// countGluedMagnitudes is the same table for languages that do not put spaces
// between words, so the magnitude is not a field and has to be matched as a
// prefix of whatever followed the number.
//
// Korean renders twenty million subscribers as "2000만명의 구독자": the magnitude,
// the counter and the noun run together, and splitting on spaces gives "만명의",
// which is in no table. These are also the two magnitudes English has no word
// for, ten thousand and a hundred million, which is why east Asia writes twenty
// million as two thousand of something.
var countGluedMagnitudes = map[string]float64{
	"万": 1e4, // ja, zh-Hans, zh-Hant
	"만": 1e4, // ko
	"천": 1e3, // ko
	"億": 1e8, // ja
	"亿": 1e8, // zh-Hans
	"억": 1e8, // ko
}

// countLeadRe takes the leading number of a rendered count with its separators
// still in it, so that the separators can be read rather than deleted. Spaces
// inside the number are a group separator in French and Czech and are matched
// here for the same reason.
var countLeadRe = regexp.MustCompile(`^[0-9][0-9.,\x{00a0}\x{202f}\x{2009} ]*`)

// parseCountText converts a rendered count to a number: "1.2M views", "5K",
// "3,400", "20 Tr người đăng ký", "7.190.898.973 lượt xem".
//
// It returns 0 for a string with no number in it, which every caller already
// treats as absent.
func parseCountText(s string) int64 {
	s = strings.TrimSpace(strings.ToLower(normaliseSpaces(s)))
	if s == "" {
		return 0
	}
	lead := countLeadRe.FindString(s)
	if lead == "" {
		return 0
	}
	n, ok := parseGroupedNumber(strings.TrimSpace(lead))
	if !ok {
		return 0
	}
	return int64(n * countMagnitude(s[len(lead):]))
}

// countMagnitude reads the magnitude word off what followed the number. Only the
// first word is looked at, because the rest is the noun: "Tr người đăng ký" is a
// million subscribers and "người" is not a magnitude.
//
// Spanish spells a billion "mil M", two words, and that is the one case where the
// second word matters.
func countMagnitude(rest string) float64 {
	rest = strings.TrimLeft(rest, " \t")
	for glued, mult := range countGluedMagnitudes {
		if strings.HasPrefix(rest, glued) {
			return mult
		}
	}
	fields := strings.Fields(rest)
	if len(fields) == 0 {
		return 1
	}
	first := strings.TrimRight(fields[0], ".,")
	if first == "mil" && len(fields) > 1 {
		if second := strings.TrimRight(fields[1], ".,"); second == "m" || second == "mn" {
			return 1e9
		}
	}
	if mult, ok := countMagnitudes[first]; ok {
		return mult
	}
	// A suffix glued to the number with no space, which is how English renders it:
	// "1.2M views" arrives here as "m views" only because the regexp stopped at the
	// digits, and "20M" arrives as "m". Both are the same lookup, so there is
	// nothing else to try and an unknown word means the number stands alone.
	return 1
}

// parseGroupedNumber reads a number whose separators are not known in advance.
//
// The rule is positional and needs no language: whichever separator appears last
// is the decimal point, and the other one groups. With only one kind of separator
// present, more than one of them groups, and a single one groups when it is
// followed by exactly three digits.
//
// That last case is the only judgement call in here. "1.234" is a thousand two
// hundred and thirty four in German and one point two three four in English, and
// nothing in the string says which. It is read as grouping because YouTube writes
// a fractional count as one significant digit, "1.2M", and never as three.
func parseGroupedNumber(s string) (float64, bool) {
	s = strings.ReplaceAll(s, " ", "")
	dot, comma := strings.LastIndex(s, "."), strings.LastIndex(s, ",")
	var decimal byte
	switch {
	case dot >= 0 && comma >= 0:
		if dot > comma {
			decimal = '.'
		} else {
			decimal = ','
		}
	case dot >= 0:
		decimal = groupOrPoint(s, dot, '.')
	case comma >= 0:
		decimal = groupOrPoint(s, comma, ',')
	}

	var b strings.Builder
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c >= '0' && c <= '9':
			b.WriteByte(c)
		case c == decimal && decimal != 0 && i == lastIndexByte(s, decimal):
			b.WriteByte('.')
		}
	}
	f, err := strconv.ParseFloat(b.String(), 64)
	return f, err == nil
}

// groupOrPoint decides what a single kind of separator is doing in a number.
func groupOrPoint(s string, last int, sep byte) byte {
	if strings.Count(s, string(sep)) > 1 {
		return 0
	}
	if len(s)-last-1 == 3 {
		return 0
	}
	return sep
}

func lastIndexByte(s string, c byte) int {
	return strings.LastIndex(s, string(c))
}

// normaliseSpaces replaces the three spaces a number can be broken with by an
// ordinary one. YouTube renders "20 Tr" with U+00A0 between the two, French
// groups with U+202F, and neither is a space to strings.Fields.
func normaliseSpaces(s string) string {
	return strings.NewReplacer(" ", " ", " ", " ", " ", " ").Replace(s)
}
