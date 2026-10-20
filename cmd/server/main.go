package main

import (
	"flag"
	"log"
	"net/http"
)

func main() {
	cert := flag.String("cert", "/tls/server.crt", "certificate from tls/gen.sh")
	key := flag.String("key", "/tls/server.key", "its key")
	idle := flag.Duration("idle", 0, "IdleTimeout; 0 is Go's default: never close idle connections")
	flag.Parse()

	var c conns
	work := http.HandlerFunc(c.work)
	plain := &http.Server{
		Addr:        ":8080",
		Handler:     work,
		ConnState:   c.track,
		IdleTimeout: *idle,
	}
	secure := &http.Server{
		Addr:        ":8443",
		Handler:     work,
		ConnState:   c.track,
		IdleTimeout: *idle,
	}
	stats := &http.Server{
		Addr:    ":9090",
		Handler: http.HandlerFunc(c.stats),
	}

	errs := make(chan error, 3)
	go func() {
		errs <- plain.ListenAndServe()
	}()
	go func() {
		errs <- secure.ListenAndServeTLS(*cert, *key)
	}()
	go func() {
		errs <- stats.ListenAndServe()
	}()
	log.Fatal(<-errs)
}
