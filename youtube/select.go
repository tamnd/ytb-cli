package youtube

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
)

// Selection is the outcome of resolving a -f format string against a manifest.
// When Audio is non-nil the result is two adaptive streams to be merged;
// otherwise Video alone is a complete (progressive or single) download.
type Selection struct {
	Video *Stream
	Audio *Stream
}

// Streams returns the selected streams in download order.
func (s Selection) Streams() []Stream {
	var out []Stream
	if s.Video != nil {
		out = append(out, *s.Video)
	}
	if s.Audio != nil {
		out = append(out, *s.Audio)
	}
	return out
}

// NeedsMerge reports whether the selection is a video+audio pair that ffmpeg
// must combine.
func (s Selection) NeedsMerge() bool { return s.Video != nil && s.Audio != nil }

// SelectFormat resolves a yt-dlp-style format string against the manifest's
// streams. Supported grammar:
//
//	best worst b w                  overall best/worst progressive-or-merged
//	bestvideo bestaudio bv ba       best/worst adaptive video / audio track
//	bv*                             best video, progressive allowed
//	22 137+140                      explicit itags, '+' merges two tracks
//	bv+ba/b                         '/' tries each group left to right
//	bv[height<=720] ba[ext=m4a]     [k OP v] filters: =, !=, <, <=, >, >=
//
// Recognised filter keys: height, width, fps, ext, vcodec, acodec, itag, abr, tbr.
func SelectFormat(streams []Stream, spec string) (Selection, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		spec = "best"
	}
	var lastErr error
	for _, group := range strings.Split(spec, "/") {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}
		sel, err := selectGroup(streams, group)
		if err != nil {
			lastErr = err
			continue
		}
		return sel, nil
	}
	if lastErr != nil {
		return Selection{}, lastErr
	}
	return Selection{}, fmt.Errorf("no format matched %q", spec)
}

func selectGroup(streams []Stream, group string) (Selection, error) {
	parts := strings.Split(group, "+")
	if len(parts) == 1 {
		st, err := selectOne(streams, parts[0])
		if err != nil {
			return Selection{}, err
		}
		if st.AudioOnly() {
			return Selection{Audio: st}, nil
		}
		return Selection{Video: st}, nil
	}
	if len(parts) != 2 {
		return Selection{}, fmt.Errorf("format %q: only one '+' (video+audio) is supported", group)
	}
	v, err := selectOne(streams, parts[0])
	if err != nil {
		return Selection{}, err
	}
	a, err := selectOne(streams, parts[1])
	if err != nil {
		return Selection{}, err
	}
	return Selection{Video: v, Audio: a}, nil
}

type formatFilter struct {
	key, op string
	val     string
}

// selectOne resolves a single token (no '+' or '/') to one stream.
func selectOne(streams []Stream, token string) (*Stream, error) {
	token = strings.TrimSpace(token)
	base, filters, err := splitFilters(token)
	if err != nil {
		return nil, err
	}

	// Start from the full set, narrow by the base selector, then by filters.
	cands := make([]Stream, len(streams))
	copy(cands, streams)

	pickBest := true
	switch base {
	case "best", "b", "":
		cands = preferMuxed(cands)
	case "worst", "w":
		cands, pickBest = preferMuxed(cands), false
	case "bestvideo", "bv":
		cands = filterFunc(cands, Stream.VideoOnly)
	case "worstvideo", "wv":
		cands, pickBest = filterFunc(cands, Stream.VideoOnly), false
	case "bv*":
		cands = filterFunc(cands, func(s Stream) bool { return s.HasVideo })
	case "bestaudio", "ba":
		cands = filterFunc(cands, Stream.AudioOnly)
	case "worstaudio", "wa":
		cands, pickBest = filterFunc(cands, Stream.AudioOnly), false
	case "ba*":
		cands = filterFunc(cands, func(s Stream) bool { return s.HasAudio })
	case "mp4", "webm", "m4a", "3gp":
		cands = filterFunc(cands, func(s Stream) bool { return s.Ext() == base })
	default:
		if itag, err := strconv.Atoi(base); err == nil {
			for i := range streams {
				if streams[i].ITag == itag {
					s := streams[i]
					return &s, nil
				}
			}
			return nil, fmt.Errorf("itag %d not available", itag)
		}
		return nil, fmt.Errorf("unknown format selector %q", base)
	}

	for _, f := range filters {
		cands = filterFunc(cands, func(s Stream) bool { return matchFilter(s, f) })
	}
	if len(cands) == 0 {
		return nil, fmt.Errorf("no stream matched %q", token)
	}
	sortStreams(cands)
	chosen := cands[len(cands)-1]
	if !pickBest {
		chosen = cands[0]
	}
	return &chosen, nil
}

// splitFilters separates "bv[height<=720][ext=mp4]" into the base selector and
// its filter list.
func splitFilters(token string) (string, []formatFilter, error) {
	i := strings.IndexByte(token, '[')
	if i < 0 {
		return token, nil, nil
	}
	base := token[:i]
	rest := token[i:]
	var filters []formatFilter
	for rest != "" {
		if rest[0] != '[' {
			return "", nil, fmt.Errorf("malformed filter in %q", token)
		}
		end := strings.IndexByte(rest, ']')
		if end < 0 {
			return "", nil, fmt.Errorf("unterminated filter in %q", token)
		}
		f, err := parseFilter(rest[1:end])
		if err != nil {
			return "", nil, err
		}
		filters = append(filters, f)
		rest = rest[end+1:]
	}
	return base, filters, nil
}

func parseFilter(s string) (formatFilter, error) {
	for _, op := range []string{"<=", ">=", "!=", "=", "<", ">"} {
		if i := strings.Index(s, op); i >= 0 {
			return formatFilter{
				key: strings.TrimSpace(s[:i]),
				op:  op,
				val: strings.TrimSpace(s[i+len(op):]),
			}, nil
		}
	}
	return formatFilter{}, fmt.Errorf("filter %q has no operator", s)
}

func matchFilter(s Stream, f formatFilter) bool {
	switch f.key {
	case "ext":
		return compareStr(s.Ext(), f.op, f.val)
	case "vcodec":
		return compareStr(s.VideoCodec, f.op, f.val)
	case "acodec":
		return compareStr(s.AudioCodec, f.op, f.val)
	case "height":
		return compareInt(int64(s.Height), f.op, f.val)
	case "width":
		return compareInt(int64(s.Width), f.op, f.val)
	case "fps":
		return compareInt(int64(s.FPS), f.op, f.val)
	case "itag":
		return compareInt(int64(s.ITag), f.op, f.val)
	case "abr", "tbr", "br":
		return compareInt(s.Bitrate/1000, f.op, f.val)
	default:
		return false
	}
}

func compareStr(have, op, want string) bool {
	switch op {
	case "=":
		return have == want
	case "!=":
		return have != want
	}
	return false
}

func compareInt(have int64, op, want string) bool {
	n, err := strconv.ParseInt(want, 10, 64)
	if err != nil {
		return false
	}
	switch op {
	case "=":
		return have == n
	case "!=":
		return have != n
	case "<":
		return have < n
	case "<=":
		return have <= n
	case ">":
		return have > n
	case ">=":
		return have >= n
	}
	return false
}

// preferMuxed narrows to progressive (video+audio) streams, matching yt-dlp's
// best/worst (without '*'). When a video has no progressive stream it falls
// back to all video-bearing streams, then to everything, so the selector still
// resolves rather than failing on adaptive-only videos.
func preferMuxed(in []Stream) []Stream {
	if m := filterFunc(in, Stream.Muxed); len(m) > 0 {
		return m
	}
	if v := filterFunc(in, func(s Stream) bool { return s.HasVideo }); len(v) > 0 {
		return v
	}
	return in
}

func filterFunc(in []Stream, keep func(Stream) bool) []Stream {
	out := in[:0:0]
	for _, s := range in {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out
}

// sortStreams orders streams worst-to-best by resolution, then fps, then
// bitrate, so the last element is the best candidate.
func sortStreams(s []Stream) {
	sort.SliceStable(s, func(i, j int) bool {
		a, b := s[i], s[j]
		if pa, pb := a.Height*a.Width, b.Height*b.Width; pa != pb {
			return pa < pb
		}
		if a.FPS != b.FPS {
			return a.FPS < b.FPS
		}
		return a.Bitrate < b.Bitrate
	})
}
