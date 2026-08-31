# Reflector

A single-binary HTTP echo server that stands in for backend servers in
load-balancer demos and testing. Every response identifies the instance
that served it, health can be toggled live to demonstrate failover, and
request capture verifies exactly what a load balancer forwarded.

See [PLAN.md](PLAN.md) for the full design and flag reference.

## Quickstart (60 seconds)

```sh
brew install rlnorthcutt/tap/reflector
reflector --preset rest-api
```

```sh
curl localhost:8080/                # identity + request envelope
curl localhost:8080/api/users       # fake JSON API, from the rest-api preset
```

No Homebrew? Any of these work the same way:

```sh
# Prebuilt binary (linux/darwin/windows, amd64/arm64)
# — download from https://github.com/rlnorthcutt/reflector/releases

# Go toolchain
go install github.com/rlnorthcutt/reflector/cmd/reflector@latest

# Docker
docker run -p 8080:8080 -p 8081:8081 ghcr.io/rlnorthcutt/reflector:latest --preset rest-api
```

## Cookbook

Each recipe builds on a preset — `reflector init <preset>` extracts its
route files to `routes.d/` and `payloads/` if you want to edit them.

### Visible load balancing

Run three instances and put HAProxy in front. Every response's
`server.hostname`/`instance_id` (or the plaintext `Hostname:`/`InstanceID:`
lines) shows exactly which one answered.

```sh
reflector --port 9001 --admin-port 9081 --hostname-override web-1 &
reflector --port 9002 --admin-port 9082 --hostname-override web-2 &
reflector --port 9003 --admin-port 9083 --hostname-override web-3 &
```

```haproxy
frontend fe_demo
    bind *:8080
    default_backend be_reflector

backend be_reflector
    balance roundrobin
    option httpchk GET /healthz
    default-server inter 1s fall 2 rise 2
    server web-1 127.0.0.1:9001 check
    server web-2 127.0.0.1:9002 check
    server web-3 127.0.0.1:9003 check
```

```sh
for i in 1 2 3 4 5 6; do curl -s localhost:8080/ | grep Hostname; done
```

### Failover

Using the three instances and HAProxy config above, fail one out without
touching HAProxy at all — flip it via its admin API, which is exactly
what `option httpchk GET /healthz` is watching. With `inter 1s fall 2`,
HAProxy notices within about 2 seconds:

```sh
curl -X POST localhost:9081/admin/health/down   # web-1 starts failing its health check
```

```sh
sleep 2   # let HAProxy's health check catch up
for i in 1 2 3 4 5 6; do curl -s localhost:8080/ | grep Hostname; done   # only web-2/web-3 now
```

Bring it back with:

```sh
curl -X POST localhost:9081/admin/health/up
```

### Slow backend

```sh
reflector --preset slow
curl -w '\n%{time_total}s\n' localhost:8080/slow-api          # ~800ms ± 400ms jitter
curl -w '\n%{time_total}s\n' localhost:8080/slow-api/report   # ~2s ± 1s jitter
```

Useful for tuning HAProxy `timeout server`/`timeout connect`, or for
demoing what a slow upstream does to client-perceived latency.

### Flaky backend

```sh
reflector --preset flaky
for i in $(seq 1 10); do curl -s -o /dev/null -w '%{http_code} ' localhost:8080/flaky; done
echo
```

`/flaky` fails ~30% of requests with a 503; `/flaky/checkout` (POST) fails
~50% with a 500. Good for demoing HAProxy retry policies
(`retry-on`, `retries`) or a client's own backoff logic.

### Fake API

```sh
reflector --preset rest-api
curl localhost:8080/api/users
curl localhost:8080/api/users/42
curl localhost:8080/api/orders
```

A believable JSON API with templated path params, useful anywhere you
need "a backend" without writing one. Extract it with
`reflector init rest-api` and edit `routes.d/rest-api.yaml` to reshape it
into your own fake service.

### Verifying LB-injected headers

Confirm exactly what HAProxy adds to (or rewrites on) a request before it
reaches the backend — `/headers` reflects precisely what arrived, with no
normalization:

```haproxy
frontend fe_demo
    bind *:8080
    default_backend be_reflector

backend be_reflector
    option forwardfor
    http-request set-header X-Demo-Backend reflector
    server web-1 127.0.0.1:9001 check
```

```sh
curl -H 'Accept: application/json' localhost:8080/headers
```

Look for `X-Forwarded-For` (from `option forwardfor`) and `X-Demo-Backend`
(from the explicit `set-header`) in the response — if they're not there,
they never left HAProxy.

## Request capture

Turn on `--capture` and read back exactly what reached the backend —
forwarded headers, rewritten paths, persistence cookies:

```sh
reflector --capture
# ...send some traffic through your load balancer...
curl localhost:8081/admin/requests | jq
```

## Docker

```sh
docker run -p 8080:8080 -p 8081:8081 \
  -v $(pwd)/routes.d:/app/routes.d \
  -v $(pwd)/payloads:/app/payloads \
  ghcr.io/rlnorthcutt/reflector:latest
```

Multi-arch (`linux/amd64`, `linux/arm64`), built `FROM scratch` — see
[Dockerfile](Dockerfile).

## Kubernetes

```sh
kubectl apply -k k8s/
```

Three replicas behind a Service, a ConfigMap-mounted route, and
liveness/readiness wired to `/healthz`/`/readyz` — see
[k8s/README.md](k8s/README.md).

## Building from source

```sh
git clone https://github.com/rlnorthcutt/reflector
cd reflector
go build ./cmd/reflector
```
