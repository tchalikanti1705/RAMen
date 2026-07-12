package server

import (
	"strconv"
	"strings"
)

func (c *conn) cmdLPush(args []string) error { return c.push(args, "lpush", true) }
func (c *conn) cmdRPush(args []string) error { return c.push(args, "rpush", false) }

func (c *conn) push(args []string, name string, left bool) error {
	if len(args) < 3 {
		return c.wrongArgs(name)
	}
	var (
		n   int
		err error
	)
	if left {
		n, err = c.s.store.LPush(args[1], args[2:]...)
	} else {
		n, err = c.s.store.RPush(args[1], args[2:]...)
	}
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdLPop(args []string) error { return c.pop(args, "lpop", true) }
func (c *conn) cmdRPop(args []string) error { return c.pop(args, "rpop", false) }

func (c *conn) pop(args []string, name string, left bool) error {
	if len(args) != 2 {
		return c.wrongArgs(name)
	}
	var (
		v   string
		ok  bool
		err error
	)
	if left {
		v, ok, err = c.s.store.LPop(args[1])
	} else {
		v, ok, err = c.s.store.RPop(args[1])
	}
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

func (c *conn) cmdLLen(args []string) error {
	if len(args) != 2 {
		return c.wrongArgs("llen")
	}
	n, err := c.s.store.LLen(args[1])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdLIndex(args []string) error {
	if len(args) != 3 {
		return c.wrongArgs("lindex")
	}
	idx, err := strconv.Atoi(args[2])
	if err != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	v, ok, err := c.s.store.LIndex(args[1], idx)
	if err != nil {
		return c.storeErr(err)
	}
	if !ok {
		return c.writeNull()
	}
	return c.writeBulk(v)
}

func (c *conn) cmdLRange(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("lrange")
	}
	start, err1 := strconv.Atoi(args[2])
	stop, err2 := strconv.Atoi(args[3])
	if err1 != nil || err2 != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	items, err := c.s.store.LRange(args[1], start, stop)
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeStringArray(items)
}

func (c *conn) cmdLSet(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("lset")
	}
	idx, err := strconv.Atoi(args[2])
	if err != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	if err := c.s.store.LSet(args[1], idx, args[3]); err != nil {
		return c.storeErr(err)
	}
	return c.writeSimple("OK")
}

func (c *conn) cmdLRem(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("lrem")
	}
	count, err := strconv.Atoi(args[2])
	if err != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	n, err := c.s.store.LRem(args[1], count, args[3])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}

func (c *conn) cmdLTrim(args []string) error {
	if len(args) != 4 {
		return c.wrongArgs("ltrim")
	}
	start, err1 := strconv.Atoi(args[2])
	stop, err2 := strconv.Atoi(args[3])
	if err1 != nil || err2 != nil {
		return c.writeError("ERR value is not an integer or out of range")
	}
	if err := c.s.store.LTrim(args[1], start, stop); err != nil {
		return c.storeErr(err)
	}
	return c.writeSimple("OK")
}

func (c *conn) cmdLInsert(args []string) error {
	if len(args) != 5 {
		return c.wrongArgs("linsert")
	}
	var before bool
	switch strings.ToUpper(args[2]) {
	case "BEFORE":
		before = true
	case "AFTER":
		before = false
	default:
		return c.writeError("ERR syntax error")
	}
	n, err := c.s.store.LInsert(args[1], before, args[3], args[4])
	if err != nil {
		return c.storeErr(err)
	}
	return c.writeInt(int64(n))
}
