package store

import (
	"strconv"
	"sync"
	"testing"
)

// twoKeysOnDifferentShards returns two key names that hash to different shards,
// so a RENAME between them actually moves the entry across shards.
func twoKeysOnDifferentShards(s *Store) (string, string) {
	base := "k0"
	bi := s.shardIndex(base)
	for i := 1; ; i++ {
		other := "k" + strconv.Itoa(i)
		if s.shardIndex(other) != bi {
			return base, other
		}
	}
}

// TestExportConsistentUnderRename guards against the snapshot losing or
// duplicating a key that RENAME moves between shards. Only one of {a, b} exists
// at any instant (they are ping-ponged), so every Export must observe exactly
// one of them. Before Export locked all shards for the whole scan, a snapshot
// overlapping a cross-shard rename could see the key under both names or under
// neither. Run with -race to also catch torn map access.
func TestExportConsistentUnderRename(t *testing.T) {
	s := New()
	a, b := twoKeysOnDifferentShards(s)
	s.Set(a, "v", SetOptions{})

	stop := make(chan struct{})
	var pinger sync.WaitGroup

	// Ping-pong the single key between the two shards until stopped.
	pinger.Add(1)
	go func() {
		defer pinger.Done()
		src, dst := a, b
		for {
			select {
			case <-stop:
				return
			default:
			}
			if err := s.Rename(src, dst); err != nil {
				t.Errorf("Rename(%s,%s): %v", src, dst, err)
				return
			}
			src, dst = dst, src
		}
	}()

	// Snapshot repeatedly; each snapshot must contain exactly one of a/b.
	for i := 0; i < 200000; i++ {
		n := 0
		for _, rec := range s.Export() {
			if rec.Key == a || rec.Key == b {
				n++
			}
		}
		if n != 1 {
			t.Fatalf("snapshot saw the renamed key %d times, want exactly 1", n)
		}
	}

	close(stop)
	pinger.Wait()
}
