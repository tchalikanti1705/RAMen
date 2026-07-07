package store

import (
	"math"
	"strconv"
)

// asHash returns the hash value of e, or ErrWrongType.
func asHash(e *entry) (map[string]string, error) {
	h, ok := e.val.(map[string]string)
	if !ok {
		return nil, ErrWrongType
	}
	return h, nil
}

// HSet sets the given field/value pairs on the hash at key (creating it if
// absent) and returns the number of newly created fields.
func (s *Store) HSet(key string, pairs map[string]string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var h map[string]string
	if found {
		var err error
		if h, err = asHash(e); err != nil {
			return 0, err
		}
	} else {
		h = make(map[string]string)
		sh.m[key] = &entry{val: h}
	}
	added := 0
	for f, v := range pairs {
		if _, ok := h[f]; !ok {
			added++
		}
		h[f] = v
	}
	return added, nil
}

// HSetNX sets field to value only when field does not already exist. It creates
// the hash when key is absent and reports whether the write happened.
func (s *Store) HSetNX(key, field, value string) (bool, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var h map[string]string
	if found {
		var err error
		if h, err = asHash(e); err != nil {
			return false, err
		}
	} else {
		h = make(map[string]string)
		sh.m[key] = &entry{val: h}
	}
	if _, ok := h[field]; ok {
		return false, nil
	}
	h[field] = value
	return true, nil
}

// HIncrBy adds delta to an integer field, treating a missing key or field as 0.
func (s *Store) HIncrBy(key, field string, delta int64) (int64, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var h map[string]string
	if found {
		var err error
		if h, err = asHash(e); err != nil {
			return 0, err
		}
	} else {
		h = make(map[string]string)
		sh.m[key] = &entry{val: h}
	}
	var cur int64
	if v, ok := h[field]; ok {
		n, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			return 0, ErrHashNotInteger
		}
		cur = n
	}
	if (delta > 0 && cur > math.MaxInt64-delta) || (delta < 0 && cur < math.MinInt64-delta) {
		return 0, ErrIntegerOverflow
	}
	cur += delta
	h[field] = strconv.FormatInt(cur, 10)
	return cur, nil
}

// HIncrByFloat adds delta to a float field, treating a missing key or field as 0.
func (s *Store) HIncrByFloat(key, field string, delta float64) (string, error) {
	if math.IsNaN(delta) || math.IsInf(delta, 0) {
		return "", ErrNotFloat
	}
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	var h map[string]string
	if found {
		var err error
		if h, err = asHash(e); err != nil {
			return "", err
		}
	} else {
		h = make(map[string]string)
		sh.m[key] = &entry{val: h}
	}
	var cur float64
	if v, ok := h[field]; ok {
		n, err := strconv.ParseFloat(v, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return "", ErrHashNotFloat
		}
		cur = n
	}
	cur += delta
	if math.IsNaN(cur) || math.IsInf(cur, 0) {
		return "", ErrFloatOverflow
	}
	// normalize negative zero so we return "0", not "-0", like Redis
	if cur == 0 {
		cur = 0
	}
	out := strconv.FormatFloat(cur, 'f', -1, 64)
	h[field] = out
	return out, nil
}

// HGet returns the value of a single field.
func (s *Store) HGet(key, field string) (string, bool, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return "", false, nil
	}
	h, err := asHash(e)
	if err != nil {
		return "", false, err
	}
	v, ok := h[field]
	return v, ok, nil
}

// HDel removes fields and returns how many were removed. The key is dropped
// when its last field is deleted.
func (s *Store) HDel(key string, fields ...string) (int, error) {
	sh := s.shardFor(key)
	sh.mu.Lock()
	defer sh.mu.Unlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return 0, nil
	}
	h, err := asHash(e)
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, f := range fields {
		if _, ok := h[f]; ok {
			delete(h, f)
			removed++
		}
	}
	if len(h) == 0 {
		delete(sh.m, key)
	}
	return removed, nil
}

// HGetAll returns all fields and values as a flat [f1,v1,f2,v2,...] slice.
func (s *Store) HGetAll(key string) ([]string, error) {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		return nil, nil
	}
	h, err := asHash(e)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(h)*2)
	for f, v := range h {
		out = append(out, f, v)
	}
	return out, nil
}

// hashView runs fn against the hash at key under a read lock; fn receives nil
// for a missing key.
func (s *Store) hashView(key string, fn func(map[string]string)) error {
	sh := s.shardFor(key)
	sh.mu.RLock()
	defer sh.mu.RUnlock()
	e, found := sh.getLive(key, s.now())
	if !found {
		fn(nil)
		return nil
	}
	h, err := asHash(e)
	if err != nil {
		return err
	}
	fn(h)
	return nil
}

// HKeys returns the field names of the hash.
func (s *Store) HKeys(key string) ([]string, error) {
	var out []string
	err := s.hashView(key, func(h map[string]string) {
		for f := range h {
			out = append(out, f)
		}
	})
	return out, err
}

// HVals returns the values of the hash.
func (s *Store) HVals(key string) ([]string, error) {
	var out []string
	err := s.hashView(key, func(h map[string]string) {
		for _, v := range h {
			out = append(out, v)
		}
	})
	return out, err
}

// HLen returns the number of fields in the hash.
func (s *Store) HLen(key string) (int, error) {
	n := 0
	err := s.hashView(key, func(h map[string]string) { n = len(h) })
	return n, err
}
