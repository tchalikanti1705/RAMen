package store

import (
	"sort"
	"time"
)

// Cursor iteration (SCAN and its typed variants).
//
// Go randomises map iteration order on every range, so the cursor cannot be a
// count of "keys already seen": two calls with the same cursor would walk
// different keys and both skip and duplicate. Instead we impose an order the
// map does not give us by sorting keys, and the cursor encodes a position in
// that order.
//
// For the keyspace SCAN the cursor packs two numbers into one integer: the
// shard index (there are shardCount shards) in the low part and the offset into
// that shard's sorted key slice in the high part:
//
//	cursor = offset*shardCount + shardIndex
//
// Cursor 0 therefore starts at shard 0, offset 0, and we return 0 once the last
// shard is exhausted. The cursor stays a plain integer, which matters because
// many clients parse it as one, and no per-connection server state is needed.
//
// The guarantee is weaker than a snapshot: keys added or deleted mid-iteration
// shift the offsets, so a key present for the whole scan can be missed or
// returned twice. Redis makes a similarly weak promise, and this is documented
// in docs/commands.md.

// sortedLiveKeys returns the shard's live (non-expired) keys in sorted order. It
// takes the shard read lock itself so a scan never holds a lock across shards.
func (sh *shard) sortedLiveKeys(now time.Time) []string {
	sh.mu.RLock()
	keys := make([]string, 0, len(sh.m))
	for k, e := range sh.m {
		if !e.expired(now) {
			keys = append(keys, k)
		}
	}
	sh.mu.RUnlock()
	sort.Strings(keys)
	return keys
}

// Scan iterates the keyspace one page at a time. Starting from cursor (0 begins
// a fresh iteration), it examines up to count keys, advancing across shards as
// needed, and returns the keys in that window whose name matches the glob
// pattern (an empty pattern matches everything) together with the next cursor.
// The next cursor is 0 when iteration is complete. Because MATCH is applied only
// after the count keys are fetched, a page can legitimately come back empty with
// a non-zero cursor; callers must keep going until the cursor is 0.
func (s *Store) Scan(cursor uint64, match string, count int) (uint64, []string) {
	if count <= 0 {
		count = defaultScanCount
	}
	now := s.now()
	shardIdx := int(cursor % shardCount)
	offset := int(cursor / shardCount)

	var out []string
	examined := 0
	for shardIdx < shardCount {
		keys := s.shards[shardIdx].sortedLiveKeys(now)
		i := offset
		for i < len(keys) && examined < count {
			if match == "" || matchPattern(match, keys[i]) {
				out = append(out, keys[i])
			}
			i++
			examined++
		}
		if i < len(keys) {
			// Stopped mid-shard because the count budget ran out; resume here.
			return uint64(i)*shardCount + uint64(shardIdx), out
		}
		// This shard is exhausted; roll to the next one at offset 0.
		shardIdx++
		offset = 0
		if examined >= count {
			if shardIdx >= shardCount {
				return 0, out
			}
			return uint64(shardIdx), out // offset 0 in the next shard
		}
	}
	return 0, out
}

// defaultScanCount is the COUNT hint used when the client does not supply one,
// matching Redis' default of 10.
const defaultScanCount = 10

// HScan iterates the fields of the hash at key, returning a page of matching
// field/value pairs as a flat [field, value, ...] slice plus the next cursor. A
// missing key yields cursor 0 and no elements; a key of the wrong type returns
// ErrWrongType. Like SCAN, MATCH is applied (to the field name) after the count
// window is taken, so a page can be empty with a non-zero cursor.
func (s *Store) HScan(key string, cursor uint64, match string, count int) (uint64, []string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil, nil
	}
	h, err := asHash(e)
	if err != nil {
		return 0, nil, err
	}
	fields := make([]string, 0, len(h))
	for f := range h {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	next, window := scanWindow(fields, cursor, count)
	out := make([]string, 0, len(window)*2)
	for _, f := range window {
		if match == "" || matchPattern(match, f) {
			out = append(out, f, h[f])
		}
	}
	return next, out, nil
}

// SScan iterates the members of the set at key, returning a page of matching
// members plus the next cursor. A missing key yields cursor 0 and no elements;
// a key of the wrong type returns ErrWrongType.
func (s *Store) SScan(key string, cursor uint64, match string, count int) (uint64, []string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil, nil
	}
	set, err := asSet(e)
	if err != nil {
		return 0, nil, err
	}
	members := make([]string, 0, len(set))
	for m := range set {
		members = append(members, m)
	}
	sort.Strings(members)
	next, window := scanWindow(members, cursor, count)
	out := make([]string, 0, len(window))
	for _, m := range window {
		if match == "" || matchPattern(match, m) {
			out = append(out, m)
		}
	}
	return next, out, nil
}

// scanWindow walks a single sorted slice (the fields of a hash, members of a set
// or sorted set) starting at cursor and returns up to count entries plus the
// next cursor. The next cursor is 0 once the slice is exhausted, and an
// out-of-range cursor (including one left dangling by deletions) also terminates
// cleanly at 0 rather than panicking. MATCH filtering is left to the caller so a
// page may still come back empty with a non-zero cursor.
func scanWindow(sorted []string, cursor uint64, count int) (uint64, []string) {
	if count <= 0 {
		count = defaultScanCount
	}
	if cursor >= uint64(len(sorted)) {
		return 0, nil
	}
	start := int(cursor)
	end := start + count
	if end >= len(sorted) {
		return 0, sorted[start:]
	}
	return uint64(end), sorted[start:end]
}
