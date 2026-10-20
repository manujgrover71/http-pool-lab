package load

import (
	"fmt"
)

// Transport is one http.Transport setup to compare. Apart from Name, the zero value is a
// clone of http.DefaultTransport: 2 idle connections per host and no cap on open ones.
type Transport struct {
	Name        string
	IdlePerHost int // MaxIdleConnsPerHost; 0 keeps the default of 2
	MaxPerHost  int // MaxConnsPerHost
	NoKeepAlive bool
	PerRequest  bool // a new Transport for every request
	Bare        bool // &http.Transport{}, which has no ForceAttemptHTTP2
	Resume      bool // a TLS session cache; Go's client has none by default
	HTTP1       bool // never negotiate HTTP/2, like most internal service calls
}

var (
	Default     = Transport{Name: "default"}
	NoKeepAlive = Transport{
		Name:        "no-keepalive",
		NoKeepAlive: true,
	}
	PerRequest = Transport{
		Name:       "per-request",
		PerRequest: true,
	}
	Bare = Transport{
		Name: "bare",
		Bare: true,
	}
)

func Idle(n int) Transport {
	return Transport{
		Name:        fmt.Sprintf("idle-%d", n),
		IdlePerHost: n,
	}
}

func MaxConns(n int) Transport {
	return Transport{
		Name:       fmt.Sprintf("max-conns-%d", n),
		MaxPerHost: n,
	}
}
