package store

import (
	"math"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestStringSetGet(t *testing.T) {
	s := New()
	s.Set("k", "v", SetOptions{})
	got, ok, err := s.Get("k")
	if err != nil || !ok || got != "v" {
		t.Fatalf("Get = %q %v %v", got, ok, err)
	}
}

func TestSetNX(t *testing.T) {
	s := New()
	if !s.Set("k", "v1", SetOptions{NX: true}) {
		t.Fatal("first NX set should succeed")
	}
	if s.Set("k", "v2", SetOptions{NX: true}) {
		t.Fatal("second NX set should fail")
	}
	got, _, _ := s.Get("k")
	if got != "v1" {
		t.Fatalf("got %q", got)
	}
}

func TestIncrBy(t *testing.T) {
	s := New()
	n, err := s.IncrBy("c", 5)
	if err != nil || n != 5 {
		t.Fatalf("IncrBy = %d %v", n, err)
	}
	n, _ = s.IncrBy("c", -2)
	if n != 3 {
		t.Fatalf("got %d", n)
	}
}

func TestHashSetNX(t *testing.T) {
	s := New()
	ok, err := s.HSetNX("h", "f", "v1")
	if err != nil || !ok {
		t.Fatalf("HSetNX create = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", "f"); err != nil || !found || v != "v1" {
		t.Fatalf("HGet after HSetNX = %q %v %v", v, found, err)
	}
	ok, err = s.HSetNX("h", "f", "v2")
	if err != nil || ok {
		t.Fatalf("HSetNX existing = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", "f"); err != nil || !found || v != "v1" {
		t.Fatalf("HSetNX overwrote existing field: %q %v %v", v, found, err)
	}
	ok, err = s.HSetNX("h", "", "")
	if err != nil || !ok {
		t.Fatalf("HSetNX empty field/value = %v %v", ok, err)
	}
	if v, found, err := s.HGet("h", ""); err != nil || !found || v != "" {
		t.Fatalf("HGet empty field = %q %v %v", v, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HSetNX("str", "f", "v"); err != ErrWrongType {
		t.Fatalf("HSetNX wrong type = %v", err)
	}
}

func TestHashIncrBy(t *testing.T) {
	s := New()
	n, err := s.HIncrBy("h", "count", 5)
	if err != nil || n != 5 {
		t.Fatalf("HIncrBy create = %d %v", n, err)
	}
	if v, found, err := s.HGet("h", "count"); err != nil || !found || v != "5" {
		t.Fatalf("HGet after HIncrBy create = %q %v %v", v, found, err)
	}
	n, err = s.HIncrBy("h", "count", -2)
	if err != nil || n != 3 {
		t.Fatalf("HIncrBy existing = %d %v", n, err)
	}
	if v, found, err := s.HGet("h", "count"); err != nil || !found || v != "3" {
		t.Fatalf("HGet after HIncrBy existing = %q %v %v", v, found, err)
	}
	s.HSet("h", map[string]string{"bad": "abc"})
	if _, err := s.HIncrBy("h", "bad", 1); err != ErrHashNotInteger {
		t.Fatalf("HIncrBy non-integer field = %v", err)
	}
	if v, found, err := s.HGet("h", "bad"); err != nil || !found || v != "abc" {
		t.Fatalf("HIncrBy changed bad field = %q %v %v", v, found, err)
	}
	max := strconv.FormatInt(math.MaxInt64, 10)
	s.HSet("h", map[string]string{"max": max})
	if _, err := s.HIncrBy("h", "max", 1); err != ErrIntegerOverflow {
		t.Fatalf("HIncrBy overflow = %v", err)
	}
	if v, found, err := s.HGet("h", "max"); err != nil || !found || v != max {
		t.Fatalf("HIncrBy changed overflow field = %q %v %v", v, found, err)
	}
	min := strconv.FormatInt(math.MinInt64, 10)
	s.HSet("h", map[string]string{"min": min})
	if _, err := s.HIncrBy("h", "min", -1); err != ErrIntegerOverflow {
		t.Fatalf("HIncrBy underflow = %v", err)
	}
	if v, found, err := s.HGet("h", "min"); err != nil || !found || v != min {
		t.Fatalf("HIncrBy changed underflow field = %q %v %v", v, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HIncrBy("str", "f", 1); err != ErrWrongType {
		t.Fatalf("HIncrBy wrong type = %v", err)
	}
}

func TestHashIncrByFloat(t *testing.T) {
	s := New()
	v, err := s.HIncrByFloat("h", "score", 1.5)
	if err != nil || v != "1.5" {
		t.Fatalf("HIncrByFloat create = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "score", 2.25)
	if err != nil || v != "3.75" {
		t.Fatalf("HIncrByFloat existing = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "score", -3.75)
	if err != nil || v != "0" {
		t.Fatalf("HIncrByFloat zero normalization = %q %v", v, err)
	}
	v, err = s.HIncrByFloat("h", "whole", 5)
	if err != nil || v != "5" {
		t.Fatalf("HIncrByFloat integer-valued result = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"neg": "-0"})
	v, err = s.HIncrByFloat("h", "neg", math.Copysign(0, -1))
	if err != nil || v != "0" {
		t.Fatalf("HIncrByFloat negative-zero normalization = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"sci": "1e3"})
	v, err = s.HIncrByFloat("h", "sci", 0.5)
	if err != nil || v != "1000.5" {
		t.Fatalf("HIncrByFloat scientific field = %q %v", v, err)
	}
	s.HSet("h", map[string]string{"bad": "abc", "nan": "NaN"})
	if _, err := s.HIncrByFloat("h", "bad", 1); err != ErrHashNotFloat {
		t.Fatalf("HIncrByFloat bad field = %v", err)
	}
	if _, err := s.HIncrByFloat("h", "nan", 1); err != ErrHashNotFloat {
		t.Fatalf("HIncrByFloat NaN field = %v", err)
	}
	if _, err := s.HIncrByFloat("h", "inf-delta", math.Inf(1)); err != ErrNotFloat {
		t.Fatalf("HIncrByFloat infinite delta = %v", err)
	}
	if _, found, err := s.HGet("h", "inf-delta"); err != nil || found {
		t.Fatalf("HIncrByFloat wrote infinite delta field = %v %v", found, err)
	}
	s.HSet("h", map[string]string{"huge": "1e308"})
	if _, err := s.HIncrByFloat("h", "huge", 1e308); err != ErrFloatOverflow {
		t.Fatalf("HIncrByFloat overflow = %v", err)
	}
	if got, found, err := s.HGet("h", "huge"); err != nil || !found || got != "1e308" {
		t.Fatalf("HIncrByFloat changed overflow field = %q %v %v", got, found, err)
	}
	s.Set("str", "value", SetOptions{})
	if _, err := s.HIncrByFloat("str", "f", 1.5); err != ErrWrongType {
		t.Fatalf("HIncrByFloat wrong type = %v", err)
	}
}

func TestWrongType(t *testing.T) {
	s := New()
	s.LPush("l", "a")
	if _, _, err := s.Get("l"); err != ErrWrongType {
		t.Fatalf("want ErrWrongType, got %v", err)
	}
}

func TestExpiryLazy(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }
	s.Set("k", "v", SetOptions{})
	s.Expire("k", time.Second)
	cur = cur.Add(2 * time.Second)
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("key should have lazily expired")
	}
}

func TestSnapshotRoundtrip(t *testing.T) {
	s := New()
	s.Set("str", "hello", SetOptions{})
	s.HSet("h", map[string]string{"f": "v"})
	s.RPush("l", "a", "b")
	s.SAdd("set", "x", "y")
	s.ZAdd("z", []ZMember{{Member: "m", Score: 1.5}})
	s.VSet("vec", "id1", []float32{1, 2, 3}, "meta")

	recs := s.Export()
	s2 := New()
	s2.Import(recs)

	if v, _, _ := s2.Get("str"); v != "hello" {
		t.Fatalf("str = %q", v)
	}
	if v, _, _ := s2.HGet("h", "f"); v != "v" {
		t.Fatalf("hash = %q", v)
	}
	if n, _ := s2.LLen("l"); n != 2 {
		t.Fatalf("list len = %d", n)
	}
	if n, _ := s2.SCard("set"); n != 2 {
		t.Fatalf("set card = %d", n)
	}
	if sc, ok, _ := s2.ZScore("z", "m"); !ok || sc != 1.5 {
		t.Fatalf("zscore = %f %v", sc, ok)
	}
	if n, _ := s2.VCard("vec"); n != 1 {
		t.Fatalf("vcard = %d", n)
	}
}

func TestListSet(t *testing.T) {
	s := New()
	if err := s.LSet("l", 0, "x"); err != ErrNoSuchKey {
		t.Fatalf("LSet missing key = %v", err)
	}
	s.RPush("l", "a", "b", "c")
	if err := s.LSet("l", 1, "B"); err != nil {
		t.Fatalf("LSet = %v", err)
	}
	if v, _, _ := s.LIndex("l", 1); v != "B" {
		t.Fatalf("LSet did not update index 1 = %q", v)
	}
	if err := s.LSet("l", -1, "C"); err != nil {
		t.Fatalf("LSet negative index = %v", err)
	}
	if v, _, _ := s.LIndex("l", 2); v != "C" {
		t.Fatalf("LSet negative index did not update tail = %q", v)
	}
	if err := s.LSet("l", 3, "z"); err != ErrIndexOutOfRange {
		t.Fatalf("LSet out of range = %v", err)
	}
	if err := s.LSet("l", -4, "z"); err != ErrIndexOutOfRange {
		t.Fatalf("LSet negative out of range = %v", err)
	}
	s.Set("str", "value", SetOptions{})
	if err := s.LSet("str", 0, "x"); err != ErrWrongType {
		t.Fatalf("LSet wrong type = %v", err)
	}
}

func TestListRem(t *testing.T) {
	s := New()
	if n, err := s.LRem("nope", 0, "a"); err != nil || n != 0 {
		t.Fatalf("LRem missing key = %d %v", n, err)
	}

	s.RPush("pos", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("pos", 2, "a"); n != 2 {
		t.Fatalf("LRem count>0 removed = %d", n)
	}
	if got, _ := s.LRange("pos", 0, -1); strings.Join(got, ",") != "b,c,a" {
		t.Fatalf("LRem count>0 result = %v", got)
	}

	s.RPush("neg", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("neg", -2, "a"); n != 2 {
		t.Fatalf("LRem count<0 removed = %d", n)
	}
	if got, _ := s.LRange("neg", 0, -1); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("LRem count<0 result = %v", got)
	}

	s.RPush("all", "a", "b", "a", "c", "a")
	if n, _ := s.LRem("all", 0, "a"); n != 3 {
		t.Fatalf("LRem count==0 removed = %d", n)
	}
	if got, _ := s.LRange("all", 0, -1); strings.Join(got, ",") != "b,c" {
		t.Fatalf("LRem count==0 result = %v", got)
	}
	if n, _ := s.LRem("all", 0, "zzz"); n != 0 {
		t.Fatalf("LRem no match = %d", n)
	}

	s.RPush("empty", "x", "x")
	s.LRem("empty", 0, "x")
	if s.Exists("empty") != 0 {
		t.Fatalf("LRem did not drop the emptied key")
	}

	s.Set("str", "v", SetOptions{})
	if _, err := s.LRem("str", 0, "x"); err != ErrWrongType {
		t.Fatalf("LRem wrong type = %v", err)
	}
}
