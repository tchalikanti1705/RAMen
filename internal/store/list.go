package store

import "errors"

// ErrNoSuchKey mirrors Redis' "no such key", returned by LSET on a missing key.
var ErrNoSuchKey = errors.New("ERR no such key")

// ErrIndexOutOfRange mirrors Redis' LSET error for an index outside the list.
var ErrIndexOutOfRange = errors.New("ERR index out of range")

// list is RAMen's list type. A slice is sufficient for V1's basic ops; the
// PRD scopes lists to LPUSH/RPUSH/LRANGE/etc. only.
type list struct {
	items []string
}

func asList(e *entry) (*list, error) {
	l, ok := e.val.(*list)
	if !ok {
		return nil, ErrWrongType
	}
	return l, nil
}

// push prepends (left) or appends (right) values, creating the list if absent,
// and returns the new length.
func (s *Store) push(key string, left bool, values []string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var l *list
	if found {
		var err error
		if l, err = asList(e); err != nil {
			return 0, err
		}
	} else {
		l = &list{}
		sh.m[key] = &entry{val: l}
	}
	if left {
		// LPUSH inserts each value at the head, so the last argument ends up
		// first — matching Redis semantics.
		for _, v := range values {
			l.items = append([]string{v}, l.items...)
		}
	} else {
		l.items = append(l.items, values...)
	}
	return len(l.items), nil
}

// LPush prepends values to the list at key.
func (s *Store) LPush(key string, values ...string) (int, error) {
	return s.push(key, true, values)
}

// RPush appends values to the list at key.
func (s *Store) RPush(key string, values ...string) (int, error) {
	return s.push(key, false, values)
}

// pop removes and returns one element from the head (left) or tail (right).
func (s *Store) pop(key string, left bool) (string, bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", false, nil
	}
	l, err := asList(e)
	if err != nil {
		return "", false, err
	}
	if len(l.items) == 0 {
		return "", false, nil
	}
	var v string
	if left {
		v, l.items = l.items[0], l.items[1:]
	} else {
		v, l.items = l.items[len(l.items)-1], l.items[:len(l.items)-1]
	}
	if len(l.items) == 0 {
		delete(sh.m, key)
	}
	return v, true, nil
}

// LPop removes and returns the head element.
func (s *Store) LPop(key string) (string, bool, error) { return s.pop(key, true) }

// RPop removes and returns the tail element.
func (s *Store) RPop(key string) (string, bool, error) { return s.pop(key, false) }

// LLen returns the list length.
func (s *Store) LLen(key string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return 0, nil
	}
	l, err := asList(e)
	if err != nil {
		return 0, err
	}
	return len(l.items), nil
}

// LIndex returns the element at index (negative counts from the tail).
func (s *Store) LIndex(key string, idx int) (string, bool, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return "", false, nil
	}
	l, err := asList(e)
	if err != nil {
		return "", false, err
	}
	i := idx
	if i < 0 {
		i += len(l.items)
	}
	if i < 0 || i >= len(l.items) {
		return "", false, nil
	}
	return l.items[i], true, nil
}

// LSet overwrites the element at index (negative counts from the tail) with
// value. It errors if the key is missing or the index is out of range.
func (s *Store) LSet(key string, idx int, value string) error {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return ErrNoSuchKey
	}
	l, err := asList(e)
	if err != nil {
		return err
	}
	i := idx
	if i < 0 {
		i += len(l.items)
	}
	if i < 0 || i >= len(l.items) {
		return ErrIndexOutOfRange
	}
	l.items[i] = value
	return nil
}

// LRem removes elements equal to value: count>0 walks head->tail, count<0 walks
// tail->head, count==0 removes every match. It returns how many were removed and
// drops the key when the list becomes empty.
func (s *Store) LRem(key string, count int, value string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return 0, nil
	}
	l, err := asList(e)
	if err != nil {
		return 0, err
	}
	removed := 0
	out := make([]string, 0, len(l.items))
	if count < 0 {
		// walk from the tail, dropping up to -count matches
		limit := -count
		if limit < 0 {
			// -count overflows when count is the int64 minimum; Redis treats
			// that as "remove every match", so cap it at the list length.
			limit = len(l.items)
		}
		remove := make([]bool, len(l.items))
		for i := len(l.items) - 1; i >= 0; i-- {
			if l.items[i] == value && removed < limit {
				remove[i] = true
				removed++
			}
		}
		for i, v := range l.items {
			if !remove[i] {
				out = append(out, v)
			}
		}
	} else {
		// walk from the head; count==0 means no limit
		for _, v := range l.items {
			if v == value && (count == 0 || removed < count) {
				removed++
				continue
			}
			out = append(out, v)
		}
	}
	if len(out) == 0 {
		delete(sh.m, key)
	} else {
		l.items = out
	}
	return removed, nil
}

// LTrim keeps only the elements in the inclusive range [start, stop], with
// Redis-style negative indices, and drops the key when the range is empty.
func (s *Store) LTrim(key string, start, stop int) error {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return nil
	}
	l, err := asList(e)
	if err != nil {
		return err
	}
	start, stop = normalizeRange(start, stop, len(l.items))
	if start > stop {
		delete(sh.m, key)
		return nil
	}
	kept := make([]string, stop-start+1)
	copy(kept, l.items[start:stop+1])
	l.items = kept
	return nil
}

// LInsert inserts value before or after the first element equal to pivot. It
// returns the new length, 0 when the key is missing, or -1 when pivot is absent.
func (s *Store) LInsert(key string, before bool, pivot, value string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return 0, nil
	}
	l, err := asList(e)
	if err != nil {
		return 0, err
	}
	pos := -1
	for i, v := range l.items {
		if v == pivot {
			pos = i
			break
		}
	}
	if pos < 0 {
		return -1, nil
	}
	if !before {
		pos++
	}
	l.items = append(l.items, "")
	copy(l.items[pos+1:], l.items[pos:])
	l.items[pos] = value
	return len(l.items), nil
}

// LRange returns the elements in the inclusive index range [start, stop],
// where negative indices count from the tail (Redis semantics).
func (s *Store) LRange(key string, start, stop int) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.peekLive(key, s.now())
	if !found {
		return nil, nil
	}
	l, err := asList(e)
	if err != nil {
		return nil, err
	}
	start, stop = normalizeRange(start, stop, len(l.items))
	if start > stop {
		return []string{}, nil
	}
	out := make([]string, stop-start+1)
	copy(out, l.items[start:stop+1])
	return out, nil
}

// normalizeRange resolves negative indices and clamps to [0, n-1], returning a
// range that, when start>stop, denotes an empty selection.
func normalizeRange(start, stop, n int) (int, int) {
	if start < 0 {
		start += n
	}
	if stop < 0 {
		stop += n
	}
	if start < 0 {
		start = 0
	}
	if stop >= n {
		stop = n - 1
	}
	return start, stop
}
