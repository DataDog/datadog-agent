# pkg/api

## Purpose

`pkg/api` provides shared HTTP API utilities used by multiple agent binaries (core agent, cluster agent, security agent, CLC runner). It is **not** a full HTTP server implementation — that lives in `comp/api`. This package supplies the building blocks that those servers depend on:

- **`security/`** — auth token and TLS certificate management for inter-process communication (IPC).
- **`security/cert/`** — lower-level certificate generation, persistence, and TLS config construction.
- **`util/`** — token validation middleware and Cluster Agent (DCA) auth token helpers.
- **`version/`** — a standard HTTP handler that writes the agent version as JSON.
- **`coverage/`** — optional (build tag `e2ecoverage`) handler that flushes Go coverage counters to disk.

## Key elements

### `pkg/api/security`

| Symbol | Description |
|--------|-------------|
| `FetchAuthToken(config) (string, error)` | Reads the agent auth token from disk (path from `auth_token_file_path` config key, or next to `datadog.yaml`). Does not create the file. |
| `FetchOrCreateAuthToken(ctx, config) (string, error)` | Like `FetchAuthToken` but creates a new 32-byte random hex token if the file does not exist. |
| `GetClusterAgentAuthToken(config) (string, error)` | Reads the DCA token from `cluster_agent.auth_token` config or from `cluster_agent.auth_token` file. Does not create the file. |
| `CreateOrGetClusterAgentAuthToken(ctx, config) (string, error)` | Like above but creates the file if it does not exist. |
| `GetAuthTokenFilepath(config) string` | Returns the resolved path to the auth token file. |
| `GenerateKeyPair(bits int) (*rsa.PrivateKey, error)` | Generates an RSA key pair. |
| `GenerateRootCert(hosts []string, bits int)` | Generates a self-signed RSA root certificate valid for 10 years. Returns the `*x509.Certificate`, PEM bytes, and private key. |
| `CertTemplate() (*x509.Certificate, error)` | Returns an `x509.Certificate` template with a random 128-bit serial number and a 10-year validity window. |

Auth tokens are at least 32 characters. The minimum length is validated on load.

### `pkg/api/security/cert`

Manages the **IPC certificate** (`ipc_cert.pem` by default, next to `datadog.yaml`) used for mutual TLS between agent processes (agent ↔ system-probe, agent ↔ cluster agent, etc.).

| Symbol | Description |
|--------|-------------|
| `Certificate` | Opaque struct holding a PEM-encoded cert/key pair. |
| `FetchIPCCert(config) (clientTLS, serverTLS, clusterClientTLS *tls.Config, err error)` | Loads the IPC cert from disk (no creation). Returns separate `*tls.Config` objects for the local client, local server, and cross-cluster client roles. |
| `FetchOrCreateIPCCert(ctx, config) (clientTLS, serverTLS, clusterClientTLS *tls.Config, err error)` | Like `FetchIPCCert` but generates and persists a new ECDSA P-256 cert if the file is absent. |
| `GetTLSConfigFromCert(cert, key []byte) (clientTLS, serverTLS *tls.Config, err error)` | Constructs client and server `*tls.Config` values from raw PEM bytes. The server config sets `ClientAuth: tls.VerifyClientCertIfGiven` to enable optional mTLS. |

**Cluster trust chain** (`cluster_trust_chain.*` config keys): when a shared cluster CA cert and key are configured, IPC certs are signed by that CA and their SANs include the cluster agent service DNS names and/or the CLC runner IP. Cross-cluster TLS verification can be enforced via `cluster_trust_chain.enable_tls_verification`.

### `pkg/api/util`

Stateful, process-global helpers for the DCA auth token and cross-node TLS.

| Symbol | Description |
|--------|-------------|
| `InitDCAAuthToken(config) error` | Initialises the process-global DCA token (no-op if already set). Calls `CreateOrGetClusterAgentAuthToken` internally. |
| `GetDCAAuthToken() string` | Returns the cached DCA token. |
| `SetCrossNodeClientTLSConfig(config *tls.Config)` | Stores the TLS config used for cross-node (agent-to-agent) HTTP calls. Write-once: logs a warning and ignores subsequent calls. |
| `GetCrossNodeClientTLSConfig() (*tls.Config, error)` | Returns the stored cross-node TLS config, or an error if not yet set. |
| `TokenValidator(tokenGetter func() string) func(http.ResponseWriter, *http.Request) error` | Returns an HTTP middleware that validates a `Bearer` token in the `Authorization` header using a constant-time comparison. Returns `401` for missing/unsupported schemes and `403` for invalid tokens. |
| `IsForbidden(ip string) bool` | Returns `true` for wildcard bind addresses (`""`, `0.0.0.0`, `::`, etc.). Used to prevent accidentally over-permissive listener IPs. |
| `IsIPv6(ip string) bool` | Returns `true` if the string parses as an IPv6 address (not IPv4-mapped). |

### `pkg/api/version`

| Symbol | Description |
|--------|-------------|
| `Get(w http.ResponseWriter, r *http.Request)` | Writes the current agent version as a JSON object (using `pkg/version.Agent()`). Used as the `/version` endpoint on every agent HTTP API. |

### `pkg/api/coverage`

Build-tag-gated (`e2ecoverage`). Registers a `GET /coverage` endpoint that calls `runtime/coverage.WriteCountersDir` and `WriteMetaDir` to flush instrumentation counters to a temporary directory. Used during end-to-end coverage runs.

## Usage

### Auth tokens

Every agent that exposes an HTTP API initialises its auth token at startup:

```go
// Agent / Security Agent / CLC Runner
security.FetchOrCreateAuthToken(ctx, config)

// Cluster Agent side
util.InitDCAAuthToken(config)
```

Consumers (CLI commands, other agent processes) load the token with `security.FetchAuthToken(config)` and inject it as `Authorization: Bearer <token>` on every request.

### IPC TLS

The IPC component (`comp/core/ipc`) calls `cert.FetchOrCreateIPCCert` at startup to obtain the three `*tls.Config` objects. All internal HTTP servers use the server config; all internal clients use the client config.

### `/version` endpoint

Registered across all agent binaries via `comp/api/commonendpoints`:

```go
VersionEndpoint: api.NewAgentEndpointProvider(version.Get, "/version", "GET")
```

### Token validation middleware

Used by the cluster agent and CLC runner API servers:

```go
router.Handle("/path", util.TokenValidator(util.GetDCAAuthToken)).Methods("GET")
```

## Cross-references

| Topic | See also |
|-------|----------|
| `comp/core/ipc` — fx component that wraps this package's primitives into a lifecycle-managed component with `HTTPMiddleware`, `GetClient()`, and TLS configs | [`comp/core/ipc`](../comp/core/ipc.md) |
| `comp/api/api` — HTTP server that uses `ipc.HTTPMiddleware` (backed by `pkg/api/util.TokenValidator`) and the IPC cert from `cert.FetchOrCreateIPCCert` | [`comp/api/api`](../comp/api/api.md) |
| Credential scrubbing applied to flares and log output (separate concern, not a dependency) | [`pkg/util/scrubber`](util/scrubber.md) |

### Layer diagram

`pkg/api` is the lowest layer in the IPC security stack. Higher-level packages build on it:

```
pkg/api/security            ← raw file I/O: auth token (FetchOrCreateAuthToken),
pkg/api/security/cert            IPC cert (FetchOrCreateIPCCert, GetTLSConfigFromCert)
pkg/api/util                ← stateless helpers: TokenValidator, IsForbidden, DCA token cache
    │
    ▼
comp/core/ipc               ← fx lifecycle: reads/creates token + cert at startup,
                               exposes GetAuthToken(), GetTLSServerConfig(),
                               GetTLSClientConfig(), HTTPMiddleware, GetClient()
    │
    ▼
comp/api/api                ← starts CMD server (bearer token via ipc.HTTPMiddleware)
comp/api/grpcserver              and IPC server (mTLS via cert); multiplexes gRPC on CMD port
pkg/util/grpc               ← uses GetAuthToken() + GetTLSServerConfig() to build
                               gRPC server interceptors and client credentials
```

### Relationship to `pkg/util/scrubber`

`pkg/api` does not depend on `pkg/util/scrubber`. Scrubbing is applied at a higher layer — for example, `comp/metadata/inventoryagent` scrubs configuration snapshots before including them in inventory payloads, and `comp/core/flare` scrubs every file before adding it to an archive. Auth tokens and TLS key material are never included in those payloads, so no scrubbing is needed at the `pkg/api` level.
