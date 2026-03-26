# pkg/util/http

## Purpose

`pkg/util/http` provides shared HTTP client infrastructure for agent components. It centralises
proxy resolution, TLS configuration, connection management, and a few thin helpers so that
every outbound HTTP call in the agent uses consistent, config-driven behaviour instead of
rolling its own `http.Transport`.

## Key elements

### Transport

**`CreateHTTPTransport(cfg pkgconfigmodel.Reader, opts ...func(*http.Transport)) *http.Transport`**

Creates a `*http.Transport` pre-configured from the agent configuration:

| Config key | Effect |
|---|---|
| `skip_ssl_validation` | Sets `InsecureSkipVerify` |
| `min_tls_version` | Minimum TLS version (`tlsv1.0`–`tlsv1.3`, default `tlsv1.2`) |
| `tls_handshake_timeout` | TLS handshake timeout (default 10 s) |
| `http_dial_fallback_delay` | RFC 6555 Happy Eyeballs fallback delay (disabled by default) |
| `sslkeylogfile` | Path for NSS key log (TLS session debug) |
| `proxy.*` | Proxy settings; delegates to `GetProxyTransportFunc` when set |

HTTP/2 is disabled by default (setting a custom `DialContext` suppresses the automatic upgrade).

Optional transport modifier functions (`opts`) are applied last, allowing callers to override
individual fields.

**`WithHTTP2() func(*http.Transport)`** — transport option that enables HTTP/2 via
`golang.org/x/net/http2.ConfigureTransport`.

**`MaxConnsPerHost(n int) func(*http.Transport)`** — transport option to cap per-host connections.

**`GetProxyTransportFunc(p *pkgconfigmodel.Proxy, cfg pkgconfigmodel.Reader) func(*http.Request) (*url.URL, error)`**

Returns a `http.Transport.Proxy` function that evaluates the `proxy.http`, `proxy.https`, and
`proxy.no_proxy` config values for each request. When `no_proxy_nonexact_match` is `false`
(the current default) it performs exact-host matching; when `true` it delegates to the
standard `golang.org/x/net/http/httpproxy` library. Transitional deprecation warnings are
emitted once per URL for cases where the two modes would behave differently (see proxy warnings
section below).

### Client with connection resets

**`ResetClient`** wraps `*http.Client` and periodically recreates the underlying client to
evict stale connections. This is important for long-lived agent processes where upstream
endpoints can change or connections silently break.

```go
client := httputils.NewResetClient(
    endpoint.ConnectionResetInterval, // 0 disables resets
    func() *http.Client {
        return &http.Client{Transport: httputils.CreateHTTPTransport(cfg)}
    },
)
resp, err := client.Do(req)
```

`Do` is thread-safe. When the reset interval elapses, `CloseIdleConnections` is called on the
old client and a new one is created via the factory.

### Auto-renewing API token

**`APIToken`** caches a bearer token value with an expiration date and calls a renewal
callback when the token is expired. Concurrent callers are handled with a read/write lock:
only one goroutine performs the renewal.

```go
token := httputils.NewAPIToken(func(ctx context.Context) (string, time.Time, error) {
    // fetch token from credentials store
    return value, expiresAt, nil
})
val, err := token.Get(ctx)
```

### High-level helpers

| Function | Description |
|---|---|
| `Get(ctx, url, headers, timeout, cfg)` | One-shot GET; returns body as string |
| `Put(ctx, url, headers, body, timeout, cfg)` | One-shot PUT; returns body as string |
| `SetJSONError(w, err, code)` | Writes a JSON error response to an `http.ResponseWriter` |

Both `Get` and `Put` create a fresh transport from config on each call. They are convenience
functions for simple, infrequent requests. Long-lived, high-throughput paths should use
`ResetClient` directly.

**`StatusCodeError`** is returned by the helpers when the server responds with a non-200
status. Callers can type-assert to inspect `StatusCode`, `Method`, and `URL`.

### Proxy deprecation warnings

`GetProxyTransportFunc` tracks three categories of URLs (keyed by scheme+host) and emits
each warning at most once:

| Getter | Meaning |
|---|---|
| `GetProxyIgnoredWarnings()` | URL uses a proxy today but will bypass it when `no_proxy_nonexact_match` becomes the default |
| `GetProxyUsedInFutureWarnings()` | URL bypasses a proxy today but will use one in the future |
| `GetProxyChangedWarnings()` | Proxy selection for the URL will change |
| `GetNumberOfWarnings()` | Total count across all three categories |

## Usage

### Typical transport/client setup

Most components call `CreateHTTPTransport` once during startup and wrap the result in a
standard `http.Client` or in a `ResetClient`:

```go
// logs pipeline (pkg/logs/client/http/destination.go)
transport = httputils.CreateHTTPTransport(cfg)
// or with HTTP/2:
transport = httputils.CreateHTTPTransport(cfg, httputils.WithHTTP2())

// long-lived client with periodic connection reset
client = httputils.NewResetClient(resetInterval, func() *http.Client {
    return &http.Client{
        Transport: httputils.CreateHTTPTransport(cfg),
        Timeout:   timeout,
    }
})
```

### Scrubbing proxy credentials from logs

`GetProxyTransportFunc` never logs the raw proxy URL. It replaces credentials with
`*****:*****@` before writing debug messages, so proxy passwords never appear in agent logs.

For broader credential scrubbing across the agent (API keys, tokens, YAML passwords, TLS
certificates), see [`pkg/util/scrubber`](scrubber.md). The `DefaultScrubber` is installed as
the agent's log scrub function so every log message — including HTTP-related ones — is cleaned
before being written.

### IPC transport

When agents communicate over the internal IPC channel (e.g., a CLI command talking to the core
agent daemon), the transport is handled by [`comp/core/ipc`](../../comp/core/ipc.md) rather
than by this package directly. `comp/core/ipc` wraps an `http.Client` pre-loaded with the IPC
bearer token and mutual-TLS certificate; it uses a Unix domain socket or vsock rather than a
TCP connection.

For gRPC-over-TLS between agent processes, see [`pkg/util/grpc`](grpc.md), which wraps the TLS
config sourced from `comp/core/ipc` into gRPC dial options.

### Remote Configuration HTTP client

The Remote Configuration `api` sub-package (`pkg/config/remote/api`) uses
`httputils.CreateHTTPTransport` to build the HTTPS client that calls the Datadog RC backend.
See [`pkg/config/remote`](../../pkg/config/remote.md) for the full RC architecture.

### Configuration reference

All settings read by this package live under the standard agent config. No build tags are
required; the package is always compiled.

## Cross-references

| Topic | See also |
|-------|----------|
| Credential/token scrubbing for logs and flares | [`pkg/util/scrubber`](scrubber.md) |
| IPC bearer token and mTLS transport for inter-process calls | [`comp/core/ipc`](../../comp/core/ipc.md) |
| gRPC transport over TLS | [`pkg/util/grpc`](grpc.md) |
| Remote Configuration HTTP client to Datadog backend | [`pkg/config/remote`](../../pkg/config/remote.md) |
