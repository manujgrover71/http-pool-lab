package load

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptrace"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/csvw"
)

// Run writes ops.csv, conns.csv, cpu.csv and server.csv into dir.
func Run(ctx context.Context, cfg Config, dir string) error {
	ops, err := csvw.Create(filepath.Join(dir, "ops.csv"), opsColumns...)
	if err != nil {
		return err
	}
	connsCSV, err := csvw.Create(filepath.Join(dir, "conns.csv"), connColumns...)
	if err != nil {
		return err
	}

	var c counters
	cl := newClient(cfg.Transport, cfg.TLS, &c)
	defer cl.close()

	rows := make(chan result, 4096)
	wrote := make(chan error, 1)
	go func() {
		wrote <- writeOps(ops, rows)
	}()

	start := time.Now()
	stop := make(chan struct{})
	every := cfg.SampleEvery
	if every == 0 {
		every = 10 * time.Millisecond
	}
	bg := []chan error{make(chan error, 1), make(chan error, 1), make(chan error, 1)}
	go func() {
		bg[0] <- tick(connsCSV, &c, start, stop)
	}()
	go func() {
		bg[1] <- sampleCPU(dir, start, every, stop)
	}()
	go func() {
		bg[2] <- sampleServer(cfg.Stats, dir, start, stop)
	}()

	var wg sync.WaitGroup
	var req int64
	next := start
	for _, ph := range cfg.Plan {
		delay := cfg.Delay
		if ph.Delay > 0 {
			delay = ph.Delay
		}
		url := fmt.Sprintf("%s/work?delay=%s&size=%d", cfg.Target, delay, cfg.Size)
		gap := time.Duration(float64(time.Second) / ph.Rate)
		end := next.Add(ph.For)
		for ; next.Before(end) && ctx.Err() == nil; next = next.Add(gap) {
			time.Sleep(time.Until(next))
			req++
			wg.Add(1)
			go func(id int64) {
				defer wg.Done()
				send(ctx, cfg, cl, url, id, start, rows, &c)
			}(req)
		}
	}
	wg.Wait()

	close(rows)
	close(stop)
	errs := []error{<-wrote}
	for _, ch := range bg {
		errs = append(errs, <-ch)
	}
	return errors.Join(errs...)
}

func send(
	ctx context.Context, cfg Config, cl *client, url string, id int64, t0 time.Time,
	rows chan<- result, c *counters,
) {
	c.inflight.Add(1)
	defer c.inflight.Add(-1)

	rctx := ctx
	if cfg.Timeout > 0 {
		var cancel context.CancelFunc
		rctx, cancel = context.WithTimeout(ctx, cfg.Timeout)
		defer cancel()
	}
	tr := &reqTrace{c: c}
	req, err := http.NewRequestWithContext(
		httptrace.WithClientTrace(rctx, tr.hooks()), http.MethodGet, url, nil,
	)
	if err != nil {
		panic(err)
	}

	began := time.Now()
	proto, err := cl.do(req, cfg.Body)
	total := time.Since(began)
	if err == nil {
		c.done.Add(1)
	} else {
		c.failed.Add(1)
	}
	rows <- tr.result(id, began.Sub(t0), total, proto, class(err))
}

func writeOps(ops *csvw.File, rows <-chan result) error {
	for r := range rows {
		ops.Write(
			r.req, ms(r.start), us(r.wait), us(r.connect), us(r.tls), us(r.ttfb), us(r.total),
			r.reused, r.wasIdle, r.proto, r.err,
		)
	}
	return ops.Close()
}

func class(err error) string {
	switch {
	case err == nil:
		return ""
	case errors.Is(err, context.DeadlineExceeded):
		return "timeout"
	case errors.Is(err, syscall.EADDRNOTAVAIL):
		return "no_ports" // ephemeral ports used up, usually by TIME_WAIT
	case errors.Is(err, syscall.ECONNREFUSED):
		return "refused"
	case errors.Is(err, syscall.ECONNRESET):
		return "reset"
	case errors.Is(err, syscall.EMFILE):
		return "no_fds"
	default:
		return "other"
	}
}

var opsColumns = []string{
	"req", "start_ms", "wait_us", "connect_us", "tls_us", "ttfb_us", "total_us", "reused",
	"was_idle", "proto", "err",
}

func ms(d time.Duration) int64 {
	return d.Milliseconds()
}

func us(d time.Duration) int64 {
	return d.Microseconds()
}
