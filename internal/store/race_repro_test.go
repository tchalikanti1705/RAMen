package store

import (
	"strconv"
	"sync"
	"testing"
	"time"
)

// TestConcurrentExpiredReads is a regression test for the data race in #23:
// read commands used to lazily delete expired keys while holding only a shard
// read lock, so two concurrent readers could write the same map at once and
// crash the process with "concurrent map read and map write".
//
// Reading an expired key must report it as missing without mutating the map.
// Run with the race detector to catch a regression: go test -race ./internal/store/
func TestConcurrentExpiredReads(t *testing.T) {
	s := New()

	// Freeze the clock, populate keys with a TTL, then jump past it so every
	// key is expired but still sitting in the shard maps (no sweeper running).
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	const n = 2000
	for i := 0; i < n; i++ {
		k := "k" + strconv.Itoa(i)
		s.Set(k, "v", SetOptions{TTL: 30 * time.Second, HasEx: true})
		if _, err := s.HSet("h"+strconv.Itoa(i), map[string]string{"f": "v"}); err != nil {
			t.Fatalf("HSet: %v", err)
		}
		s.Expire("h"+strconv.Itoa(i), 30*time.Second)
	}
	cur = cur.Add(time.Minute) // everything is expired now

	// Hammer two read-lock paths (store-level TTL and hash-level HGet) from
	// many goroutines at once. Before the fix this races on delete(sh.m, key).
	const goroutines = 8
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < n; i++ {
				if _, _, ok := s.TTL("k" + strconv.Itoa(i)); ok {
					t.Errorf("expired key reported as live")
				}
				if _, ok, _ := s.HGet("h"+strconv.Itoa(i), "f"); ok {
					t.Errorf("expired hash field reported as live")
				}
			}
		}()
	}
	wg.Wait()
}
