package dashboard

import (
	"context"
	"io"
	"net"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/client"
	"github.com/Rohit-Dnath/RAMen/internal/server"
	"github.com/Rohit-Dnath/RAMen/internal/store"
)

func TestMetricsEndpoint(t *testing.T) {
	// Boot a real server on an ephemeral port so a real command moves the
	// counters, then scrape /metrics through the handler directly.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	srv := server.New(store.New(), server.Config{Addr: addr})
	ctx, cancel := context.WithCancel(context.Background())
	go srv.ListenAndServe(ctx)
	defer cancel()

	var cli *client.Client
	for i := 0; i < 50; i++ {
		if cli, err = client.Dial(addr); err == nil {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer cli.Close()

	// a real command so ramen_commands_processed_total and ramen_keys leave zero
	if _, err := cli.Do("SET", "k", "v"); err != nil {
		t.Fatalf("SET: %v", err)
	}
	// give the cache ratio a fractional value so the assertion below can pin the
	// %g float formatting (a regression to %f would print 0.666667, not the full
	// value), and so the cache counters leave zero too
	srv.Stats().CacheHits.Add(2)
	srv.Stats().CacheMisses.Add(1)

	d := New(srv, ":0") // build the dashboard without starting its HTTP server
	rec := httptest.NewRecorder()
	// route through the dashboard mux (not handleMetrics directly) so the test
	// also exercises the /metrics registration and that the "/" catch-all does
	// not shadow it
	d.http.Handler.ServeHTTP(rec, httptest.NewRequest("GET", "/metrics", nil))
	res := rec.Result()

	if ct := res.Header.Get("Content-Type"); ct != "text/plain; version=0.0.4; charset=utf-8" {
		t.Fatalf("Content-Type = %q", ct)
	}
	body, _ := io.ReadAll(res.Body)
	text := string(body)

	// every metric must be present with its HELP and TYPE lines
	for _, m := range []struct{ name, typ string }{
		{"ramen_uptime_seconds", "gauge"},
		{"ramen_connected_clients", "gauge"},
		{"ramen_connections_total", "counter"},
		{"ramen_commands_processed_total", "counter"},
		{"ramen_cache_hits_total", "counter"},
		{"ramen_cache_misses_total", "counter"},
		{"ramen_cache_hit_ratio", "gauge"},
		{"ramen_keys", "gauge"},
		{"ramen_memory_alloc_bytes", "gauge"},
	} {
		if !strings.Contains(text, "# HELP "+m.name+" ") {
			t.Errorf("missing HELP line for %s", m.name)
		}
		if !strings.Contains(text, "# TYPE "+m.name+" "+m.typ+"\n") {
			t.Errorf("missing or wrong TYPE line for %s", m.name)
		}
	}

	// the body must end with a trailing newline
	if !strings.HasSuffix(text, "\n") {
		t.Error("body should end with a trailing newline")
	}

	// the counters must have actually moved, not be a permanently-zero body
	if got := sampleValue(t, text, "ramen_commands_processed_total"); got < 1 {
		t.Errorf("ramen_commands_processed_total = %d after a command, want >= 1", got)
	}
	if got := sampleValue(t, text, "ramen_keys"); got != 1 {
		t.Errorf("ramen_keys = %d after one SET, want 1", got)
	}
	if got := sampleValue(t, text, "ramen_connections_total"); got < 1 {
		t.Errorf("ramen_connections_total = %d, want >= 1", got)
	}
	if got := sampleValue(t, text, "ramen_cache_hits_total"); got != 2 {
		t.Errorf("ramen_cache_hits_total = %d, want 2", got)
	}
	if got := sampleValue(t, text, "ramen_cache_misses_total"); got != 1 {
		t.Errorf("ramen_cache_misses_total = %d, want 1", got)
	}

	// the ratio must be %g-formatted (full precision), never %f: 2/3 must render
	// as the full float, not a truncated 0.666667
	wantRatio := strconv.FormatFloat(2.0/3.0, 'g', -1, 64)
	if !strings.Contains(text, "ramen_cache_hit_ratio "+wantRatio+"\n") {
		t.Errorf("ramen_cache_hit_ratio not %%g-formatted; want line %q in body:\n%s", "ramen_cache_hit_ratio "+wantRatio, text)
	}
}

// sampleValue returns the integer value on a metric's sample line (the line that
// is "<name> <value>", not a "# HELP"/"# TYPE" comment).
func sampleValue(t *testing.T, text, name string) int64 {
	t.Helper()
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, name+" ") {
			v, err := strconv.ParseInt(strings.TrimPrefix(line, name+" "), 10, 64)
			if err != nil {
				t.Fatalf("bad sample for %s: %q", name, line)
			}
			return v
		}
	}
	t.Fatalf("no sample line for %s", name)
	return 0
}

func TestEscapeHelp(t *testing.T) {
	cases := map[string]string{
		"plain ASCII help.":   "plain ASCII help.",
		"has\\backslash":      `has\\backslash`,
		"line one\ntwo":       `line one\ntwo`,
		"both \\ and \n here": `both \\ and \n here`,
	}
	for in, want := range cases {
		if got := escapeHelp(in); got != want {
			t.Fatalf("escapeHelp(%q) = %q, want %q", in, got, want)
		}
	}
}
