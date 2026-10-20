package load

import (
	"crypto/tls"
	"encoding/json"
	"time"
)

type Phase struct {
	Rate  float64
	For   time.Duration
	Delay time.Duration // server-side time per request in this phase; 0 keeps Config.Delay
}

type Body string

const (
	Drain     Body = "drain" // read to EOF, then Close: what lets the connection be reused
	CloseOnly Body = "close"
)

type Config struct {
	Target    string
	TLS       *tls.Config
	Transport Transport
	Body      Body

	Delay time.Duration // server-side time per request
	Size  int

	Timeout time.Duration // per request, connection included; 0 is none
	Plan    []Phase

	Stats       string        // empty skips server.csv
	SampleEvery time.Duration // cpu.csv resolution; 0 is 10 ms
}

func (c Config) MarshalJSON() ([]byte, error) {
	plan := make([]map[string]any, len(c.Plan))
	for i, p := range c.Plan {
		plan[i] = map[string]any{
			"rate":  p.Rate,
			"for":   p.For.String(),
			"delay": p.Delay.String(),
		}
	}
	return json.Marshal(map[string]any{
		"target":    c.Target,
		"transport": c.Transport,
		"body":      c.Body,
		"delay":     c.Delay.String(),
		"size":      c.Size,
		"timeout":   c.Timeout.String(),
		"plan":      plan,
	})
}
