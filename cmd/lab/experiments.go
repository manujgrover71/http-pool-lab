package main

import (
	"context"
	"fmt"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/load"
)

// By Little's law the caller needs rate × delay connections in flight: 100/s is 2, 1,600/s is 32.
const delay = 20 * time.Millisecond

type experiment struct {
	name, about string
	run         func(context.Context, *lab) error
}

var experiments = []experiment{
	{
		name:  "reuse-sweep",
		about: "arrival rate against connection reuse: default vs 128 idle per host",
		run:   reuseSweep,
	},
	{
		name:  "open-cost",
		about: "the same requests on warm vs always-new connections, plain and TLS",
		run:   openCost,
	},
	{
		name:  "time-wait",
		about: "which side collects TIME_WAIT, and how fast it grows",
		run:   timeWait,
	},
	{
		name:  "drain",
		about: "a 64 KB body read to the end vs closed unread",
		run:   drain,
	},
	{
		name:  "h2",
		about: "over TLS: a DefaultTransport clone (HTTP/2) vs a bare &http.Transport{} (HTTP/1.1)",
		run:   h2,
	},
	{
		name:  "max-conns",
		about: "MaxConnsPerHost: the one setup where requests queue for a connection",
		run:   maxConns,
	},
	{
		name:  "slowdown",
		about: "a downstream slowing from 20 to 100 ms at a steady rate: default vs 1024 idle",
		run:   slowdown,
	},
	{
		name:  "incident",
		about: "a CPU-limited caller over TLS, through a burst: churn vs reuse vs resumption",
		run:   incident,
	},
}

func phase(rate float64, d time.Duration) load.Phase {
	return load.Phase{
		Rate: rate,
		For:  d,
	}
}

func steady(rate float64, d time.Duration) []load.Phase {
	return []load.Phase{phase(rate, d)}
}

func reuseSweep(ctx context.Context, l *lab) error {
	for _, t := range []load.Transport{load.Default, load.Idle(128)} {
		for _, rate := range []float64{50, 100, 200, 400, 800, 1600, 3200} {
			name := fmt.Sprintf("%s-%d", t.Name, int(rate))
			err := l.variant(ctx, "reuse-sweep", name, load.Config{
				Target:    l.plain,
				Transport: t,
				Body:      load.Drain,
				Delay:     delay,
				Plan:      steady(rate, 20*time.Second),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func openCost(ctx context.Context, l *lab) error {
	targets := map[string]string{
		"plain": l.plain,
		"tls":   l.secure,
	}
	for _, target := range []string{"plain", "tls"} {
		for _, t := range []load.Transport{load.Idle(128), load.NoKeepAlive} {
			err := l.variant(ctx, "open-cost", t.Name+"-"+target, load.Config{
				Target:    targets[target],
				Transport: t,
				Body:      load.Drain,
				Delay:     5 * time.Millisecond,
				Plan:      steady(500, 30*time.Second),
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

// Whoever closes a TCP connection first holds its TIME_WAIT: the client when its pool has no
// room to keep it, the server when the client sent Connection: close.
func timeWait(ctx context.Context, l *lab) error {
	ramp := []load.Phase{
		phase(500, 20*time.Second),
		phase(1000, 20*time.Second),
		phase(2000, 20*time.Second),
	}
	for _, t := range []load.Transport{load.Default, load.NoKeepAlive} {
		err := l.variant(ctx, "time-wait", t.Name, load.Config{
			Target:    l.plain,
			Transport: t,
			Body:      load.Drain,
			Delay:     50 * time.Millisecond,
			Timeout:   5 * time.Second,
			Plan:      ramp,
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func drain(ctx context.Context, l *lab) error {
	for _, b := range []load.Body{load.Drain, load.CloseOnly} {
		err := l.variant(ctx, "drain", string(b), load.Config{
			Target:    l.plain,
			Transport: load.Idle(128),
			Body:      b,
			Delay:     delay,
			Size:      64 << 10,
			Plan:      steady(400, 20*time.Second),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func h2(ctx context.Context, l *lab) error {
	for _, t := range []load.Transport{load.Default, load.Bare} {
		err := l.variant(ctx, "h2", t.Name, load.Config{
			Target:    l.secure,
			Transport: t,
			Body:      load.Drain,
			Delay:     delay,
			Plan:      steady(800, 20*time.Second),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// 800/s × 20 ms needs 16 connections. A cap of 8 carries 400/s and the rest queue until the
// deadline; 32 has room.
func maxConns(ctx context.Context, l *lab) error {
	for _, n := range []int{8, 32} {
		t := load.MaxConns(n)
		t.IdlePerHost = n // so the cap is measured, not idle churn
		err := l.variant(ctx, "max-conns", t.Name, load.Config{
			Target:    l.plain,
			Transport: t,
			Body:      load.Drain,
			Delay:     delay,
			Timeout:   2 * time.Second,
			Plan:      steady(800, 20*time.Second),
		})
		if err != nil {
			return err
		}
	}
	return nil
}

// The shape of the real incident: a CPU-limited caller with a deadline, calling one downstream
// over HTTP/1.1 through a burst. Over TLS a DefaultTransport would negotiate HTTP/2 and carry
// everything on one connection, which hides the problem (see h2). Tune -rate until the default
// variants throttle.
func incident(ctx context.Context, l *lab) error {
	burst := []load.Phase{
		phase(l.rate/2, 15*time.Second),
		phase(l.rate, 30*time.Second),
		phase(l.rate/2, 15*time.Second),
	}
	targets := map[string]string{
		"plain": l.plain,
		"tls":   l.secure,
	}
	for _, target := range []string{"plain", "tls"} {
		for _, t := range incidentTransports() {
			if t.Resume && target == "plain" {
				continue
			}
			err := l.variant(ctx, "incident", t.Name+"-"+target, load.Config{
				Target:    targets[target],
				Transport: t,
				Body:      load.Drain,
				Delay:     delay,
				Timeout:   100 * time.Millisecond,
				Plan:      burst,
			})
			if err != nil {
				return err
			}
		}
	}
	return nil
}

func incidentTransports() []load.Transport {
	resume := load.Default
	resume.Name, resume.Resume = "default-resume", true
	idle := load.Idle(512)
	ts := []load.Transport{load.Default, resume, idle}
	for i := range ts {
		ts[i].HTTP1 = true
	}
	return ts
}

// The shape of a downstream that slows down, like a third-party API: the rate stays put and the
// latency goes from 20 ms to 100 ms for 30 seconds, so by Little's law in-flight requests go from
// about 20 to about 100. HTTPS on HTTP/1.1, against 1024 idle per host, which is what
// z/go/http's pooled transport sets.
func slowdown(ctx context.Context, l *lab) error {
	plan := []load.Phase{
		phase(1000, 15*time.Second),
		{
			Rate:  1000,
			For:   30 * time.Second,
			Delay: 100 * time.Millisecond,
		},
		phase(1000, 15*time.Second),
	}
	ts := []load.Transport{load.Default, load.Idle(1024)}
	for _, t := range ts {
		t.HTTP1 = true
		err := l.variant(ctx, "slowdown", t.Name, load.Config{
			Target:    l.secure,
			Transport: t,
			Body:      load.Drain,
			Delay:     delay,
			Timeout:   2 * time.Second,
			Plan:      plan,
		})
		if err != nil {
			return err
		}
	}
	return nil
}
