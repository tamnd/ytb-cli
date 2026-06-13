package youtube

import (
	"fmt"
	"strconv"
	"strings"
)

// ItemSelector resolves a yt-dlp-style --playlist-items spec against a 1-based
// index. Supported forms, comma-separated: "1", "3-7", "5-" (open end), "-3"
// (from start), and negative indices counting from the end ("-1" is last when a
// total is known).
type ItemSelector struct {
	ranges []itemRange
	empty  bool
}

type itemRange struct {
	start, end int // 0 means unbounded on that side; negatives count from end
}

// ParseItemSelector compiles a --playlist-items spec. An empty spec selects all.
func ParseItemSelector(spec string) (*ItemSelector, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return &ItemSelector{empty: true}, nil
	}
	sel := &ItemSelector{}
	for _, tok := range strings.Split(spec, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		r, err := parseItemRange(tok)
		if err != nil {
			return nil, err
		}
		sel.ranges = append(sel.ranges, r)
	}
	if len(sel.ranges) == 0 {
		sel.empty = true
	}
	return sel, nil
}

func parseItemRange(tok string) (itemRange, error) {
	if strings.Contains(tok, "-") && !strings.HasPrefix(tok, "-") {
		parts := strings.SplitN(tok, "-", 2)
		start, err := atoiMaybe(parts[0])
		if err != nil {
			return itemRange{}, err
		}
		end := 0
		if strings.TrimSpace(parts[1]) != "" {
			end, err = atoiMaybe(parts[1])
			if err != nil {
				return itemRange{}, err
			}
		}
		return itemRange{start: start, end: end}, nil
	}
	// Single index (possibly negative).
	n, err := strconv.Atoi(tok)
	if err != nil {
		return itemRange{}, fmt.Errorf("invalid item %q", tok)
	}
	return itemRange{start: n, end: n}, nil
}

func atoiMaybe(s string) (int, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("invalid index %q", s)
	}
	return n, nil
}

// Selects reports whether the 1-based index is selected. total may be 0 when
// unknown; negative bounds then never match.
func (s *ItemSelector) Selects(index, total int) bool {
	if s == nil || s.empty {
		return true
	}
	for _, r := range s.ranges {
		start := resolveBound(r.start, total)
		end := resolveBound(r.end, total)
		switch {
		case start != 0 && end != 0:
			if index >= start && index <= end {
				return true
			}
		case start != 0 && end == 0:
			if index >= start {
				return true
			}
		case start == 0 && end != 0:
			if index <= end {
				return true
			}
		}
	}
	return false
}

func resolveBound(v, total int) int {
	if v >= 0 {
		return v
	}
	if total <= 0 {
		return 0 // unknown total: negative bound can't be resolved
	}
	if r := total + v + 1; r > 0 {
		return r
	}
	return 0
}
