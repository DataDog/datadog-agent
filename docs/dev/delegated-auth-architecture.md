# Delegated Authentication in the Agent: Architecture and Path Forward

## Background: how the agent authenticates today

Before delegated auth, every telemetry signal the agent sends carries a static API key. The operator puts the key in `api_key` (or `DD_API_KEY`), and the agent stamps it as `DD-Api-Key` on every outbound request. For multi-org shipping, the operator adds `additional_endpoints` — a map of domain to API keys — and each endpoint gets its own key from the config.

```
  datadog.yaml                         Agent runtime
  ────────────                         ─────────────

  api_key: <key>                       Each consumer reads its API key(s)
                                         from config at startup
  additional_endpoints:
    trace.datadoghq.com:               Forwarder ───► DD-Api-Key: <key>
      - <key1>                         Trace writers ───► DD-Api-Key: <key1>
      - <key2>                         Logs agent ───► DD-Api-Key: <key2>

  logs_config.additional_endpoints:
    - api_key: <key3>                  Process ───► DD-Api-Key: <key>
      host: ...                        Orchestrator ───► DD-Api-Key: <key>

  apm_config.additional_endpoints:
    trace.datadoghq.com:               Trace proxies ───► DD-Api-Key: <key>
      - <key4>
```

The key is a static string in the config file, but it can change at runtime through fleet automation, remote config, or OpAMP — all of which can push config updates (including API key rotations) to a running agent without a restart. When that happens, the config tree updates and consumers pick up the new key via their existing `OnUpdate` callbacks. What doesn't exist is a credential that resolves *asynchronously* — one that starts as a placeholder and becomes a real key later, after a cloud exchange completes. That asynchronous resolution is what delegated auth needs, and it doesn't fit the static-key model.

This works fine when the operator has a key. It doesn't work when the operator wants the agent to use a cloud identity (IAM role, workload identity) instead — that identity has to be exchanged for a Datadog API key at runtime, and the key may rotate on its own schedule.

### WIF without dual shipping

Workload identity federation (WIF) already works in the agent for single-org use. The operator sets `delegated_auth.org_uuid` alongside any `api_key` config prefix — global, `logs_config`, `remote_configuration`, `evp_proxy_config`, `ol_proxy_config` — and the delegated auth component exchanges a cloud auth proof for a Datadog API key at startup. The resolved key is written back into that same `api_key` config slot before other components initialize, so consumers pick it up as if the operator had set it directly.

```
datadog.yaml                         Agent runtime
────────────                         ─────────────

api_key: <will be replaced>           delegatedauth.AddInstance(org, provider)
delegated_auth:                          │
  org_uuid: <org>                        ▼
  provider: aws                       exchange AWS proof → API key
                                          │
                                          ▼
                                      write key back into api_key
                                      (SourceSecret, before consumers start)
                                          │
                                          ▼
                                      consumers read api_key at startup
                                      as if the operator set it directly
```

This works because there is exactly one `api_key` per config prefix — the resolved key replaces the placeholder, and every consumer reads that slot. But it does not work for `additional_endpoints`, which is a map of domain to *multiple* keys. A DELA directive in that list can't be written back to one slot without overwriting the others, and each directive may target a different org with a different cloud identity. That is the gap these PRs fill.

## The problem with the first delegated auth attempt

The first implementation ([#53517](https://github.com/DataDog/datadog-agent/pull/53517) + [#54803](https://github.com/DataDog/datadog-agent/pull/54803)) wrote the resolved API key back into the config tree and relied on `OnUpdate` callbacks to propagate it to consumers. This had several issues:

```
  DELA(org, aws) in config
        │
        ▼
  delegatedauth.AddInstance → exchange auth proof → API key
        │
        ▼
  write key back into the SAME config slot (SourceSecret)
        │
        ▼
  config OnUpdate fires → each consumer's callback picks up the new key
```

- **Every consumer needed its own config watcher.** The forwarder, logs, process, orchestrator, and trace agent each implemented an `OnUpdate` callback to re-read the config slot and rebuild its endpoint list — 38 config settings across 12 map-shape and 26 list-shape keys, each with consumer-specific reload logic.
- **The resolved key entered the config tree.** Writing at `SourceSecret` meant the key lived in the config object in memory — a wider exposure surface than necessary.
- **Some consumers couldn't reload at all.** Consumers that snapshot endpoints at construction (e.g. `apm_config.telemetry.additional_endpoints`) had no rebuild path, so a key resolving there needed an agent restart.
- **Each consumer had to know about `DELA(...)`.** The directive text sat in the config slot until the exchange completed, so every consumer needed logic to avoid shipping it as a literal API key. That logic was duplicated across five subsystems.
- **Logs endpoints that started pending stayed on the best-effort path** for the process lifetime. The pipeline partitioned reliable vs. unreliable destinations once at startup, so a pending endpoint that later resolved could not be promoted back to reliable.

## The new approach: Provider interface

The new approach replaces write-back with a `Provider` interface. The resolved key stays in a provider registry — it never enters the config tree. Consumers call `Authorize` on every request; the provider stamps the credential or returns `false` to signal "buffer and retry."

```
  Previous: config write-back              New: Provider interface
  ──────────────────────────              ────────────────────────

  DELA(org, aws) in config                 DELA(org, aws) in config
         │                                          │
         ▼                                          ▼
  exchange → API key                        exchange → API key
         │                                          │
         ▼                                          ▼
  write into config slot                    register Provider in registry
  (SourceSecret)                            (key never enters config)
         │                                          │
         ▼                                          ▼
  OnUpdate callback fires                   consumer calls Authorize(h)
  each consumer rebuilds                     on every request — lock-free
  its endpoint list                          buffer until resolved, then send
```

Key differences: no per-consumer config watcher, no `DELA(...)` filtering, no reload logic, and the resolved key never enters the config tree. The credential is read on the request path via an atomic load, so it works even for consumers that can't reload.

## The Provider interface

```go
type Provider interface {
    Authorize(h http.Header) bool
}
```

`Authorize` returns `true` when a credential is available (stamping it onto headers), `false` when not yet resolved (caller buffers). It never means "send unauthenticated." The credential is held in an `atomic.Pointer` — lock-free on the request path.

```
                 ┌──────────────────────────────────────────┐
start ──────────▶│ resolving        Authorize → false        │  caller buffers
                 └───────────┬──────────────────┬───────────┘
         exchange succeeded  │                  │  exchange failed
                             ▼                  ▼
        ┌────────────────────────────┐   ┌──────────────────────────────────┐
        │ resolved                   │   │ fallback configured?              │
        │ Authorize → true (real)    │   │  yes → Authorize → true (static)  │
        └────────────────────────────┘   │  no  → Authorize → false          │
                     ▲                   └──────────────┬───────────────────┘
                     └──────────────────────────────────┘
                            a later refresh succeeds
```

A fallback is only used after an exchange has actually failed. While the first exchange is in flight, the provider reports "not yet," so callers hold payloads rather than shipping under a safety-net key.

## The PRs

```
PR1: #55479  foundation (base: main)
 │    Provider interface, DELA directive discovery, config model
 │
 └──► PR2: #55480  consumers (base: PR1)
      │    Forwarder, trace writers, logs agent wiring
      │
      ├──► PR3: #55481  otel wiring (base: PR2)
      ├──► #55564  process + orchestrator (base: PR2)
      └──► #55565  trace proxies (base: PR2)
```

**PR1** adds the `Provider` interface, the `instanceProvider` (managing the buffering → resolved → fallback lifecycle via atomic swaps), DELA directive parsing across three config key shapes (`additional_endpoints`, `apm_config.additional_endpoints`, `logs_config.additional_endpoints`), and `SkipConfigWriteback` mode. No consumers are wired yet.

**PR2** wires the forwarder, trace writers, and logs agent. Each calls `ProvidersFor` at startup and `Authorize` per request, buffering until the credential resolves. Falls back to static API keys for endpoints without a DELA directive.

**PR3** replaces the noop delegated auth component in the OTel agent with the real one, so DELA directives are discovered during config loading.

## Coverage

| Subsystem | Previous (#54803) | Provider PR |
|---|---|---|
| Forwarder | Pending-key + OnUpdate | #55480 |
| Trace writers | DELA skip + reload | #55480 |
| Logs agent | Pending-key + unreliable | #55480 |
| OTel sync forwarder | Not covered | #55481 |
| Process + orchestrator | Pending-key | #55564 |
| Trace proxies (4 paths) | DELA skip | #55565 |

The follow-up PRs require less code per subsystem — the provider approach drops per-subsystem directive filtering, pending-state handling, and reload logic.

## Locking

Three mechanisms, deliberately split:

- **`mu` (RWMutex)** — guards instances/providers maps. Network I/O (IMDS, auth exchange) happens outside the lock to avoid blocking.
- **`additionalEndpointsMu` (Mutex)** — serializes config writes. Separate from `mu` because config writes trigger `OnUpdate` callbacks that could call back into the component and deadlock.
- **`atomic.Pointer`** — the provider's credential. Lock-free on the request path (`Authorize` does a single atomic load).

## Future work

**Dynamic ENC[] resolution.** Today `fallback=ENC[...]` is resolved once at config-load time. The provider path could be extended to re-resolve secrets on each refresh cycle, unifying secret resolution and delegated auth under one mechanism.

**Token-based auth, not just API keys.** `Authorize` sets whatever header the provider chooses. A future provider could stamp `Authorization: Bearer ...` instead of `DD-Api-Key`, enabling stateless token auth without changing consumer code.

**Other:** non-AWS providers (GCP, Azure — the parser already supports new provider names), MRF support for trace writers, and config reload without restart (the component supports replacing instances, but discovery only runs during `LoadDatadog`).
