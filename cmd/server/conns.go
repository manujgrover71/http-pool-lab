package main

import (
	"bytes"
	"encoding/json"
	"net"
	"net/http"
	"strconv"
	"sync/atomic"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/stat"
)

var payload = bytes.Repeat([]byte("x"), 1<<20)

// conns counts at accept, before any TLS handshake, so on :8443 every open is also a full
// handshake unless the client resumes a session.
type conns struct {
	opened, closed, active, requests atomic.Int64
}

func (c *conns) track(_ net.Conn, s http.ConnState) {
	switch s {
	case http.StateNew:
		c.opened.Add(1)
		c.active.Add(1)
	case http.StateClosed, http.StateHijacked:
		c.closed.Add(1)
		c.active.Add(-1)
	}
}

func (c *conns) work(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	if d, err := time.ParseDuration(q.Get("delay")); err == nil {
		time.Sleep(d)
	}
	size, _ := strconv.Atoi(q.Get("size"))
	size = min(max(size, 0), len(payload))
	w.Header().Set("Content-Length", strconv.Itoa(size))
	w.Write(payload[:size])
	c.requests.Add(1)
}

func (c *conns) stats(w http.ResponseWriter, _ *http.Request) {
	cpu, _ := stat.ReadCPU()
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]int64{
		"usage_usec":     int64(cpu.UsageUsec),
		"nr_periods":     int64(cpu.Periods),
		"nr_throttled":   int64(cpu.Throttled),
		"throttled_usec": int64(cpu.ThrottledUsec),
		"opened":         c.opened.Load(),
		"closed":         c.closed.Load(),
		"active":         c.active.Load(),
		"requests":       c.requests.Load(),
		"timewait":       int64(stat.TimeWait()),
	})
}
