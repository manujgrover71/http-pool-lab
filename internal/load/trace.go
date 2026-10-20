package load

import (
	"crypto/tls"
	"net/http/httptrace"
	"sync"
	"time"
)

// reqTrace's connect and TLS stamps belong to the dial this request started. The transport
// races that dial against connections other requests return, so the request can end up on a
// returned connection while its dial goes to someone else. reused says which one it got.
type reqTrace struct {
	c  *counters
	mu sync.Mutex

	getConn, gotConn    time.Time
	connStart, connDone time.Time
	tlsStart, tlsDone   time.Time
	wrote, firstByte    time.Time
	reused, wasIdle     bool
}

func (t *reqTrace) stamp(p *time.Time) {
	now := time.Now()
	t.mu.Lock()
	if p.IsZero() {
		*p = now
	}
	t.mu.Unlock()
}

func (t *reqTrace) hooks() *httptrace.ClientTrace {
	return &httptrace.ClientTrace{
		GetConn:              t.onGetConn,
		GotConn:              t.onGotConn,
		ConnectStart:         t.onConnectStart,
		ConnectDone:          t.onConnectDone,
		TLSHandshakeStart:    t.onTLSStart,
		TLSHandshakeDone:     t.onTLSDone,
		WroteRequest:         t.onWroteRequest,
		GotFirstResponseByte: t.onFirstByte,
	}
}

func (t *reqTrace) onGetConn(string) {
	t.stamp(&t.getConn)
}

func (t *reqTrace) onGotConn(i httptrace.GotConnInfo) {
	t.mu.Lock()
	t.gotConn, t.reused, t.wasIdle = time.Now(), i.Reused, i.WasIdle
	t.mu.Unlock()
	if i.Reused {
		t.c.gotReused.Add(1)
	} else {
		t.c.gotNew.Add(1)
	}
}

func (t *reqTrace) onConnectStart(_, _ string) {
	t.stamp(&t.connStart)
}

func (t *reqTrace) onConnectDone(_, _ string, _ error) {
	t.stamp(&t.connDone)
}

func (t *reqTrace) onTLSStart() {
	t.stamp(&t.tlsStart)
}

func (t *reqTrace) onTLSDone(_ tls.ConnectionState, err error) {
	t.stamp(&t.tlsDone)
	if err == nil {
		t.c.handshakes.Add(1)
	}
}

func (t *reqTrace) onWroteRequest(httptrace.WroteRequestInfo) {
	t.stamp(&t.wrote)
}

func (t *reqTrace) onFirstByte() {
	t.stamp(&t.firstByte)
}

type result struct {
	req                      int64
	start                    time.Duration
	wait, connect, tls, ttfb time.Duration
	total                    time.Duration
	reused, wasIdle          bool
	proto, err               string
}

func (t *reqTrace) result(req int64, start, total time.Duration, proto, err string) result {
	t.mu.Lock()
	defer t.mu.Unlock()
	wait := total // never got a connection: all of it was waiting for one
	if !t.gotConn.IsZero() {
		wait = t.gotConn.Sub(t.getConn)
	}
	return result{
		req:     req,
		start:   start,
		wait:    wait,
		connect: span(t.connStart, t.connDone),
		tls:     span(t.tlsStart, t.tlsDone),
		ttfb:    span(t.wrote, t.firstByte),
		total:   total,
		reused:  t.reused,
		wasIdle: t.wasIdle,
		proto:   proto,
		err:     err,
	}
}

func span(a, b time.Time) time.Duration {
	if a.IsZero() || b.IsZero() {
		return 0
	}
	return b.Sub(a)
}
