# Reflector: project plan and specification

A single-binary Go HTTP echo server that stands in for backend servers in demos and testing. It ships with a built-in set of inspection, simulation, and synthetic-response routes, and is extended after build through YAML route definitions — no rebuilding, no code.

Primary use case: a demo backend behind a load balancer. Every response identifies the instance that served it, health can be toggled live to demonstrate failover, and request capture verifies what the load balancer actually forwarded.

## Design principles

1. **Zero-config useful.** `./reflector` with no arguments starts a server that echoes requests and identifies itself. Everything else is additive.
2. **Extend without rebuilding.** Response behavior lives in YAML route files and payload files on disk. The binary ships the engine and built-in patterns; users ship their own routes.
3. **Every response tells you who served it.** Hostname, instance ID, port, and timestamp appear in all built-in responses and are available to user templates.
4. **Familiar paths, own schema.** Built-in routes reuse the httpbin path vocabulary (`/status/{code}`, `/delay/{dur}`, `/headers`) because it's what people already type. Response bodies use Reflector's own consistent JSON envelope, not httpbin's schemas.
5. **Per-request overrides.** Any client can bend a response with query params or headers (status, delay, body) without touching config.
6. **One static binary.** stdlib-only core, scratch container, trivially cross-compiled.
7. **Never the bottleneck.** Built-in routes stream synthetic payloads and avoid per-request heavy allocation so Reflector doesn't distort the systems being tested. This is a floor, not a benchmarking mission.

## Architecture

```
reflector (binary)
├── HTTP server (stdlib net/http, Go 1.22+ mux)
│   ├── HTTP/1.1 with keep-alive
│   ├── HTTP/2 over TLS (h2, via ALPN)
│   └── HTTP/2 cleartext (h2c, opt-in flag)
├── TCP echo listener (RFC 862, opt-in via --tcp-port)
├── Built-in routes (compiled in)
│   ├── Inspection:  /  /echo  /anything/*  /headers  /ip  /user-agent
│   ├── Simulation:  /status/{code}  /delay/{dur}  /drip  /stream-bytes/{n}
│   ├── Synthetic:   /size/{bytes}  /bytes/{n}  /json  /xml  /html
│   ├── Auth:        /basic-auth/{user}/{pass}  /bearer  /cookies*
│   ├── Encoding:    /gzip  /deflate
│   ├── WebSocket:   /ws  (RFC 6455 echo, hand-rolled over http.Hijacker)
│   └── Probes:      /healthz  /readyz
├── Route engine
│   ├── routes.d/*.yaml    user route definitions
│   ├── payloads/          static body files referenced by routes
│   └── embedded presets   route packs compiled in via go:embed
├── Template engine (Go text/template + helpers)
├── Request capture (bounded ring buffer, off by default)
└── Admin API (/admin/*, separate port, default 8081)
```

Configuration is command-line flags only (`reflector --help` for the full list); there is no environment-variable or config-file layer.

## Response envelope

Built-in inspection routes return one consistent JSON shape:

```json
{
  "server": {
    "hostname": "web-2",
    "instance_id": "a1b2c3",
    "port": 8080,
    "version": "0.3.1"
  },
  "request": {
    "method": "GET",
    "path": "/anything/orders/42",
    "proto": "HTTP/2.0",
    "headers": { "X-Forwarded-For": ["203.0.113.9"] },
    "query": { "verbose": ["1"] },
    "body": "",
    "client": { "ip": "10.0.0.5", "port": 55302 },
    "tls": { "enabled": true, "version": "1.3", "sni": "demo.local" }
  },
  "timestamp": "2026-08-06T14:22:31Z"
}
```

Rules: headers and query params are multi-value arrays (no silent loss of duplicates); `server` block appears in every built-in JSON response; plaintext output (default content negotiation for curl without an Accept header) presents the same fields in whoami-style lines.

## Built-in routes

**Inspection.** `/` returns the envelope, content-negotiated between plaintext and JSON. `/echo` and `/anything/*` accept any verb and any subpath and return the full envelope including body. `/headers`, `/ip`, `/user-agent` return just those sections of the envelope.

**Simulation.** `/status/{code}` responds instantly with the given status. `/delay/{dur}` waits before responding; implemented with `time.NewTimer` + `select` on `r.Context().Done()` so disconnecting clients release resources immediately. `/drip?numbytes=&duration=&delay=&code=&jitter=` trickles bytes over a duration for slow-backend, timeout, and retry-policy demos. `/stream-bytes/{n}?chunk_size=` streams chunked binary.

**Synthetic payloads.** `/size/{bytes}` and `/bytes/{n}?seed=` generate exact-size responses (zero-filled, or deterministic pseudo-random when seeded), streamed from shared static buffer pages via `io.CopyN`. `/json`, `/xml`, `/html` serve small structural fixtures.

**Auth.** `/basic-auth/{user}/{pass}` issues a 401 challenge and validates credentials. `/bearer` validates presence of a bearer token. `/cookies`, `/cookies/set`, `/cookies/delete` support cookie round-trips, including load balancer persistence-cookie demos.

**Encoding.** `/gzip` and `/deflate` negotiate and return compressed responses using pooled writers with retention caps.

**WebSocket.** `/ws` completes an RFC 6455 handshake and echoes text and binary frames verbatim, answering ping with pong and closing cleanly — a stand-in backend for the one non-HTTP protocol demo HAProxy configs commonly get wrong (`timeout tunnel`, upgrade handling). See [WebSocket echo mode](#websocket-echo-mode).

**Probes.** `/healthz` (liveness) and `/readyz` (readiness), both toggleable through the admin API.

## User-defined routes

Route files in `routes.d/` are loaded at startup. Route precedence on path conflict: user routes in `routes.d/` > loaded presets > built-ins. Within `routes.d/`, later files win. User routes may shadow any built-in except `/admin/*`, `/healthz`, `/readyz`, and `/ws`; `--strict-builtins` locks all built-ins.

```yaml
# routes.d/users-api.yaml
routes:
  - path: /api/users
    method: GET
    response:
      status: 200
      headers:
        Content-Type: application/json
      body_file: payloads/users.json
      delay: 150ms

  - path: /api/users/{id}
    method: GET
    response:
      status: 200
      body: |
        {
          "id": "{{ .PathParam "id" }}",
          "served_by": "{{ .Hostname }}",
          "requested_at": "{{ .Timestamp }}"
        }

  - path: /flaky
    response:
      status: 200
      body: "ok"
      failure:
        rate: 0.3        # 30% of requests return the failure status
        status: 503

  - path: /slow-api
    response:
      status: 200
      body_file: payloads/report.json
      delay: 800ms±400ms   # base latency with uniform jitter
```

Field spec: `path` required; `method` defaults to any; `status` defaults to 200; `body` and `body_file` mutually exclusive; `headers` map; `delay` accepts Go durations with optional `±jitter`; `failure.rate` in [0,1] with `failure.status`.

Template context: request method, path, path params, query params, headers, raw body, parsed JSON body; server hostname, instance ID, port; helpers for timestamp, uuid, random int/string, and string repetition (payload padding). Templates prioritize flexibility; the allocation-light guarantees apply to built-in routes only.

Tooling: `reflector validate [dir]` checks route files and fails with file/line errors before deploy. `reflector routes` prints the resolved route table with sources. `--watch` hot-reloads route files via fsnotify.

## Preset route packs

Five route packs are embedded in the binary via `go:embed`, each teaching one demo scenario:

- **rest-api** — fake JSON API with templated path params (users, orders).
- **flaky** — endpoints with error-rate injection.
- **slow** — endpoints with latency and jitter.
- **auth** — bearer-token-gated endpoints returning 401 without credentials.
- **big-payloads** — large JSON and binary responses.

Two ways to use them:

- `--preset rest-api,flaky` loads packs directly into the route table at startup — a fresh binary serves a fake API with no files on disk. Loaded presets sit below `routes.d/` in precedence, so user routes always win.
- `reflector init [preset...]` writes the packs out to `routes.d/` and `payloads/` as plain editable files — the worked-example path for learning the format. `init` never overwrites existing files unless `--force` is passed.

The embedded preset directory is also the repo's `examples/` directory (one `go:embed` source), so repo browsers, `--preset` users, and `init` users see identical content with no drift. Preset paths and shapes are versioned API surface: packs stay small, and changes to them are treated as breaking.

## Per-request overrides

All routes honor overrides unless `--no-overrides` is set:

- `?_status=503` or `X-Reflector-Status: 503`
- `?_delay=2s` or `X-Reflector-Delay: 2s`
- `?_body=...` or `X-Reflector-Body: ...`
- `?_connection=close` or `X-Reflector-Connection: close` — force the response to close the connection (`Connection: close`), for testing load-balancer behavior on a dropped keep-alive.
- `?_abort=1` or `X-Reflector-Abort: 1` — write a partial response and then close the connection mid-body, simulating a backend that dies while streaming.

## Request capture

`--capture` enables a bounded in-memory ring buffer of recent requests. Purpose: send traffic through the load balancer, then read back exactly what reached the backend — forwarded headers, rewritten paths, persistence cookies.

- `GET /admin/requests` returns captured requests as JSON; `DELETE /admin/requests` clears.
- Bounds: fixed capacity (`--capture-max`, default 100, FIFO eviction); bodies truncated at 1 MiB and flagged `truncated: true`; disabled by default.

## Admin API

Served on its own port (`--admin-port`, default 8081) so it can be firewalled separately from traffic.

- `POST /admin/health/down` and `POST /admin/health/up`: flip probe endpoints to failing/passing, for live failover demos.
- `GET|DELETE /admin/requests`: capture ring access.
- `POST /admin/routes/reload`: re-read routes.d without restart.
- `GET /admin/routes`: list active routes and their sources.

## TLS and protocols

- `--crt` / `--key` for provided certificates.
- `--tls-self-signed` generates an ephemeral in-memory ECDSA certificate at startup (SANs: localhost, 127.0.0.1, ::1) — instant HTTPS with no files.
- HTTP/2 (h2) negotiated via ALPN whenever TLS is active.
- `--h2c` enables cleartext HTTP/2, for topologies where TLS terminates at the load balancer.

## WebSocket echo mode

`GET /ws` upgrades to WebSocket and echoes frames back, for putting a non-HTTP protocol behind HAProxy without a second demo tool.

- Hand-rolled RFC 6455 over `http.Hijacker` — no new dependency, same stdlib-only ethos as the rest of the core.
- Text and binary frames are echoed verbatim (including the FIN bit, so fragmented messages pass through frame-by-frame with no reassembly); ping gets pong; close is answered in kind.
- On connect, one text frame carries the identity banner (`reflector host=web-2 instance=a1b2c3 port=9000`) — the WebSocket analogue of `--tcp-banner`, so L7 round-robin and stickiness stay visible over a persistent connection.
- A non-upgrade request gets `426 Upgrade Required` instead of hanging.
- Good for exercising `timeout tunnel`, `option http-server-close`, and upgrade-handling config, which is where HAProxy WebSocket configs are subtly wrong most often.

## TCP echo mode

`--tcp-port {port}` starts a raw TCP echo listener alongside the HTTP server, for L4 load balancing demos and connection-level testing.

- Default behavior is pure echo (RFC 862): bytes in, bytes out, connection stays open until the client closes. Predictable for scripted tests.
- `--tcp-banner` sends a single identity line on connect (`reflector host=web-2 instance=a1b2c3 port=9000`) before echoing — this is what makes L4 round-robin visible from `nc` in a demo.
- Per-connection read deadline (`--tcp-idle-timeout`, default 5m) so abandoned connections don't accumulate.
- Off unless the flag is set; no UDP.

## Performance requirements

- Synthetic payloads stream from shared static buffers; no per-request allocation proportional to response size.
- Compression writers pooled via `sync.Pool` with a retention cap; oversized buffers dropped, not retained.
- All delay/drip timers select against request context; client disconnects free resources immediately.
- Server-side read/write/idle timeouts and a bounded request-body read limit on by default.
- stdlib `net/http` throughout; no custom multiplexer or non-standard HTTP stack.

## Milestones

### v0.1 — Core reflection server
Ship when it's the best zero-config identity-aware echo binary available.

- Response envelope, content negotiation, instance identity.
- Inspection routes: /, /echo, /anything/*, /headers, /ip, /user-agent.
- Simulation: /status, /delay. Synthetic: /size, /bytes.
- /healthz, /readyz.
- TCP echo mode: --tcp-port, --tcp-banner, idle timeout.
- Flags: --port, --host, --hostname-override, --quiet.
- Structured logging (slog), graceful shutdown, default timeouts.
- Unit tests on all built-ins; GitHub Actions CI.

### v0.2 — Config-driven routes
Ship when a user can define a fake REST API without writing Go.

- Route engine with precedence and shadowing rules; template engine and context; body_file with Content-Type inference.
- `reflector validate` and `reflector routes`.
- Embedded presets (rest-api, flaky, slow, auth, big-payloads) with `--preset` loading and `reflector init` extraction; presets double as the repo's examples/ directory.

### v0.3 — Behavior simulation and demo controls
Ship when a live failover demo runs end to end on it.

- failure blocks and delay jitter; /drip, /stream-bytes.
- Auth routes (/basic-auth, /bearer, /cookies) and encoding routes (/gzip, /deflate).
- Admin API: health toggle, routes reload/list.
- Request capture + /admin/requests.
- --watch hot reload.
- Connection behaviors: forced Connection: close, abort mid-response.
- TLS (provided and self-signed), h2, --h2c.
- Per-request overrides and --no-overrides.

### v0.4 — Distribution
Ship when a stranger is running it in under a minute.

- goreleaser: linux/darwin/windows, amd64/arm64 release binaries.
- Docker: scratch-based multi-arch images; routes.d as a volume mount.
- Kubernetes examples: Deployment + Service, ConfigMap-mounted routes, probe wiring.
- README: 60-second quickstart (`docker run` → `reflector --preset rest-api`) plus a cookbook — one recipe per scenario (visible load balancing, failover, slow backend, flaky backend, fake API, verifying LB-injected headers), each built on a preset.

## Out of scope for v1

SSE/JSON Lines streaming, synthetic image generation, multi-bin request capture with TTLs, Brotli, response sequencing, UDP listeners, gRPC, Prometheus metrics, web dashboard. Any of these can be added later without structural change; none block the core use case.

## References

- traefik/whoami — instance-identity response conventions.
- mccutchen/go-httpbin — path vocabulary reference for built-in routes.
- Ealenn/Echo-Server — per-request override pattern.
- friendsofgo/killgrave — config-file route definition schema, for comparison when finalizing ours.
