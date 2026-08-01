# Lighthouse - DNS server for difficult days

A caching DNS proxy with a persistent record book and a beacon-driven
survival mode, plus an embedded web dashboard and Prometheus metrics.

## How it works

- Incoming queries (UDP/TCP/DoT/DoH) are answered from the in-memory cache
  ([otter](https://github.com/maypok86/otter), per-entry TTL) when possible.
- On a miss, **all** configured backend servers are queried concurrently and the
  fastest non-empty response wins. If every server answers empty, an empty
  response is served.
- Every backend answer is persisted to the **record book**
  ([lotusdb](https://github.com/lotusdblabs/lotusdb)) together with its TTL
  (informational) and the query date.
- A **beacon** record is probed on every backend server at an interval. If the
  beacon is unreachable on all servers for `BEACON_FAIL_AFTER`, the proxy
  enters **survival mode**: backends are no longer queried and answers are
  served from the record book, even past their TTL. Once the beacon is
  continuously reachable for `BEACON_RECOVER_AFTER`, normal mode resumes.

## Configuration (environment variables)

| Variable | Default | Description |
|---|---|---|
| `LIGHTHOUSE_BACKENDS` | *(required)* | Comma-separated backend URIs: `udp://1.1.1.1:53,tcp://9.9.9.9:53,tls://8.8.8.8:853,https://dns.google/dns-query` |
| `LIGHTHOUSE_LISTEN_UDP` | `:53` | UDP listener (empty to disable) |
| `LIGHTHOUSE_LISTEN_TCP` | `:53` | TCP listener (empty to disable) |
| `LIGHTHOUSE_LISTEN_DOT` | | DoT listener, e.g. `:853` |
| `LIGHTHOUSE_LISTEN_DOH` | | DoH (HTTPS) listener, e.g. `:443`, endpoint `/dns-query` |
| `LIGHTHOUSE_TLS_CERT` / `LIGHTHOUSE_TLS_KEY` | | TLS keypair, required for DoT/DoH |
| `LIGHTHOUSE_BACKEND_TIMEOUT` | `3s` | Per-backend query timeout |
| `LIGHTHOUSE_CACHE_MAX_SIZE` | `100000` | Max entries in the in-memory cache |
| `LIGHTHOUSE_RECORDBOOK_PATH` | `./data/recordbook` | lotusdb directory |
| `LIGHTHOUSE_STORE_NEGATIVE` | `false` | Also persist NXDOMAIN/empty answers |
| `LIGHTHOUSE_BEACON_NAME` | *(required)* | Beacon record name |
| `LIGHTHOUSE_BEACON_TYPE` | `A` | Beacon record type |
| `LIGHTHOUSE_BEACON_INTERVAL` | `10s` | Probe interval |
| `LIGHTHOUSE_BEACON_FAIL_AFTER` | `1m` | Beacon down on all backends this long → survival mode |
| `LIGHTHOUSE_BEACON_RECOVER_AFTER` | `1m` | Beacon continuously up this long → normal mode |
| `LIGHTHOUSE_HTTP_ADDR` | `:8080` | Dashboard + JSON API |
| `LIGHTHOUSE_OPS_ADDR` | `:9090` | `/metrics`, `/healthz`, `/readyz` |
| `LIGHTHOUSE_LOG_LEVEL` | `info` | slog level (JSON logs on stdout) |

## Build & run

Requires Go 1.26.5 and Node (both via `asdf install`).

```sh
make build   # builds the Nuxt dashboard, embeds it, compiles the binary
LIGHTHOUSE_BACKENDS=udp://1.1.1.1:53,udp://8.8.8.8:53 \
LIGHTHOUSE_BEACON_NAME=example.com \
./lighthouse
```

Dashboard at `http://localhost:8080` (status, QPS charts, backend health,
record book explorer with search/delete/re-query). Prometheus metrics at
`http://localhost:9090/metrics` (`lighthouse_frontend_*`,
`lighthouse_backend_*`, `lighthouse_cache_entries`,
`lighthouse_recordbook_records`, `lighthouse_survival_mode`).

## Docker

```sh
docker build -t lighthouse .
docker run -d --name lighthouse \
  -e LIGHTHOUSE_BACKENDS=udp://1.1.1.1:53,udp://8.8.8.8:53 \
  -e LIGHTHOUSE_BEACON_NAME=example.com \
  -p 53:53/udp -p 53:53/tcp -p 8080:8080 -p 9090:9090 \
  -v lighthouse-data:/data \
  lighthouse
```

The image is distroless and runs as nonroot; the record book lives in the
`/data` volume. CI (`.github/workflows/`) runs `go vet` + `go test -race` and
the dashboard build on every push/PR, and pushes images to
`ghcr.io/<owner>/lighthouse` on `main` (`latest`, `main`, `sha-*`) and on
`v*` tags (semver tags).

## Layout

Services live under `internal/<service>/` with `service.go`, `model/`, and
`storage/` (repositories). Cross-service calls use consumer-side interfaces
built on primitive types, satisfied by the services directly; `internal/adapters/`
is reserved for cases where service models must be translated (currently none).
Delivery edges (`internal/dnsserver`, `internal/httpserver`) consume services
directly via consumer interfaces. Wiring happens in `cmd/lighthouse/main.go`.
