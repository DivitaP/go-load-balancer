# Adaptive Layer 7 Load Balancer

An HTTP reverse proxy written in Go. It sits in front of a pool of backend servers, picks one per request, checks whether each backend is actually alive, and exposes what it's doing through a live dashboard and a Prometheus endpoint.

I built this to learn Go. I have a few years of backend work in Java and Spring Boot, plus a distributed systems background, but Go was new to me. Writing a load balancer seemed like a good way in, because it forces you to deal with concurrency, shared state, and the network stack all at once rather than reading about them separately.

## What it does

- Reverse proxies HTTP and HTTPS traffic to a pool of backends
- Three routing strategies: round robin, least connections, weighted
- Active health checks per backend, with a configurable failure threshold
- Per-backend latency tracking using an exponential moving average
- A `/metrics` endpoint in Prometheus text format, written by hand
- A live dashboard at `/dashboard` showing request rate, latency, and backend health
- TLS termination
- YAML config

## Running it

You need Go 1.20 or later.

```bash
git clone https://github.com/DivitaP/go-load-balancer
cd go-load-balancer
go mod download
```

Start two backends in separate terminals:

```bash
go run ./cmd/testbackend -port 8081
go run ./cmd/testbackend -port 8082
```

Start the load balancer:

```bash
go run ./cmd/loadbalancer -config config.yaml
```

Then send it some traffic:

```bash
curl http://127.0.0.1:8080/hello
```

Open `http://127.0.0.1:8080/dashboard` in a browser. Charts need two samples before they draw anything, so give it about five seconds.

To generate real load, use `hey` (`brew install hey`):

```bash
hey -z 30s -c 50 http://127.0.0.1:8080/
```

Raise your file descriptor limit first if you're pushing much traffic on macOS:

```bash
ulimit -n 10240
```

### Watching a backend fail

This is the part worth seeing. With load running, kill one of the backends. After three failed health probes it gets marked down, its line on the dashboard drops to zero, and everything shifts to the surviving backend. The client never sees a 5xx, because the health checker pulls the dead backend out of the pool before the next routing decision happens. Bring it back up and the next successful probe puts it back in rotation.

Drop `interval` to `2s` in the config if you want this to happen faster while demoing.

### Weighted routing

Set `strategy: weighted` in the config with weights 3 and 1, restart, and drive load. The request counters on the dashboard settle at a 3:1 split.

### TLS

```bash
openssl req -x509 -newkey rsa:2048 -nodes -keyout key.pem -out cert.pem \
  -days 365 -subj "/CN=localhost"
```

Set `tls.enabled: true` in the config and restart. Then:

```bash
curl -k https://127.0.0.1:8080/hello
```

The backends keep speaking plain HTTP. The load balancer terminates TLS and forwards decrypted requests, which is what termination means.

## Config

```yaml
port: 8080
tls:
  enabled: false
  cert: cert.pem
  key: key.pem
strategy: least_connections
health_check:
  path: /health
  interval: 10s
  threshold: 3
  timeout: 2s
backends:
  - url: http://127.0.0.1:8081
    weight: 3
  - url: http://127.0.0.1:8082
    weight: 1
```

Config is validated at startup. A bad strategy name, an unparseable backend URL, or TLS enabled without a cert path all refuse to boot. I'd rather the process die on startup than at 2am on the first request.

I use `127.0.0.1` instead of `localhost` on purpose. On macOS `localhost` resolves to `::1` first, so every dial races an IPv6 and an IPv4 attempt. Pinning to IPv4 makes it deterministic and cleans up the logs.

## How it's put together

```
cmd/loadbalancer/     entrypoint, wiring, graceful shutdown
cmd/testbackend/      throwaway HTTP server for local testing
internal/backend/     per-backend state, EMA latency, health transitions
internal/balancer/    Strategy interface + three implementations
internal/proxy/       the reverse proxy and http.Handler
internal/health/      active probing loop
internal/metrics/     Prometheus text format endpoint
internal/dashboard/   ring buffer of samples, JSON API, embedded HTML page
config/               YAML parsing and validation
```

Three things run concurrently: the HTTP server accepting client requests, a health check loop probing backends on a ticker, and a sampler feeding the dashboard's history buffer. They all share the backend pool, which is where most of the interesting problems live.

## Decisions I made and why

### Mutex for some fields, atomics for others

`Backend` has both a `sync.RWMutex` and a couple of `atomic` counters, which looked inconsistent to me at first.

The mutex protects `alive`, `failCount`, and `avgLatency`. Those change together during a health transition and need to be consistent with each other. It's an `RWMutex` rather than a plain `Mutex` because routing reads `alive` on every single request while the health checker writes it every ten seconds, so reads massively outnumber writes.

The connection count and request counters are independent single integers touched on every request. Taking a lock just to increment a number is wasteful when the hardware has a compare-and-swap instruction, so those are `atomic.Int64` and `atomic.Uint64`. Same reasoning as `AtomicLong` versus `ReentrantReadWriteLock` in Java.

I originally had `Alive bool` as an exported field sitting next to the mutex. That's a bug waiting to happen, because any caller can read it without holding the lock. Unexporting it and going through `IsAlive()` means the compiler enforces the invariant.

I run the tests with `go test -race`, which instruments memory access and catches actual data races at runtime. There's a test that hammers a backend from fifty goroutines specifically to give the race detector something to find.

### One Transport per backend

This one cost me an afternoon. Under load the proxy started failing with `dial tcp: resource temporarily unavailable`, and `netstat` showed thousands of sockets stuck in `TIME_WAIT`.

The cause was that I left `ReverseProxy.Transport` nil, so it fell back to `http.DefaultTransport`, which caps idle connections per host at **two**. With twenty concurrent requests, twenty connections open, two get kept, eighteen get closed. Each closed one holds an ephemeral port in `TIME_WAIT` for twice the maximum segment lifetime. There are only about sixteen thousand ephemeral ports, so at that rate you exhaust them in seconds and the kernel starts refusing new sockets.

The fix is giving each backend its own `http.Transport` with an idle pool sized to expected concurrency. Per-backend rather than shared, so a dead backend's failed dials can't starve the pool serving healthy ones. Same bulkhead idea as separate thread pools per downstream dependency.

`TIME_WAIT` isn't waste, incidentally. It exists so a delayed packet from a closed connection can't be delivered to a new connection that reused the same four-tuple. The fix is to stop closing connections, not to tune `TIME_WAIT` away.

There's a regression test for this. It counts `http.StateNew` transitions on the origin server across two waves of concurrent requests and fails if the second wave re-dials.

### `Rewrite` instead of `Director`

`httputil.ReverseProxy` has an older `Director` hook that mutates the inbound request in place, which had header smuggling problems. The newer `Rewrite` hook gets separate views of the inbound and outbound requests, and `SetXForwarded()` populates `X-Forwarded-For`, `X-Forwarded-Host`, and `X-Forwarded-Proto` correctly.

### Health probes run concurrently

Each tick fans out to all backends at once through a `WaitGroup`. If they ran sequentially and one backend hung until its two-second timeout, detection of every other backend would queue up behind it.

The probe drains and closes the response body even though it doesn't care about the contents. Go's HTTP client only returns a connection to its pool if the body is fully read and closed, so skipping the drain means a fresh TCP connection per probe.

Cancellation goes through `context.Context`. One `signal.NotifyContext` at the top of `main` turns Ctrl+C into a cancelled context, which stops the health loop, stops the dashboard sampler, and triggers `srv.Shutdown` to drain in-flight requests. One signal fans out to every subsystem, which is cleaner than anything I've done with `Thread.interrupt()`.

### Hand-written Prometheus format

I could have imported `client_golang`. Writing the exposition format by hand meant I actually had to learn what's in it: the `HELP` and `TYPE` comment lines, the label sets, the difference between a counter that only goes up and a gauge that moves freely.

Counters are cumulative totals. A chart of `lb_requests_total` is a boring straight line. What you want is `rate(lb_requests_total[1m])`, which Prometheus computes at query time as delta over elapsed time. The dashboard does the same arithmetic in JavaScript, which is how I understood what `rate()` was doing in the first place.

The one thing I was careful about is label cardinality. I label by backend host, which is bounded by the config file. Labeling by request path or client IP would allocate a new time series per unique value, and that's what actually kills real Prometheus deployments.

For anything production I'd use `client_golang`, because it handles concurrency, cardinality limits, and content negotiation that I skipped.

### Dashboard history is a fixed-size ring buffer

A load balancer runs for weeks. An append-only slice of samples is a memory leak with a nice name. The buffer holds 300 samples at two-second intervals, so ten minutes of history in constant memory, with old samples overwritten. Bounded retention is the price of bounded memory, and I'd rather make that trade explicitly than discover it later.

The HTML page is compiled into the binary with `go:embed`. `go build` gives you one file you can drop on any machine. There's no asset directory to ship and no path bug to hit.

I got bitten by writing `// go:embed` with a space, which the compiler silently reads as an ordinary comment. The variable stayed a zero-value `embed.FS` and the handler returned a 500 at request time. I switched to embedding straight into a `[]byte`, which turns a missing file into a build failure instead of a runtime error, and deleted the error branch entirely. Same instinct as unexporting the `alive` field: if you can make the bad state unrepresentable, do that instead of handling it.

### Strategy is an interface

The proxy depends on a `Strategy` interface with one method, not on any concrete type. Adding a latency-aware strategy later is a new file, not a rewrite. Go interfaces are satisfied implicitly, so there's no `implements` keyword and no import from the implementation back to the interface.

Round robin uses an atomic counter and scans forward from its index to skip dead backends. That means fairness drifts slightly when backends are down, since some slots get skipped. I took that trade for lock-free selection.

Weighted expands each alive backend into `weight` slots and rotates over the expanded list. That's O(total weight) per request and it rebuilds the slice every time, which is fine at this scale but isn't what nginx does. Nginx uses smooth weighted round robin, which spreads a weight-3 backend as `A A B A` rather than clumping it as `A A A B`. That's on the list.

## Tests

```bash
go test -race ./...
go vet ./...
gofmt -l .
```

Everything is tested with real collaborators instead of mocks, which seems to be the Go convention. `httptest.NewServer` spins up an actual HTTP server on a random port, so the proxy tests exercise the full path over a real socket.

Some things I made sure to cover:

- Round robin distributes evenly, skips dead backends, and returns nil when everything is down
- Least connections picks the idle backend, and prefers a busy live backend over an idle dead one
- Weighted lands on the configured ratio over 400 requests
- The proxy returns 503 when no backend is alive and 502 when the selected one is unreachable, and the connection count returns to zero on the error path
- `X-Forwarded-For` carries the original client IP
- Backends go down only after the threshold is reached, and one success resets the failure count
- The connection reuse test described above
- The ring buffer wraps correctly and returns samples in chronological order

The health check tests call the check function directly instead of sleeping through ticker intervals. Timing-based tests are flaky, and what I actually want to test is the state machine, not the scheduler.

## Things I'd do next

- Retry a failed request against a second backend before returning a 502
- Passive health checking, so real request failures mark a backend down rather than waiting for the next probe
- Latency-aware routing, since I'm already tracking the EMA and doing nothing with it
- A latency histogram, so Prometheus can compute p95 and p99. Right now I expose an EMA gauge, and you can't get percentiles out of a gauge
- Smooth weighted round robin
- A separate port for `/metrics` and `/dashboard`. Right now the control plane and data plane share a port, and `ServeMux` prefix matching keeps them apart. Nginx puts the status endpoint on its own listener, and there's a reason for that
- Connection draining per backend on config reload

## Notes on the Go

Coming from Java, the things that took adjusting to:

Goroutines are cheap enough that spawning one per health check per tick is unremarkable. The runtime multiplexes them onto OS threads, so thousands of them cost almost nothing.

`defer` is a better `finally`, because it's attached to the acquisition rather than to a block far below it. `b.IncConns()` followed immediately by `defer b.DecConns()` means the decrement can't be forgotten on an error path or a panic.

`context.Context` for cancellation is explicit and cooperative, and it propagates. There's nothing to hunt down at shutdown.

Channels get all the attention, but this project mostly needs shared state and locks. Knowing when *not* to reach for a channel turned out to be part of writing idiomatic Go.

---

Divita Phadakale
[github.com/DivitaP](https://github.com/DivitaP)