package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/manujgrover71/http-pool-lab/internal/load"
	"github.com/manujgrover71/http-pool-lab/internal/stat"
)

func main() {
	l := &lab{}
	flag.StringVar(&l.out, "out", "results", "where results go")
	flag.StringVar(&l.plain, "plain", "http://server:8080", "the server's plain HTTP address")
	flag.StringVar(&l.secure, "tls", "https://server:8443", "the server's TLS address")
	flag.StringVar(
		&l.stats, "stats", "http://server:9090/stats", "the server's stats endpoint; empty to skip",
	)
	ca := flag.String("ca", "/tls/ca.pem", "CA from tls/gen.sh")
	flag.Float64Var(&l.rate, "rate", 1500, "peak requests a second for incident")
	flag.IntVar(&l.round, "round", 1, "repeat number; rounds after the first write <variant>-r<n>")
	flag.StringVar(&l.tag, "tag", "", "appended to every variant's name, e.g. for a sysctl change")
	only := flag.String("only", "", "comma-separated variants to run; empty runs them all")
	flag.Usage = usage
	flag.Parse()

	if *only != "" {
		l.only = map[string]bool{}
		for _, v := range strings.Split(*only, ",") {
			l.only[v] = true
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := run(ctx, flag.Arg(0), l, *ca); err != nil {
		fmt.Fprintln(os.Stderr, "lab:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, name string, l *lab, ca string) error {
	if name == "summary" {
		return summarize(l.out)
	}
	for _, e := range experiments {
		if e.name != name {
			continue
		}
		pem, err := os.ReadFile(ca)
		if err != nil {
			return fmt.Errorf("%w (run tls/gen.sh first)", err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(pem) {
			return fmt.Errorf("%s: no certificate in it", ca)
		}
		l.tls = &tls.Config{RootCAs: pool}
		return e.run(ctx, l)
	}
	usage()
	return fmt.Errorf("unknown experiment %q", name)
}

type lab struct {
	out, plain, secure, stats string
	tls                       *tls.Config
	rate                      float64
	round                     int
	tag                       string
	only                      map[string]bool
}

func (l *lab) variant(ctx context.Context, exp, name string, cfg load.Config) error {
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if l.only != nil && !l.only[name] {
		return nil
	}
	if l.tag != "" {
		name += "-" + l.tag
	}
	if l.round > 1 {
		name = fmt.Sprintf("%s-r%d", name, l.round)
	}
	dir := filepath.Join(l.out, exp, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	if strings.HasPrefix(cfg.Target, "https://") {
		cfg.TLS = l.tls
	}
	cfg.Stats = l.stats
	settle(ctx, l.stats)

	meta, err := json.MarshalIndent(map[string]any{
		"experiment": exp,
		"variant":    name,
		"started":    time.Now().UTC().Format(time.RFC3339Nano),
		"config":     cfg,
		"gomaxprocs": runtime.GOMAXPROCS(0),
		"numcpu":     runtime.NumCPU(),
		"cpu_max":    stat.CPUMax(),
		"sysctls":    stat.Sysctls("net.ipv4.ip_local_port_range", "net.ipv4.tcp_tw_reuse"),
		"go":         runtime.Version(),
	}, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), meta, 0o644); err != nil {
		return err
	}
	fmt.Printf("%s/%s\n", exp, name)
	return load.Run(ctx, cfg, dir)
}

// settle lets the previous variant's TIME_WAIT sockets expire (60 s on Linux) on both sides,
// so every run starts with the same ephemeral ports free and a clean server count.
func settle(ctx context.Context, statsURL string) {
	deadline := time.Now().Add(75 * time.Second)
	for time.Now().Before(deadline) && ctx.Err() == nil {
		caller, server := stat.TimeWait(), serverTimeWait(statsURL)
		if caller <= 100 && server <= 100 {
			return
		}
		fmt.Printf("  waiting for TIME_WAIT to expire: %d caller, %d server\n", caller, server)
		time.Sleep(5 * time.Second)
	}
}

// serverTimeWait returns 0 when the server can't be asked, so settle doesn't wait on it.
func serverTimeWait(statsURL string) int64 {
	if statsURL == "" {
		return 0
	}
	resp, err := http.Get(statsURL)
	if err != nil {
		return 0
	}
	defer resp.Body.Close()
	var m map[string]int64
	if json.NewDecoder(resp.Body).Decode(&m) != nil {
		return 0
	}
	return m["timewait"]
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: lab [flags] summary | <experiment>")
	fmt.Fprintln(os.Stderr, "\nexperiments:")
	for _, e := range experiments {
		fmt.Fprintf(os.Stderr, "  %-12s %s\n", e.name, e.about)
	}
	fmt.Fprintln(os.Stderr, "\nflags:")
	flag.PrintDefaults()
}
