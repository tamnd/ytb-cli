package ytb

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestPurgeReachesShardedEntries is the regression this file exists for.
//
// path shards on the first two hex characters of the key, so every entry lives
// in <dir>/ab/rest.json and the top level holds nothing but 256 directories.
// Purge used to read the top level and skip anything that was a directory,
// which is all of it, so it walked a full cache, deleted nothing, and reported
// zero. Nobody noticed because no command called it.
func TestPurgeReachesShardedEntries(t *testing.T) {
	c := NewCache(t.TempDir(), time.Hour)
	for i, url := range []string{"https://a.example/1", "https://b.example/2", "https://c.example/3"} {
		c.Put(CacheKey{Method: "GET", URL: url}, 200, []byte{byte(i)})
	}

	info, err := c.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Entries != 3 {
		t.Fatalf("stored 3 entries, Info found %d", info.Entries)
	}

	n, err := c.Purge()
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("Purge reported %d deleted, want 3", n)
	}

	after, err := c.Info()
	if err != nil {
		t.Fatal(err)
	}
	if after.Entries != 0 {
		t.Errorf("%d entries survived Purge", after.Entries)
	}
	// Reporting a count is not the same as the files being gone, and the bug
	// this test is about was exactly that gap in reverse.
	var left int
	_ = filepath.Walk(c.Dir(), func(_ string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			left++
		}
		return nil
	})
	if left != 0 {
		t.Errorf("%d files still on disk after Purge", left)
	}
}

// TestInfoCountsFreshAgainstTheCurrentTTL: the same file is fresh at one --cache-ttl
// and stale at another, so freshness is computed and not stored.
func TestInfoCountsFreshAgainstTheCurrentTTL(t *testing.T) {
	dir := t.TempDir()
	c := NewCache(dir, time.Hour)
	c.Put(CacheKey{Method: "GET", URL: "https://a.example/1"}, 200, []byte("x"))

	info, err := c.Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Fresh != 1 || info.Stale != 0 {
		t.Errorf("with a 1h ttl: fresh=%d stale=%d, want 1 and 0", info.Fresh, info.Stale)
	}
	if info.Bytes <= 0 {
		t.Errorf("size came back %d", info.Bytes)
	}

	// Same directory, shorter ttl. Nothing on disk changed.
	short, err := NewCache(dir, time.Nanosecond).Info()
	if err != nil {
		t.Fatal(err)
	}
	if short.Fresh != 0 || short.Stale != 1 {
		t.Errorf("with a 1ns ttl: fresh=%d stale=%d, want 0 and 1", short.Fresh, short.Stale)
	}
}

// TestInfoOnAnAbsentDirectory: asking about a cache nothing has written to yet
// is a zero, not an error.
func TestInfoOnAnAbsentDirectory(t *testing.T) {
	info, err := NewCache(filepath.Join(t.TempDir(), "nope"), time.Hour).Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Entries != 0 {
		t.Errorf("found %d entries in a directory that does not exist", info.Entries)
	}
}
