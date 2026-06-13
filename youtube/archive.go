package youtube

import (
	"bufio"
	"os"
	"sort"
	"strings"
	"sync"
)

// DownloadArchive tracks which videos have already been downloaded, mirroring
// yt-dlp's --download-archive file (one "youtube <id>" record per line). It is
// safe for concurrent use.
type DownloadArchive struct {
	path string
	mu   sync.Mutex
	seen map[string]bool
}

// archivePrefix labels records so an archive can be shared across extractors.
const archivePrefix = "youtube "

// OpenArchive loads (or initializes) an archive file. A missing file is treated
// as an empty archive; it is created on the first Add.
func OpenArchive(path string) (*DownloadArchive, error) {
	a := &DownloadArchive{path: path, seen: map[string]bool{}}
	if path == "" {
		return a, nil
	}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return a, nil
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		a.seen[normalizeArchiveKey(line)] = true
	}
	return a, sc.Err()
}

// Has reports whether videoID is already recorded.
func (a *DownloadArchive) Has(videoID string) bool {
	if a == nil || a.path == "" {
		return false
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.seen[normalizeArchiveKey(archivePrefix+videoID)]
}

// Add records videoID and appends it to the archive file. It is a no-op when
// the archive has no path or the id is already present.
func (a *DownloadArchive) Add(videoID string) error {
	if a == nil || a.path == "" {
		return nil
	}
	key := normalizeArchiveKey(archivePrefix + videoID)
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seen[key] {
		return nil
	}
	f, err := os.OpenFile(a.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	if _, err := f.WriteString(archivePrefix + videoID + "\n"); err != nil {
		return err
	}
	a.seen[key] = true
	return nil
}

// IDs returns the recorded video IDs in sorted order.
func (a *DownloadArchive) IDs() []string {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]string, 0, len(a.seen))
	for k := range a.seen {
		out = append(out, strings.TrimPrefix(k, archivePrefix))
	}
	sort.Strings(out)
	return out
}

func normalizeArchiveKey(line string) string {
	return strings.Join(strings.Fields(line), " ")
}
