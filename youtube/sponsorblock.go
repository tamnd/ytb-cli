package youtube

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// sponsorBlockAPI is the public SponsorBlock segment endpoint. It is a
// community service independent of YouTube; no key is required.
const sponsorBlockAPI = "https://sponsor.ajay.app/api/skipSegments"

// SponsorSegment is one community-submitted segment of a video.
type SponsorSegment struct {
	Category string  `json:"category"`
	Action   string  `json:"action"`
	Start    float64 `json:"start_seconds"`
	End      float64 `json:"end_seconds"`
	UUID     string  `json:"uuid"`
}

// AllSponsorCategories are the segment categories SponsorBlock publishes.
var AllSponsorCategories = []string{
	"sponsor", "selfpromo", "interaction", "intro", "outro",
	"preview", "music_offtopic", "filler",
}

// SponsorSegments fetches segments for a video. When categories is empty all
// categories are requested.
func (c *Client) SponsorSegments(ctx context.Context, videoID string, categories []string) ([]SponsorSegment, error) {
	if len(categories) == 0 {
		categories = AllSponsorCategories
	}
	cats, _ := json.Marshal(categories)
	q := url.Values{}
	q.Set("videoID", videoID)
	q.Set("categories", string(cats))
	reqURL := sponsorBlockAPI + "?" + q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "ytb-cli")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNotFound {
		return nil, nil // no segments submitted for this video
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sponsorblock: HTTP %d", resp.StatusCode)
	}
	var raw []struct {
		Category string    `json:"category"`
		Action   string    `json:"actionType"`
		Segment  []float64 `json:"segment"`
		UUID     string    `json:"UUID"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("sponsorblock: %w", err)
	}
	out := make([]SponsorSegment, 0, len(raw))
	for _, r := range raw {
		if len(r.Segment) != 2 {
			continue
		}
		out = append(out, SponsorSegment{
			Category: r.Category,
			Action:   r.Action,
			Start:    r.Segment[0],
			End:      r.Segment[1],
			UUID:     r.UUID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Start < out[j].Start })
	return out, nil
}

// SponsorChapters builds an ffmpeg metadata file marking sponsor segments so
// players can show them as chapters. It is an alternative to cutting them out.
func SponsorChapters(segs []SponsorSegment) string {
	var b strings.Builder
	b.WriteString(";FFMETADATA1\n")
	for _, s := range segs {
		fmt.Fprintf(&b, "[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n",
			int64(s.Start*1000), int64(s.End*1000), s.Category)
	}
	return b.String()
}
