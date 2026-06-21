package youtube

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// Stream is one downloadable format with the data needed to resolve its URL.
// It is the download-oriented sibling of VideoFormat (which is metadata only).
type Stream struct {
	ITag            int    `json:"itag"`
	MimeType        string `json:"mime_type"`
	Container       string `json:"container"` // mp4, webm, m4a, 3gp
	VideoCodec      string `json:"video_codec"`
	AudioCodec      string `json:"audio_codec"`
	Quality         string `json:"quality"`
	QualityLabel    string `json:"quality_label"`
	Width           int    `json:"width"`
	Height          int    `json:"height"`
	FPS             int    `json:"fps"`
	Bitrate         int64  `json:"bitrate"`
	ContentLength   int64  `json:"content_length"`
	AudioQuality    string `json:"audio_quality"`
	AudioChannels   int    `json:"audio_channels"`
	AudioSampleRate int    `json:"audio_sample_rate"`
	IsAdaptive      bool   `json:"is_adaptive"`
	HasVideo        bool   `json:"has_video"`
	HasAudio        bool   `json:"has_audio"`

	url             string
	signatureCipher string
	userAgent       string
}

// AudioOnly reports a track carrying audio but no video.
func (s Stream) AudioOnly() bool { return s.HasAudio && !s.HasVideo }

// VideoOnly reports a track carrying video but no audio.
func (s Stream) VideoOnly() bool { return s.HasVideo && !s.HasAudio }

// Muxed reports a progressive track carrying both audio and video.
func (s Stream) Muxed() bool { return s.HasVideo && s.HasAudio }

// UserAgent returns the client user agent that produced this stream URL.
func (s Stream) UserAgent() string { return s.userAgent }

// Ext returns the file extension to use when saving this stream on its own.
func (s Stream) Ext() string {
	if s.AudioOnly() {
		switch s.Container {
		case "mp4":
			return "m4a"
		case "webm":
			return "webm"
		}
	}
	if s.Container != "" {
		return s.Container
	}
	return "bin"
}

// StreamManifest is the resolved set of streams for one video plus the player
// URL needed to decipher them.
type StreamManifest struct {
	VideoID  string   `json:"video_id"`
	Title    string   `json:"title"`
	Author   string   `json:"author"`
	Duration int      `json:"duration_seconds"`
	IsLive   bool     `json:"is_live"`
	HLSURL   string   `json:"hls_url,omitempty"`
	DASHURL  string   `json:"dash_url,omitempty"`
	Streams  []Stream `json:"streams"`

	playerURL string
}

// StreamManifest resolves the downloadable streams for a video. It leads with
// the ANDROID_VR client (plain, token-free URLs) and falls back to the watch
// page's player response (ciphered URLs solved via base.js).
func (c *Client) StreamManifest(ctx context.Context, idOrURL string) (*StreamManifest, error) {
	videoID := ExtractVideoID(idOrURL)
	if videoID == "" {
		videoID = idOrURL
	}

	var webPR map[string]any
	var pageData *PageData
	playerURL := ""
	if data, _, err := c.FetchPageData(ctx, NormalizeVideoURL(idOrURL)); err == nil && data != nil {
		pageData = data
		playerURL = extractPlayerJSURL(data.HTML)
		if pr, ok := data.PlayerResp.(map[string]any); ok {
			webPR = pr
		}
	}

	it := NewInnerTube(c)
	signatureTimestamp := c.signatureTimestampFor(ctx, playerURL)
	visitorData := visitorDataFromPlayer(webPR)
	if pageData != nil && pageData.VisitorData != "" {
		visitorData = pageData.VisitorData
	}
	avr, _ := it.AndroidVRPlayer(ctx, videoID, visitorData, signatureTimestamp)
	safariPR, _ := it.WebSafariPlayer(ctx, videoID, visitorData, signatureTimestamp)

	responses := []map[string]any{avr, webPR, safariPR}
	chosen := firstPlayerWithStreams(responses...)
	if chosen == nil {
		if reason := playabilityReason(avr); reason != "" {
			return nil, fmt.Errorf("video unavailable: %s", reason)
		}
		if reason := playabilityReason(webPR); reason != "" {
			return nil, fmt.Errorf("video unavailable: %s", reason)
		}
		if reason := playabilityReason(safariPR); reason != "" {
			return nil, fmt.Errorf("video unavailable: %s", reason)
		}
		return nil, fmt.Errorf("no downloadable streams for %s (formats may be SABR-only or require a token)", videoID)
	}

	m := &StreamManifest{VideoID: videoID, playerURL: playerURL}
	if vd := mapValue(chosen, "videoDetails"); vd != nil {
		m.Title = stringValue(vd["title"])
		m.Author = stringValue(vd["author"])
		m.Duration = int(int64Value(vd["lengthSeconds"]))
		if b, ok := vd["isLiveContent"].(bool); ok {
			m.IsLive = b
		}
	}
	if sd := mapValue(chosen, "streamingData"); sd != nil {
		m.HLSURL = stringValue(sd["hlsManifestUrl"])
		m.DASHURL = stringValue(sd["dashManifestUrl"])
	}
	m.Streams = mergeStreams(streamSource{avr, androidVRUA}, streamSource{webPR, c.userAgents[0]}, streamSource{safariPR, webSafariUA})
	return m, nil
}

// ResolveStreamURL turns a stream into a fetchable googlevideo URL, deciphering
// the signature and transforming the n parameter as needed.
func (c *Client) ResolveStreamURL(ctx context.Context, m *StreamManifest, s *Stream) (string, error) {
	// A plain URL with no known player can be returned as-is (n untransformed);
	// it may be throttled but still downloads.
	if m.playerURL == "" {
		if s.url != "" {
			return s.url, nil
		}
		return "", fmt.Errorf("format %d needs deciphering but no player JS was found", s.ITag)
	}
	pc, err := c.cipherFor(ctx, m.playerURL)
	if err != nil {
		if s.url != "" {
			return s.url, nil
		}
		return "", err
	}
	return pc.resolveURL(s)
}

func hasStreams(pr map[string]any) bool {
	sd := mapValue(pr, "streamingData")
	if sd == nil {
		return false
	}
	for _, key := range []string{"formats", "adaptiveFormats"} {
		for _, item := range arrayValue(sd[key]) {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			if streamMapResolvable(m) {
				return true
			}
		}
	}
	return false
}

func firstPlayerWithStreams(responses ...map[string]any) map[string]any {
	for _, pr := range responses {
		if hasStreams(pr) {
			return pr
		}
	}
	return nil
}

func streamMapResolvable(m map[string]any) bool {
	if stringValue(m["url"]) != "" {
		return true
	}
	return strings.Contains(firstNonEmpty(stringValue(m["signatureCipher"]), stringValue(m["cipher"])), "url=")
}

func visitorDataFromPlayer(pr map[string]any) string {
	if rc := mapValue(pr, "responseContext"); rc != nil {
		return stringValue(rc["visitorData"])
	}
	return ""
}

func playabilityReason(pr map[string]any) string {
	ps := mapValue(pr, "playabilityStatus")
	if ps == nil {
		return ""
	}
	if stringValue(ps["status"]) == "OK" {
		return ""
	}
	if r := stringValue(ps["reason"]); r != "" {
		return r
	}
	return stringValue(ps["status"])
}

func parseStreamsWithUserAgent(pr map[string]any, ua string) []Stream {
	sd := mapValue(pr, "streamingData")
	if sd == nil {
		return nil
	}
	var out []Stream
	for _, item := range arrayValue(sd["formats"]) {
		if s := parseStream(item, false); s != nil {
			s.userAgent = ua
			out = append(out, *s)
		}
	}
	for _, item := range arrayValue(sd["adaptiveFormats"]) {
		if s := parseStream(item, true); s != nil {
			s.userAgent = ua
			out = append(out, *s)
		}
	}
	return out
}

func parseStream(item any, adaptive bool) *Stream {
	m, ok := item.(map[string]any)
	if !ok {
		return nil
	}
	itag := int(int64Value(m["itag"]))
	if itag == 0 || !streamMapResolvable(m) {
		return nil
	}
	s := &Stream{
		ITag:            itag,
		MimeType:        stringValue(m["mimeType"]),
		Quality:         stringValue(m["quality"]),
		QualityLabel:    stringValue(m["qualityLabel"]),
		Width:           int(int64Value(m["width"])),
		Height:          int(int64Value(m["height"])),
		FPS:             int(int64Value(m["fps"])),
		Bitrate:         int64Value(m["bitrate"]),
		AudioQuality:    stringValue(m["audioQuality"]),
		AudioChannels:   int(int64Value(m["audioChannels"])),
		AudioSampleRate: atoiSafe(stringValue(m["audioSampleRate"])),
		IsAdaptive:      adaptive,
		url:             stringValue(m["url"]),
		signatureCipher: firstNonEmpty(stringValue(m["signatureCipher"]), stringValue(m["cipher"])),
	}
	if cl := stringValue(m["contentLength"]); cl != "" {
		s.ContentLength, _ = strconv.ParseInt(cl, 10, 64)
	}
	s.Container, s.VideoCodec, s.AudioCodec = parseMime(s.MimeType)
	s.HasVideo = s.VideoCodec != ""
	s.HasAudio = s.AudioCodec != ""
	return s
}

type streamSource struct {
	playerResponse map[string]any
	userAgent      string
}

func mergeStreams(sources ...streamSource) []Stream {
	seen := map[int]Stream{}
	for _, src := range sources {
		for _, s := range parseStreamsWithUserAgent(src.playerResponse, src.userAgent) {
			prev, ok := seen[s.ITag]
			if !ok || streamScore(s) > streamScore(prev) {
				seen[s.ITag] = s
			}
		}
	}
	out := make([]Stream, 0, len(seen))
	for _, s := range seen {
		out = append(out, s)
	}
	return out
}

func streamScore(s Stream) int64 {
	score := s.Bitrate
	if s.ContentLength > 0 {
		score += s.ContentLength / 1024
	}
	if s.url != "" {
		score += 1 << 40
	}
	return score
}

// parseMime splits `video/mp4; codecs="avc1.4d401f, mp4a.40.2"` into the
// container and its video/audio codecs. Audio-only mimes yield no video codec.
func parseMime(mime string) (container, vcodec, acodec string) {
	typePart := mime
	if i := strings.Index(mime, ";"); i >= 0 {
		typePart = mime[:i]
	}
	kind := ""
	if i := strings.Index(typePart, "/"); i >= 0 {
		kind = typePart[:i]
		container = typePart[i+1:]
	}
	var codecs []string
	if i := strings.Index(mime, `codecs="`); i >= 0 {
		rest := mime[i+len(`codecs="`):]
		if j := strings.Index(rest, `"`); j >= 0 {
			for _, c := range strings.Split(rest[:j], ",") {
				if c = strings.TrimSpace(c); c != "" {
					codecs = append(codecs, c)
				}
			}
		}
	}
	switch kind {
	case "audio":
		if len(codecs) > 0 {
			acodec = codecs[0]
		}
	case "video":
		if len(codecs) > 0 {
			vcodec = codecs[0]
		}
		if len(codecs) > 1 {
			acodec = codecs[1] // progressive: video,audio
		}
	}
	return container, vcodec, acodec
}

func atoiSafe(s string) int {
	n, _ := strconv.Atoi(s)
	return n
}
