# `comp/core/hostname` — Hostname Resolution

The hostname component resolves the agent's host identifier and monitors it for drift at runtime.

## Package layout

```
comp/core/hostname/
├── def/                  # Component interface, Data struct, constants (separate go module)
├── impl/                 # Canonical implementation: resolution, drift, caching
├── fx/                   # fx.Module() wiring impl.NewComponent into the fx graph
├── mock/                 # Test mock (//go:build test)
├── hostnameimpl/         # Legacy bundle: Module() delegates to fx/, NewHostnameService() for tests
├── hostnameinterface/    # Re-exports from def/ for backward compat (separate go module)
└── remotehostnameimpl/   # Agent-as-proxy: resolves hostname via IPC to a running agent
```

`pkg/util/hostname` is a thin backward-compat shim. Its `Get`/`GetWithProvider` functions
delegate directly to `comp/core/hostname/impl`. Prefer injecting `hostname.Component` or
calling `hostnameimpl.GetWithProviderFromConfig` for new code.

---

## Resolution: the provider waterfall

On the first call to `Get` or `GetWithProvider`, the implementation walks an ordered list of
**providers**. Each provider attempts to detect the hostname using a different strategy. The
first provider that succeeds and is marked `stopIfSuccessful` wins immediately. Providers
without `stopIfSuccessful` can contribute but are superseded by later higher-priority ones.

```
GetWithProvider(ctx)
        │
        ▼
 cache hit? ──yes──► return cached Data
        │
        no
        │
        ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Provider waterfall                               │
│                                                                     │
│  1. configuration      cfg: "hostname"          stopIfSuccessful ◄──┼── explicit user config
│  2. hostname_file      cfg: "hostname_file"     stopIfSuccessful    │
│  3. fargate            sidecar mode → ""        stopIfSuccessful    │
│  4. gce                GCE metadata API         stopIfSuccessful    │
│  5. azure              Azure metadata API       stopIfSuccessful    │
│                                                                     │
│  ── coupled providers (run unconditionally, last one wins) ──────   │
│                                                                     │
│  6. fqdn               system FQDN              (if hostname_fqdn)  │
│  7. container          kube → Docker → kubelet                      │
│  8. os                 os.Hostname()                                │
│  9. aws                EC2 instance ID (IMDSv2)                     │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
        │
        ▼
  warnAboutFQDN() ── advisory warning if FQDN differs
        │
        ▼
  save to cache + update expvar "hostname.provider"
        │
        ▼
  return Data{Hostname, Provider}
```

> **Key rule for coupled providers (6–9):** they receive `currentHostname` (what the previous
> provider set) and may override or skip it. For example, `fromOS` skips if a previous provider
> already found a hostname; `fromEC2` only fires on ECS or when the current hostname looks like
> an EC2 default.

### Provider details

| # | Name | Config key / trigger | Stops on success |
|---|------|----------------------|-----------------|
| 1 | `configuration` | `hostname` | yes |
| 2 | `hostnameFile` | `hostname_file` | yes |
| 3 | `fargate` | running as ECS/Fargate sidecar | yes (returns `""`) |
| 4 | `gce` | GCE metadata endpoint reachable | yes |
| 5 | `azure` | Azure metadata endpoint reachable | yes |
| 6 | `fqdn` | `hostname_fqdn: true` + OS usable | no |
| 7 | `container` | kube apiserver / Docker / kubelet | no |
| 8 | `os` | `os.Hostname()` (only if no prior result) | no |
| 9 | `aws` | ECS instance, default EC2 name, or `ec2_prioritize_instance_id_as_hostname` | no |

### Serverless

When built with `-tags serverless`, the entire waterfall is replaced by a stub that returns
an empty `Data{}`. There is no meaningful hostname in serverless environments.

---

## Caching

Results are stored in `pkg/util/cache` (an in-process TTL cache) under the key
`agent/hostname`. Subsequent calls return the cached value immediately without re-running
providers.

The drift service uses a separate key (`agent/hostname_check`) so drift comparisons always
read the original baseline and do not interfere with the main resolution cache.


### Outside the fx graph

For code that cannot use dependency injection, two standalone functions are available on
`comp/core/hostname/impl`:

```go
// Standard resolution (uses IMDSv2)
hostnameimpl.GetWithProviderFromConfig(ctx, cfg)

// Legacy resolution (EC2 IMDSv2 transition — skips IMDSv2/MDI)
hostnameimpl.GetWithLegacyResolutionProviderFromConfig(ctx, cfg)
```

`pkg/util/hostname.Get` / `GetWithProvider` are convenience wrappers that call these with
`pkgconfigsetup.Datadog()` as the config reader. They are deprecated for new callers.
