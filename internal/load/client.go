package load

import (
	"context"
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync/atomic"
	"time"
)

type client struct {
	t      Transport
	tls    *tls.Config
	c      *counters
	shared *http.Client
}

func newClient(t Transport, tlsConf *tls.Config, c *counters) *client {
	cl := &client{
		t:   t,
		tls: tlsConf,
		c:   c,
	}
	if !t.PerRequest {
		cl.shared = &http.Client{Transport: cl.build()}
	}
	return cl
}

func (cl *client) build() *http.Transport {
	t := cl.t
	var tr *http.Transport
	if t.Bare {
		tr = &http.Transport{}
	} else {
		tr = http.DefaultTransport.(*http.Transport).Clone()
	}

	// The dialer wraps every connection so closes the transport does on its own get counted.
	// On a bare Transport, a custom dialer or TLS config also turns off automatic HTTP/2,
	// the same as for any real custom transport.
	tr.DialContext = cl.dial
	if cl.tls != nil {
		conf := cl.tls.Clone()
		if t.Resume {
			conf.ClientSessionCache = tls.NewLRUClientSessionCache(0)
		}
		tr.TLSClientConfig = conf
	}
	if t.IdlePerHost > 0 {
		tr.MaxIdleConnsPerHost = t.IdlePerHost
		if tr.MaxIdleConns != 0 { // DefaultTransport's 100 across all hosts would cap it first
			tr.MaxIdleConns = max(tr.MaxIdleConns, t.IdlePerHost)
		}
	}
	tr.MaxConnsPerHost = t.MaxPerHost
	tr.DisableKeepAlives = t.NoKeepAlive
	if t.HTTP1 {
		// A non-nil, empty TLSNextProto is how net/http documents turning HTTP/2 off.
		tr.ForceAttemptHTTP2 = false
		tr.TLSNextProto = map[string]func(string, *tls.Conn) http.RoundTripper{}
	}
	return tr
}

// do returns the protocol the response came back on.
func (cl *client) do(req *http.Request, body Body) (string, error) {
	hc := cl.shared
	if hc == nil {
		tr := cl.build()
		defer tr.CloseIdleConnections()
		hc = &http.Client{Transport: tr}
	}
	resp, err := hc.Do(req)
	if err != nil {
		return "", err
	}
	if body == Drain {
		_, err = io.Copy(io.Discard, resp.Body)
	}
	resp.Body.Close()
	if err == nil && resp.StatusCode != http.StatusOK {
		err = fmt.Errorf("status %d", resp.StatusCode)
	}
	return resp.Proto, err
}

func (cl *client) close() {
	if cl.shared != nil {
		cl.shared.CloseIdleConnections()
	}
}

// dialer has DefaultTransport's own dialer settings.
var dialer = &net.Dialer{
	Timeout:   30 * time.Second,
	KeepAlive: 30 * time.Second,
}

func (cl *client) dial(ctx context.Context, network, addr string) (net.Conn, error) {
	conn, err := dialer.DialContext(ctx, network, addr)
	if err != nil {
		cl.c.dialErr.Add(1)
		return nil, err
	}
	cl.c.opened.Add(1)
	return &countedConn{
		Conn: conn,
		c:    cl.c,
	}, nil
}

type countedConn struct {
	net.Conn
	c      *counters
	closed atomic.Bool
}

func (cc *countedConn) Close() error {
	if cc.closed.CompareAndSwap(false, true) {
		cc.c.closed.Add(1)
	}
	return cc.Conn.Close()
}
