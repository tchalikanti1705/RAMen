// Package dashboard serves a small read-only web UI: a live key browser plus
// hit/miss ratio and memory usage (PRD §7). It embeds a single static page and
// exposes a couple of JSON endpoints the page polls. It deliberately has no
// build step and no external assets.
package dashboard

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/Rohit-Dnath/RAMen/internal/server"
)

//go:embed static/index.html
var indexHTML []byte

// Dashboard wraps an HTTP server bound to a RAMen server's state.
type Dashboard struct {
	srv  *server.Server
	http *http.Server
}

// New builds a dashboard for srv listening on addr (e.g. ":8080").
func New(srv *server.Server, addr string) *Dashboard {
	d := &Dashboard{srv: srv}
	mux := http.NewServeMux()
	// Serve the single embedded page at the root. The page has no external
	// assets, so there is nothing else static to serve.
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write(indexHTML)
	})
	mux.HandleFunc("/api/stats", d.handleStats)
	mux.HandleFunc("/api/keys", d.handleKeys)
	mux.HandleFunc("/metrics", d.handleMetrics)
	d.http = &http.Server{Addr: addr, Handler: mux}
	return d
}

// ListenAndServe runs the dashboard until ctx is cancelled.
func (d *Dashboard) ListenAndServe(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		shutCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		d.http.Shutdown(shutCtx)
	}()
	if err := d.http.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

// metrics is a point-in-time snapshot of the server's counters and gauges. Both
// /api/stats and /metrics read from gather so a counter added here shows up in
// both places at once, and neither handler reaches into the server on its own.
type metrics struct {
	uptimeSeconds    int
	connectedClients int64
	totalConns       int64
	commands         int64
	cacheHits        int64
	cacheMisses      int64
	cacheHitRatio    float64
	keys             int
	memAllocBytes    uint64
}

func (d *Dashboard) gather() metrics {
	st := d.srv.Stats()
	var mem runtime.MemStats
	runtime.ReadMemStats(&mem)
	return metrics{
		uptimeSeconds:    int(d.srv.Uptime().Seconds()),
		connectedClients: st.Connections.Load(),
		totalConns:       st.TotalConns.Load(),
		commands:         st.Commands.Load(),
		cacheHits:        st.CacheHits.Load(),
		cacheMisses:      st.CacheMisses.Load(),
		cacheHitRatio:    st.HitRatio(),
		keys:             d.srv.Store().DBSize(),
		memAllocBytes:    mem.Alloc,
	}
}

func (d *Dashboard) handleStats(w http.ResponseWriter, r *http.Request) {
	m := d.gather()
	writeJSON(w, map[string]any{
		"keys":              m.keys,
		"connected_clients": m.connectedClients,
		"total_connections": m.totalConns,
		"commands":          m.commands,
		"cache_hits":        m.cacheHits,
		"cache_misses":      m.cacheMisses,
		"cache_hit_ratio":   m.cacheHitRatio,
		"memory_bytes":      m.memAllocBytes,
		"uptime_seconds":    m.uptimeSeconds,
	})
}

// handleMetrics exposes the same counters as /api/stats in the Prometheus text
// exposition format (version 0.0.4) so Prometheus/Grafana can scrape them. It is
// counters-only (no key names or values) and unauthenticated, like the rest of
// the dashboard, and is only reachable when the dashboard is enabled.
func (d *Dashboard) handleMetrics(w http.ResponseWriter, r *http.Request) {
	m := d.gather()
	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	metric(w, "ramen_uptime_seconds", "Seconds since the server started.", "gauge", strconv.Itoa(m.uptimeSeconds))
	metric(w, "ramen_connected_clients", "Client connections currently open.", "gauge", strconv.FormatInt(m.connectedClients, 10))
	metric(w, "ramen_connections_total", "Client connections accepted since start.", "counter", strconv.FormatInt(m.totalConns, 10))
	metric(w, "ramen_commands_processed_total", "Commands processed since start.", "counter", strconv.FormatInt(m.commands, 10))
	metric(w, "ramen_cache_hits_total", "Semantic cache hits since start.", "counter", strconv.FormatInt(m.cacheHits, 10))
	metric(w, "ramen_cache_misses_total", "Semantic cache misses since start.", "counter", strconv.FormatInt(m.cacheMisses, 10))
	metric(w, "ramen_cache_hit_ratio", "Semantic cache hit ratio in [0,1] over the process lifetime.", "gauge", strconv.FormatFloat(m.cacheHitRatio, 'g', -1, 64))
	metric(w, "ramen_keys", "Keys currently in the keyspace.", "gauge", strconv.Itoa(m.keys))
	metric(w, "ramen_memory_alloc_bytes", "Heap memory currently allocated, in bytes.", "gauge", strconv.FormatUint(m.memAllocBytes, 10))
}

// metric writes one Prometheus metric: its HELP line, TYPE line, and a single
// sample, each terminated by a newline (so the body ends with one, which some
// parsers require). value is pre-formatted so callers pick the right encoding
// per type (%g for the ratio, plain integers otherwise).
func metric(w io.Writer, name, help, typ, value string) {
	fmt.Fprintf(w, "# HELP %s %s\n# TYPE %s %s\n%s %s\n", name, escapeHelp(help), name, typ, name, value)
}

// escapeHelp escapes a HELP string per the Prometheus text exposition format,
// where backslash and newline are the only characters that must be escaped.
// Today's help strings are plain ASCII, but this keeps the helper from emitting
// a malformed or split line if one ever contains a backslash or newline.
func escapeHelp(s string) string {
	if !strings.ContainsAny(s, "\\\n") {
		return s
	}
	return strings.NewReplacer("\\", `\\`, "\n", `\n`).Replace(s)
}

func (d *Dashboard) handleKeys(w http.ResponseWriter, r *http.Request) {
	pattern := r.URL.Query().Get("match")
	if pattern == "" {
		pattern = "*"
	}
	keys := d.srv.Store().Keys(pattern)
	const limit = 500 // protect the browser from huge keyspaces
	truncated := false
	if len(keys) > limit {
		keys = keys[:limit]
		truncated = true
	}
	type keyInfo struct {
		Key  string `json:"key"`
		Type string `json:"type"`
	}
	infos := make([]keyInfo, 0, len(keys))
	for _, k := range keys {
		infos = append(infos, keyInfo{Key: k, Type: d.srv.Store().Type(k)})
	}
	writeJSON(w, map[string]any{"keys": infos, "truncated": truncated})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
