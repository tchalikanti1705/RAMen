package store

import (
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/vector"
)

// Record is the serialisable form of a single key, used by the persist package
// to snapshot and restore the keyspace. Exactly one of the type-specific
// fields is populated according to Type.
type Record struct {
	Key            string
	Type           string // "string","hash","list","set","zset","vector"
	ExpireAtUnixMs int64  // 0 == no expiry
	Str            string
	Hash           map[string]string
	List           []string
	Set            []string
	ZSet           []ZMember
	Vectors        []VecRecord
	VecDim         int
}

// VecRecord is one stored vector inside a vector collection.
type VecRecord struct {
	ID   string
	Vec  []float32
	Meta string
	// ExpireUnix is the Unix second the item expires at; 0 == no expiry.
	// Snapshots written before this field existed simply lack it, and gob
	// decodes a missing field as 0, so old entries load as never expiring.
	ExpireUnix int64
}

// Export returns a snapshot of every live key. It is safe to call while the
// store is serving traffic; each shard is read-locked in turn.
func (s *Store) Export() []Record {
	now := s.now()
	var out []Record
	for _, sh := range s.shards {
		sh.mu.RLock()
		for k, e := range sh.m {
			if e.expired(now) {
				continue
			}
			rec := Record{Key: k}
			if !e.expireAt.IsZero() {
				rec.ExpireAtUnixMs = e.expireAt.UnixMilli()
			}
			switch v := e.val.(type) {
			case string:
				rec.Type, rec.Str = "string", v
			case map[string]string:
				rec.Type, rec.Hash = "hash", v
			case *list:
				rec.Type, rec.List = "list", v.items
			case map[string]struct{}:
				rec.Type = "set"
				for m := range v {
					rec.Set = append(rec.Set, m)
				}
			case *zset:
				rec.Type, rec.ZSet = "zset", v.sortedMembers()
			case *vector.Collection:
				rec.Type, rec.VecDim = "vector", v.Dim
				for _, it := range v.Items() {
					// An item already expired at save time is garbage awaiting
					// the sweeper; keep it out of the snapshot.
					if it.ExpireUnix != 0 && now.Unix() > it.ExpireUnix {
						continue
					}
					rec.Vectors = append(rec.Vectors, VecRecord{ID: it.ID, Vec: it.Vec, Meta: it.Meta, ExpireUnix: it.ExpireUnix})
				}
				if len(rec.Vectors) == 0 {
					// Every item was expired; don't resurrect an empty key.
					continue
				}
			default:
				continue
			}
			out = append(out, rec)
		}
		sh.mu.RUnlock()
	}
	return out
}

// Import loads records into the store, replacing any existing data. Records
// whose expiry is already in the past are skipped.
func (s *Store) Import(recs []Record) {
	now := s.now()
	for _, rec := range recs {
		var expireAt time.Time
		if rec.ExpireAtUnixMs != 0 {
			expireAt = time.UnixMilli(rec.ExpireAtUnixMs)
			if now.After(expireAt) {
				continue
			}
		}
		val := recordValue(rec, now.Unix())
		if val == nil {
			continue
		}
		sh := s.shardFor(rec.Key)
		sh.mu.Lock()
		sh.m[rec.Key] = &entry{val: val, expireAt: expireAt}
		sh.mu.Unlock()
	}
}

func recordValue(rec Record, nowUnix int64) value {
	switch rec.Type {
	case "string":
		return rec.Str
	case "hash":
		if rec.Hash == nil {
			rec.Hash = map[string]string{}
		}
		return rec.Hash
	case "list":
		return &list{items: rec.List}
	case "set":
		set := make(map[string]struct{}, len(rec.Set))
		for _, m := range rec.Set {
			set[m] = struct{}{}
		}
		return set
	case "zset":
		z := &zset{scores: make(map[string]float64, len(rec.ZSet))}
		for _, m := range rec.ZSet {
			z.scores[m.Member] = m.Score
		}
		return z
	case "vector":
		c := vector.NewCollection()
		for _, vr := range rec.Vectors {
			// Skip items whose deadline passed while the snapshot sat on disk.
			if vr.ExpireUnix != 0 && nowUnix > vr.ExpireUnix {
				continue
			}
			c.Set(vr.ID, vr.Vec, vr.Meta, vr.ExpireUnix)
		}
		if c.Len() == 0 {
			return nil
		}
		return c
	default:
		return nil
	}
}
