# Datadog Remote Config extension (development POC)

`datadog_remoteconfig` forwards the existing agentless Remote Config protocol through
DDOT. It runs inside the Collector, including standalone mode
(`DD_OTEL_STANDALONE=true`), without a companion Datadog Agent.

The SDK owns polling, signed configuration validation, apply status and sampling.
The backend owns customer and adaptive policies. This extension has no config cache,
sampler, Agent dependencies or `/v0.7/config` compatibility server.

## Configuration

Add the extension to your existing Collector configuration; keep OTLP telemetry on
the existing receiver/exporter pipelines. This example requires a server certificate
whose hostname matches the SDK's proxy address:

```yaml
extensions:
  datadog_remoteconfig:
    endpoint: 127.0.0.1:8234
    tls:
      cert_file: /path/to/collector.crt
      key_file: /path/to/collector.key
    egress:
      endpoint: https://config.datadoghq.com
      timeout: 10s
    max_request_body_size: 4194304

service:
  extensions: [datadog_remoteconfig]
```

Merge `service.extensions` with any extensions already enabled. The extension is
registered with DDOT but is not enabled in its default configuration.

| Setting | Default | Meaning |
| --- | --- | --- |
| `endpoint` | `127.0.0.1:8234` | TCP listener address. |
| `tls` | unset | Collector TLS server settings; configure for an HTTPS SDK endpoint. |
| `egress.endpoint` | `https://config.datadoghq.com` | Fixed HTTPS upstream origin. Select the same Datadog site/org as telemetry export. |
| `egress.timeout` | `10s` | Upstream request deadline, including reading the response. Must be positive. |
| `egress.proxy_url` | unset | Explicit HTTP/HTTPS network proxy; otherwise standard proxy environment settings apply. |
| `max_request_body_size` | `4194304` | Maximum request bytes buffered before forwarding. Must be positive. |

`egress.tls` supports the Collector's TLS client configuration, including CA
certificates. Normal TLS verification is enabled by default. The extension owns
its HTTP transport and closes upstream idle connections during shutdown.
Without `tls`, the local listener uses HTTP; the existing agentless client requires
HTTPS, so the POC SDK transport override must use a TLS listener.

## Forwarding contract

| Method | Path |
| --- | --- |
| GET | `/api/v0.1/org` |
| POST | `/api/v0.1/configurations` |

Request and response bodies are opaque bytes, including protobuf and compressed
payloads. The proxy preserves application headers, including the SDK's real
`DD-API-KEY`, and upstream status codes, retry headers and redirects. Standard
hop-by-hop and untrusted forwarding headers are removed. The proxy does not follow
upstream redirects or retry requests.

Other paths return 404, unsupported methods return 405, query parameters return 400,
and oversized requests return 413. Transport failures return a generic 502 and
upstream deadlines return 504, without exposing transport errors or credentials.
Shutdown drains active requests until the Collector's shutdown context expires.

The `datadog_remoteconfig.requests` counter records allowed routes and HTTP response
status; it includes upstream HTTP errors and local rejection/transport errors on
those routes. Payloads and API keys are never logged.

## Remaining work for an end-to-end Python POC

Python/libdatadog currently construct `config.<DD_SITE>` internally. An independent
RC transport endpoint override is required; changing the OTLP endpoint or
`DD_TRACE_AGENT_URL` alone does not redirect agentless RC. Keep logical site and
trust-root selection separate from the proxy hostname, and preserve signature and
organization verification. No SDK endpoint flag is introduced by this PR.

Initially the application retains its real API key. Keyless applications require a
later change to SDK agentless initialization as well as Collector credential loading
and injection. This extension does not introduce tenant routing.

Validate user-configured rates before adaptive sampling: change a service-scoped
rate from 100% to 25% to 50%, verify apply acknowledgements and exported traces
without a restart, then test rule removal and proxy outages. Verify application
egress restrictions force both RC endpoints and OTLP through DDOT.

Adaptive eligibility/backend Agent-host checks and correct APM statistics from
head-sampled traces are separate acceptance gates. A synthetic dynamic policy or
successful forwarding alone does not prove an adaptive feedback loop.
