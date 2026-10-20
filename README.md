# http-pool-lab

The experiments behind *Go's HTTP Client Keeps Two Connections*.

`http.DefaultTransport` has no cap on open connections per host, but keeps only two of them
idle (`MaxIdleConnsPerHost`). Above two requests in flight to one host, the extra connections
are dialled, used once and closed. Nothing queues, so nothing looks slow. The cost shows up
as handshakes, TIME_WAIT sockets and CPU, and on a caller with a CPU limit, as throttling.
These runs make each of those visible.

## Setup

- `server` stands in for the downstream service: plain HTTP on :8080, TLS on :8443 with a
  certificate from `tls/gen.sh` (RSA 2048 by default, `KEY=ecdsa ./tls/gen.sh` for P-256), and
  its CPU and connection counters on :9090/stats. `GET /work?delay=20ms&size=1024` sleeps,
  then replies.
- `lab` is the caller: one `http.Client` per run, built from the transport variant under test,
  with a CPU limit (`CLIENT_CPUS`, default 1). Its dialer wraps every connection so opens and
  closes are counted even when the transport does them on its own.
- Both run in Docker, in separate cgroups and network namespaces, so each side's CPU,
  throttling and TIME_WAIT are its own. `containerd/cgroups` reads `cpu.stat` and
  `prometheus/procfs` reads `/proc/net/tcp`; everything else is the standard library.

Load is **open-loop**: requests are sent on a schedule whether or not earlier ones have
finished. A fixed set of workers in a loop slows down when the caller does, which hides the
cost.

## Running

```sh
./tls/gen.sh
docker compose up -d --build server
docker compose run --rm lab reuse-sweep
go run ./cmd/lab summary
```

`docker compose run --rm lab -h` lists the experiments.

| Experiment | What it shows | Takes |
|---|---|---|
| `reuse-sweep` | rate 50 to 3,200/s (2 to 64 in flight) against reuse: default vs 128 idle per host | 5 min |
| `open-cost` | 500/s on warm vs always-new connections, plain and TLS: the CPU price of a connection on each side | 2 min |
| `time-wait` | a ramp to 2,000/s: which side ends up holding TIME_WAIT, default vs `DisableKeepAlives` | 2–4 min |
| `drain` | a 64 KB body read to the end vs closed unread | 40 s |
| `h2` | over TLS: a `DefaultTransport` clone (HTTP/2, one connection) vs a bare `&http.Transport{}` (HTTP/1.1) | 40 s |
| `max-conns` | `MaxConnsPerHost` 8 and 32 at 16 in flight: the one setup where requests wait for a connection | 40 s |
| `slowdown` | 1,000/s over HTTPS on HTTP/1.1 while the server slows from 20 ms to 100 ms for 30 s: default vs 1024 idle | 5 min |
| `incident` | a 1-CPU caller on HTTP/1.1, plain and TLS, through a burst to `-rate`: default vs session resumption vs 512 idle | 6 min |

Between variants, `lab` waits for the previous run's TIME_WAIT sockets to expire (60 s on
Linux), so every run starts with the same ephemeral ports free.

`incident` needs tuning to the machine: raise `-rate` until the default variant's caller
throttles, e.g. `docker compose run --rm lab -rate 2500 incident`. Run it again with
`GOMAXPROCS=8` (or more) to see what a Go older than 1.25, which ignored the CPU limit, does.

### Repeats and kernel settings

| Flag or variable | What it does |
|---|---|
| `-round N` | names this run's variants `<variant>-rN`, so repeats sit next to the first run |
| `-tag T` | appends `-T` to every variant's name, for runs under a changed setting |
| `-only a,b` | runs only the named variants of an experiment |
| `TW_REUSE`, `PORT_RANGE` | the caller's `net.ipv4.tcp_tw_reuse` (default 2) and `ip_local_port_range` (default `32768 60999`) |
| `CLIENT_CPUS`, `SERVER_CPUS`, `GOMAXPROCS` | the caller's CPU limit (1), the server's (4), and the caller's `GOMAXPROCS` |

The published results come from:

```sh
docker compose run --rm lab reuse-sweep          # then -round 2 and -round 3
docker compose run --rm lab -rate 2400 incident  # then -round 2 and -round 3
docker compose run --rm lab open-cost            # then -round 2 and -round 3
docker compose run --rm lab time-wait            # and drain, h2, max-conns, once each
docker compose run --rm lab slowdown             # then -round 2 and -round 3
TW_REUSE=1 docker compose run --rm lab -tag twreuse -only default-plain -rate 2400 incident
PORT_RANGE="1024 65535" docker compose run --rm lab -tag wideports -only default-plain -rate 2400 incident
TW_REUSE=1 docker compose run --rm lab -tag twreuse open-cost
```

## Results

`results/summary.md` has one table per experiment. Each run writes its raw data to
`results/<experiment>/<variant>/`; those files are large and not committed, and
`go run ./cmd/lab summary` rebuilds the summary from them.

- `ops.csv`: one row per request.
  - `wait_us` is `GetConn` to `GotConn`, the time spent getting a connection. A request that
    never got one is all wait.
  - `connect_us` and `tls_us` belong to the dial this request started. The transport races that
    dial against connections other requests return, so a request can be served on a returned
    connection while its dial goes to someone else. `reused` is which connection it actually got.
  - `ttfb_us` is from the request being written to the first response byte.
- `conns.csv`: per second, connections opened and closed by the caller, live connections,
  dial errors, requests that got a reused vs new connection, TLS handshakes, requests in
  flight, done and failed, and the caller's TIME_WAIT count.
- `cpu.csv`: the caller's cgroup `cpu.stat` every 10 ms (cumulative, as the kernel reports it).
  Throttling is decided per 100 ms period, which a per-second average hides. The sampler runs
  inside the throttled cgroup, so a stalled period delays its samples too; each row carries
  the time it was actually read.
- `server.csv`: the server's `cpu.stat`, connection counters and TIME_WAIT count, every second.
- `meta.json`: the configuration, `GOMAXPROCS`, `cpu.max`, the caller's port-range and
  `tcp_tw_reuse` sysctls, and the Go version.

## Where this is weak

- **One machine.** On macOS, Docker runs in a VM and both containers share its cores; the
  network between them is a bridge, with close to no latency. A real network makes every
  handshake slower, which makes churn cost more, not less.
- **CPU sampling is Linux only.** `cpu.csv` and the CPU columns only exist when the lab runs
  in its container; `summary` runs anywhere.
- **No session resumption by default.** Go's client has no TLS session cache unless you set
  one. That's the realistic default, and `incident` runs with and without it.
- **Evenly spaced arrivals.** Requests go out at exact intervals, so the number in flight
  barely fluctuates. Connections are thrown away when it dips and climbs back past the idle
  limit, so real, clumpy traffic likely churns more than these runs show.
- **TIME_WAIT reuse.** Linux's `tcp_tw_reuse` defaults to loopback only, and the container's
  default ephemeral range is 32768–60999. Setting `TW_REUSE=1` removed the plain-HTTP collapse
  in `incident`; widening the range only delayed it. A host tuned differently hits the port
  wall at a different rate.
- **Not explained yet.** Over TLS the caller holds almost no TIME_WAIT, and a new plain
  connection costs the caller more CPU than a TLS one (its `connect()` is slower from the first
  second). Both show up in every round; neither is understood.
