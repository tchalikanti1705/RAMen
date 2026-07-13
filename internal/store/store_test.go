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

func TestIncrByFloat(t *testing.T) {
	s := New()
	v, err := s.IncrByFloat("c", 1.5)
	if err != nil || v != "1.5" {
		t.Fatalf("IncrByFloat create = %q %v", v, err)
	}
	v, err = s.IncrByFloat("c", 2.25)
	if err != nil || v != "3.75" {
		t.Fatalf("IncrByFloat existing = %q %v", v, err)
	}
	v, err = s.IncrByFloat("c", -3.75)
	if err != nil || v != "0" {
		t.Fatalf("IncrByFloat zero normalization = %q %v", v, err)
	}
	v, err = s.IncrByFloat("whole", 5)
	if err != nil || v != "5" {
		t.Fatalf("IncrByFloat integer-valued result = %q %v", v, err)
	}
	s.Set("neg", "-0", SetOptions{})
	v, err = s.IncrByFloat("neg", math.Copysign(0, -1))
	if err != nil || v != "0" {
		t.Fatalf("IncrByFloat negative-zero normalization = %q %v", v, err)
	}
	s.Set("sci", "1e3", SetOptions{})
	v, err = s.IncrByFloat("sci", 0.5)
	if err != nil || v != "1000.5" {
		t.Fatalf("IncrByFloat scientific value = %q %v", v, err)
	}
	s.Set("bad", "abc", SetOptions{})
	if _, err := s.IncrByFloat("bad", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat non-float value = %v", err)
	}
	if got, _, _ := s.Get("bad"); got != "abc" {
		t.Fatalf("IncrByFloat changed bad value = %q", got)
	}
	s.Set("nan", "NaN", SetOptions{})
	if _, err := s.IncrByFloat("nan", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat NaN value = %v", err)
	}
	if _, err := s.IncrByFloat("d", math.Inf(1)); err != ErrNotFloat {
		t.Fatalf("IncrByFloat infinite delta = %v", err)
	}
	if _, ok, _ := s.Get("d"); ok {
		t.Fatalf("IncrByFloat wrote a key for an infinite delta")
	}
	s.Set("huge", "1e308", SetOptions{})
	if _, err := s.IncrByFloat("huge", 1e308); err != ErrFloatOverflow {
		t.Fatalf("IncrByFloat overflow = %v", err)
	}
	if got, _, _ := s.Get("huge"); got != "1e308" {
		t.Fatalf("IncrByFloat changed overflow value = %q", got)
	}
	s.Set("inf", "inf", SetOptions{})
	if _, err := s.IncrByFloat("inf", 1); err != ErrNotFloat {
		t.Fatalf("IncrByFloat infinite stored value = %v", err)
	}
	s.Set("ttl", "1", SetOptions{})
	s.Expire("ttl", time.Minute)
	if _, err := s.IncrByFloat("ttl", 1.5); err != nil {
		t.Fatalf("IncrByFloat on a key with TTL = %v", err)
	}
	if _, hasTTL, ok := s.TTL("ttl"); !ok || !hasTTL {
		t.Fatalf("IncrByFloat dropped the key's TTL: ok=%v hasTTL=%v", ok, hasTTL)
	}
	s.LPush("l", "a")
	if _, err := s.IncrByFloat("l", 1.5); err != ErrWrongType {
		t.Fatalf("IncrByFloat wrong type = %v", err)
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

func TestExpireAt(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if s.ExpireAt("nope", time.Unix(2000, 0)) {
		t.Fatal("ExpireAt on a missing key should be false")
	}

	s.Set("k", "v", SetOptions{})
	if !s.ExpireAt("k", time.Unix(2000, 0)) {
		t.Fatal("ExpireAt with a future deadline should be true")
	}
	if _, ok, _ := s.Get("k"); !ok {
		t.Fatal("key should still be alive before its deadline")
	}
	cur = time.Unix(2001, 0)
	if _, ok, _ := s.Get("k"); ok {
		t.Fatal("key should be gone once the deadline passes")
	}

	s.Set("past", "v", SetOptions{})
	if !s.ExpireAt("past", time.Unix(500, 0)) {
		t.Fatal("ExpireAt with a past deadline should still return true")
	}
	if s.Exists("past") != 0 {
		t.Fatal("ExpireAt with a past deadline should delete the key")
	}
}

func TestExpireTime(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if _, _, ok := s.ExpireTime("nope"); ok {
		t.Fatal("ExpireTime on a missing key should be ok=false")
	}

	s.Set("k", "v", SetOptions{})
	if _, hasTTL, ok := s.ExpireTime("k"); !ok || hasTTL {
		t.Fatalf("ExpireTime without a TTL = hasTTL=%v ok=%v", hasTTL, ok)
	}

	s.ExpireAt("k", time.Unix(5000, 0))
	if at, hasTTL, ok := s.ExpireTime("k"); !ok || !hasTTL || at.Unix() != 5000 {
		t.Fatalf("ExpireTime = %d hasTTL=%v ok=%v", at.Unix(), hasTTL, ok)
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

	// int64-min count: -count overflows, must still remove all matches like Redis
	s.RPush("min", "a", "b", "a")
	if n, _ := s.LRem("min", math.MinInt64, "a"); n != 2 {
		t.Fatalf("LRem int64-min count removed = %d", n)
	}
	if got, _ := s.LRange("min", 0, -1); strings.Join(got, ",") != "b" {
		t.Fatalf("LRem int64-min count result = %v", got)
	}

	s.Set("str", "v", SetOptions{})
	if _, err := s.LRem("str", 0, "x"); err != ErrWrongType {
		t.Fatalf("LRem wrong type = %v", err)
	}
}

func TestListTrim(t *testing.T) {
	s := New()
	if err := s.LTrim("nope", 0, -1); err != nil {
		t.Fatalf("LTrim missing key = %v", err)
	}

	s.RPush("l", "a", "b", "c", "d", "e")
	if err := s.LTrim("l", 1, 3); err != nil {
		t.Fatalf("LTrim = %v", err)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "b,c,d" {
		t.Fatalf("LTrim result = %v", got)
	}

	s.RPush("neg", "a", "b", "c", "d", "e")
	s.LTrim("neg", -3, -1)
	if got, _ := s.LRange("neg", 0, -1); strings.Join(got, ",") != "c,d,e" {
		t.Fatalf("LTrim negative indices = %v", got)
	}

	s.RPush("clamp", "a", "b", "c")
	s.LTrim("clamp", -100, 100)
	if got, _ := s.LRange("clamp", 0, -1); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("LTrim clamp = %v", got)
	}

	s.RPush("past", "a", "b", "c")
	s.LTrim("past", 5, 10)
	if s.Exists("past") != 0 {
		t.Fatalf("LTrim start past the end did not drop the key")
	}
	s.RPush("rev", "a", "b", "c")
	s.LTrim("rev", 2, 1)
	if s.Exists("rev") != 0 {
		t.Fatalf("LTrim start>stop did not drop the key")
	}

	s.Set("str", "v", SetOptions{})
	if err := s.LTrim("str", 0, -1); err != ErrWrongType {
		t.Fatalf("LTrim wrong type = %v", err)
	}
}

func TestListInsert(t *testing.T) {
	s := New()
	if n, err := s.LInsert("nope", true, "a", "x"); err != nil || n != 0 {
		t.Fatalf("LInsert missing key = %d %v", n, err)
	}

	s.RPush("l", "a", "b", "c")
	if n, _ := s.LInsert("l", true, "b", "X"); n != 4 {
		t.Fatalf("LInsert before length = %d", n)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "a,X,b,c" {
		t.Fatalf("LInsert before result = %v", got)
	}
	if n, _ := s.LInsert("l", false, "b", "Y"); n != 5 {
		t.Fatalf("LInsert after length = %d", n)
	}
	if got, _ := s.LRange("l", 0, -1); strings.Join(got, ",") != "a,X,b,Y,c" {
		t.Fatalf("LInsert after result = %v", got)
	}

	if n, _ := s.LInsert("l", true, "a", "HEAD"); n != 6 {
		t.Fatalf("LInsert before head length = %d", n)
	}
	if got, _ := s.LRange("l", 0, 0); got[0] != "HEAD" {
		t.Fatalf("LInsert before head result = %v", got)
	}
	if n, _ := s.LInsert("l", false, "c", "TAIL"); n != 7 {
		t.Fatalf("LInsert after tail length = %d", n)
	}
	if got, _ := s.LRange("l", -1, -1); got[0] != "TAIL" {
		t.Fatalf("LInsert after tail result = %v", got)
	}

	if n, _ := s.LInsert("l", true, "zzz", "no"); n != -1 {
		t.Fatalf("LInsert missing pivot = %d", n)
	}
	if n, _ := s.LLen("l"); n != 7 {
		t.Fatalf("LInsert missing pivot changed length = %d", n)
	}

	s.RPush("dup", "a", "b", "a")
	s.LInsert("dup", true, "a", "Z")
	if got, _ := s.LRange("dup", 0, -1); strings.Join(got, ",") != "Z,a,b,a" {
		t.Fatalf("LInsert first-occurrence only = %v", got)
	}

	s.Set("str", "v", SetOptions{})
	if _, err := s.LInsert("str", true, "a", "x"); err != ErrWrongType {
		t.Fatalf("LInsert wrong type = %v", err)
	}
}

func TestGetDel(t *testing.T) {
	s := New()

	// missing key: not found, nothing deleted
	if _, ok, err := s.GetDel("nope"); ok || err != nil {
		t.Fatalf("GetDel missing = ok=%v err=%v", ok, err)
	}

	// get-and-delete
	s.Set("k", "v", SetOptions{})
	if v, ok, err := s.GetDel("k"); err != nil || !ok || v != "v" {
		t.Fatalf("GetDel = %q ok=%v err=%v", v, ok, err)
	}
	if s.Exists("k") != 0 {
		t.Fatal("GetDel did not delete the key")
	}

	// a WRONGTYPE key errors and is left in place
	s.push("lst", true, []string{"x"})
	if _, _, err := s.GetDel("lst"); err != ErrWrongType {
		t.Fatalf("GetDel wrong type = %v, want ErrWrongType", err)
	}
	if s.Exists("lst") != 1 {
		t.Fatal("GetDel deleted a WRONGTYPE key")
	}
}

func TestGetEx(t *testing.T) {
	s := New()
	cur := time.Unix(1000, 0)
	s.now = func() time.Time { return cur }

	if _, ok, err := s.GetEx("nope", GetExOp{}); ok || err != nil {
		t.Fatalf("GetEx missing = ok=%v err=%v", ok, err)
	}

	// no change leaves an existing TTL untouched
	s.Set("k", "v", SetOptions{})
	s.Expire("k", 100*time.Second) // deadline at 1100
	if v, ok, err := s.GetEx("k", GetExOp{Mode: GetExNoChange}); err != nil || !ok || v != "v" {
		t.Fatalf("GetEx no-change = %q ok=%v err=%v", v, ok, err)
	}
	if at, hasTTL, _ := s.ExpireTime("k"); !hasTTL || at.Unix() != 1100 {
		t.Fatalf("GetEx no-change altered the TTL: at=%d hasTTL=%v", at.Unix(), hasTTL)
	}

	// relative TTL
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExSetTTL, TTL: 50 * time.Second}); !ok {
		t.Fatal("GetEx set-ttl not ok")
	}
	if at, _, _ := s.ExpireTime("k"); at.Unix() != 1050 {
		t.Fatalf("GetEx set-ttl deadline = %d, want 1050", at.Unix())
	}

	// persist removes the TTL
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExPersist}); !ok {
		t.Fatal("GetEx persist not ok")
	}
	if _, hasTTL, _ := s.ExpireTime("k"); hasTTL {
		t.Fatal("GetEx persist left a TTL")
	}

	// absolute deadline in the future
	if _, ok, _ := s.GetEx("k", GetExOp{Mode: GetExSetAt, At: time.Unix(2000, 0)}); !ok {
		t.Fatal("GetEx set-at not ok")
	}
	if at, _, _ := s.ExpireTime("k"); at.Unix() != 2000 {
		t.Fatalf("GetEx set-at deadline = %d, want 2000", at.Unix())
	}

	// absolute deadline in the past: returns the value but deletes the key
	if v, ok, err := s.GetEx("k", GetExOp{Mode: GetExSetAt, At: time.Unix(500, 0)}); err != nil || !ok || v != "v" {
		t.Fatalf("GetEx past-at = %q ok=%v err=%v", v, ok, err)
	}
	if s.Exists("k") != 0 {
		t.Fatal("GetEx with a past deadline did not delete the key")
	}

	// a WRONGTYPE key errors and keeps its TTL untouched
	s.push("lst", true, []string{"x"})
	if _, _, err := s.GetEx("lst", GetExOp{Mode: GetExPersist}); err != ErrWrongType {
		t.Fatalf("GetEx wrong type = %v, want ErrWrongType", err)
	}
}
