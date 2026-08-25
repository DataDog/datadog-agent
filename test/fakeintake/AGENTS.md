# Fakeintake

## Overview

Fakeintake is a mock Datadog intake server used by E2E tests. It captures all
payloads the agent sends (metrics, logs, traces, check runs, etc.) and exposes
them via a query API so tests can assert on agent output.

## Structure

```
test/fakeintake/
├── api/              # Shared types (Payload, ResponseOverride)
├── aggregator/       # Payload parsers — one per Datadog endpoint
├── client/           # Go client library used by E2E tests
├── server/           # HTTP server + in-memory store
│   └── serverstore/  # Storage layer and payload parsers
└── cmd/
    ├── server/       # Server binary entry point
    └── client/       # CLI tool (fakeintakectl)
```

## Supported endpoints

| Route | Aggregator | Client method |
|-------|-----------|---------------|
| `/api/v2/series` | MetricAggregator | `FilterMetrics()` |
| `/api/beta/sketches` | SketchAggregator | `FilterSketches()` |
| `/api/v1/check_run` | CheckRunAggregator | `FilterCheckRuns()` |
| `/api/v2/logs` | LogAggregator | `FilterLogs()` |
| `/api/v2/compliance` | (raw store) | `GetComplianceFindings()` |
| `/intake/` | EventAggregator | `FilterEvents()` |
| `/api/v0.2/traces` | TraceAggregator | `GetTraces()` |
| `/api/v0.2/stats` | APMStatsAggregator | `GetAPMStats()` |
| `/api/v1/collector` | ProcessAggregator | `GetProcesses()` |
| `/api/v1/connections` | ConnectionsAggregator | `GetConnections()` |
| `/api/v1/container` | ContainerAggregator | `GetContainers()` |
| `/api/v2/agentdiscovery` | AgentDiscoveryAggregator | `GetAgentDiscoveryPayloads()` |
| `/api/v2/contimage` | ContainerImageAggregator | `GetContainerImageNames()` / `FilterContainerImages()` |
| `/api/v2/contlcycle` | ContainerLifecycleAggregator | `GetContainerLifecycleEvents()` |
| `/api/v2/sbom` | SBOMAggregator | `GetSBOMIDs()` / `FilterSBOMs()` |
| `/api/v2/orch` | OrchestratorAggregator | `GetOrchestratorResources()` |
| `/api/v2/ndmflow` | NDMFlowAggregator | GetNDMFlows() |
| `/api/v2/netpath` | NetpathAggregator | `GetLatestNetpathEvents()` |
| `/api/v2/agenthealth` | AgentHealthAggregator | GetAgentHealth() |
| `/support/flare` | Flare parser | `GetLatestFlare()` |
| `/api/v0.1/configurations` | (TUF-signed RC) | `RCStats()` (poll counter) |
| `/api/v0.1/org` | (Remote Config) | — |
| `/api/v0.1/status` | (Remote Config) | — |
| `/api/unstable/on_prem_runners` | PAR enrollment | `GetPAREnrollmentCount()` |
| `/api/v2/on-prem-management-service/workflow-tasks/dequeue` | PAR task queue | `EnqueuePARTask()` / `EnqueueSignedPARTask()` |
| `/api/v2/on-prem-management-service/workflow-tasks/publish-task-update` | PAR task results | `GetPARTaskResult()` |

## Client usage

```go
import (
    "github.com/DataDog/datadog-agent/test/fakeintake/client"
    "github.com/DataDog/datadog-agent/test/fakeintake/aggregator"
)

fakeintake := s.Env().FakeIntake.Client()

// Filter metrics by name + tags
metrics, err := fakeintake.FilterMetrics("system.cpu.user",
    client.WithTags[*aggregator.MetricSeries]([]string{"env:prod"}),
    client.WithMetricValueHigherThan(0),
)

// Filter logs by service
logs, err := fakeintake.FilterLogs("myservice",
    client.WithMessageContaining("error"),
)

// Filter check runs
checks, err := fakeintake.FilterCheckRuns("ntp.in_sync")

// Reset state between tests
fakeintake.FlushServerAndResetAggregators()

// Debug: list all received metric names
names, _ := fakeintake.GetMetricNames()
```

## Private Action Runner

Fakeintake simulates the OPMS endpoints used by the Private Action Runner and
provides control endpoints for tests:

| Route | Purpose |
|-------|---------|
| `POST /api/unstable/on_prem_runners` | PAR self-enrolls and receives a runner ID |
| `POST /api/unstable/on_prem_runners/api_key_only` | PAR self-enrolls with only an API key |
| `POST /api/v2/on-prem-management-service/workflow-tasks/dequeue` | PAR dequeues a task |
| `POST /api/v2/on-prem-management-service/workflow-tasks/publish-task-update` | PAR publishes a result |
| `POST /api/v2/on-prem-management-service/workflow-tasks/heartbeat` | PAR sends a task heartbeat |
| `GET /api/v2/on-prem-management-service/runner/health-check` | PAR checks OPMS health |
| `POST /fakeintake/par/enqueue` | Enqueue a task from a test |
| `GET /fakeintake/par/result` | Read a task result |
| `POST /fakeintake/par/signing-key` | Register the signing identity used for dequeued tasks |
| `POST /fakeintake/par/flush` | Clear queued tasks and results |
| `GET /fakeintake/par/stats` | Read the dequeue, health-check, and enrollment counts |

By default, dequeued tasks have unsigned envelopes. To exercise real task
verification, first push the matching public key through the `AP_RUNNER_KEYS`
Remote Config product, then register the private key with fakeintake:

```go
err := fakeintake.SetPARSigningKey(keyID, privateKey, orgID, runnerID, connectionID)
```

The key ID and private key must match the public key delivered to PAR. Calls to
`EnqueuePARTask` after registration return signed envelopes that PAR can verify.

## Remote Config

Fakeintake can stand in for the Datadog Remote Config backend so the agent
applies test-defined configs end-to-end (TUF-signed). Disabled by default.

Enable on the server:

```
fakeintake-server --remoteconfig \
    [--rc-key-path /path/to/signing.key] \
    [--rc-state preload.yaml] \
    [--rc-version 5]
```

Routes added when enabled:

| Route | Purpose |
|-------|---------|
| `POST /api/v0.1/configurations` | Agent polls for TUF-signed configs |
| `GET  /api/v0.1/org` | Returns org UUID |
| `GET  /api/v0.1/status` | Reports RC enabled/authorized |
| `POST /fakeintake/rc/config` | Push/replace a config (control) |
| `POST /fakeintake/rc/expiration` | Change non-root TUF expiry for expiration tests |
| `GET  /fakeintake/rc/configs` | List stored configs (control) |
| `DELETE /fakeintake/rc/config/<key>` | Delete a config (control) |
| `GET  /fakeintake/rc/stats` | Poll counter, version, signing key info |

Point the agent at fakeintake by setting `remote_configuration.rc_dd_url`
plus the `config_root` / `director_root` printed at fakeintake startup.

Push configs from a test:

```go
err := fakeintake.RCAddConfig("42", "METRIC_CONTROL", "abc", "filterlist",
    []byte(`{"blocked_metrics":{"by_name":{"values":[{"metric_name":"foo"}]}}}`))

// Publish a short-lived authoritative snapshot, then restore a fresh horizon
// after the Agent has accepted it and observed its expiration.
err = fakeintake.RCSetExpiration(time.Now().Add(30 * time.Second))
err = fakeintake.RCSetExpiration(time.Now().Add(24 * time.Hour))
```

Or via CLI:

```
fakeintakectl --url http://localhost:80 rc add \
    --product METRIC_CONTROL --config-id abc --config-name filterlist \
    --data @config.json
fakeintakectl --url ... rc list
fakeintakectl --url ... rc stats --watch
fakeintakectl --url ... rc delete 42/METRIC_CONTROL/abc/filterlist
```

The signing key is persisted at `~/.fakeintake/signing.key` by default — it must
match the agent's stored `remote-config.db`. Generating a new key requires
flushing that DB.

## CLI usage (`fakeintakectl`)

Use `fakeintakectl` (built from `cmd/client/`) for non-Go callers — Python
integration tests, shell debugging, ad-hoc queries. **Prefer this over hitting
`/fakeintake/payloads?endpoint=...&format=json` directly**: the CLI gives you
typed JSON output (the parsed `MetricSeries` / `Sketch` slices, not raw
payloads), filter-by-name and filter-by-tags built in, and tracks the same
aggregator surface the Go client exposes.

```bash
# Build server + CLI (artifacts land in test/fakeintake/build/):
dda inv fakeintake.build

FAKEINTAKECTL=test/fakeintake/build/fakeintakectl

# Filter series at /api/v2/series (counts/gauges/rates) — JSON list of
# aggregator.MetricSeries, each with .metric, .tags, .points[].value/.timestamp,
# .type (1=COUNT, 2=RATE, 3=GAUGE).
$FAKEINTAKECTL --url http://127.0.0.1:8080 \
    filter metrics --name my.count.metric --tags env:prod

# Filter sketches at /api/beta/sketches (distributions) — JSON list of
# aggregator.Sketch, each with .metric, .tags, .dogsketches[].sum/.cnt/.min/...
$FAKEINTAKECTL --url http://127.0.0.1:8080 \
    filter sketches --name my.distribution

# Discovery: what did the agent actually send?
$FAKEINTAKECTL --url http://127.0.0.1:8080 get metric names
$FAKEINTAKECTL --url http://127.0.0.1:8080 get sketch names
$FAKEINTAKECTL --url http://127.0.0.1:8080 route-stats

# Reset state between assertions:
$FAKEINTAKECTL --url http://127.0.0.1:8080 flush
```

**`filter <type>` exits non-zero with no matches** — wrap in shell with
`|| true` or treat empty/`null` stdout as no matches in callers.

**Stale-binary trap:** the CLI is a thin wrapper around `client/client.go`.
Adding a new endpoint without rebuilding means the new subcommand silently
isn't there — `filter sketches` may even be missing from older builds.
Always run `dda inv fakeintake.build` after pulling.


## Adding a new payload type

When the agent starts sending a new type of data to a new endpoint:

1. **Create an aggregator** in `aggregator/<type>Aggregator.go`:
   - Define a struct implementing the `PayloadItem` interface (`name()`,
     `GetTags()`, `GetCollectedTime()`)
   - Write a `Parse<Type>(payload api.Payload) ([]*<Type>, error)` function
   - Create a `<Type>Aggregator` wrapping `Aggregator[*<Type>]`

2. **Register the parser** in `server/serverstore/parser.go`:
   - Add the route → parser mapping to `parserMap`

3. **Add client methods** in `client/client.go`:
   - Add the aggregator field to the `Client` struct
   - Create `get<Type>()` to fetch from the endpoint
   - Create public `Filter<Type>()` / `Get<Type>()` methods
   - Add filter options (`MatchOpt[*<Type>]`) as needed

4. **Register CLI subcommand(s)** in `cmd/client/cmd/`:
   - Add `filter<type>.go` and/or `get<type>.go` wired into `filter.go` /
     `get.go` so `fakeintakectl filter <type>` and `fakeintakectl get <type>`
     work for shell / Python callers
   - Without this step, non-Go callers have to fall back to raw
     `/fakeintake/payloads`

5. **Add tests** for the new aggregator and client methods

## Image version pinning

The fakeintake Docker image consumed by e2e tests is pinned, not `:latest`:

- `version/VERSION` holds the pinned tag (e.g. `v1`), embedded by
  `version/version.go` and exposed as `version.Tag` (this package only holds the
  tag — it has no knowledge of overrides).
- The e2e-framework helper `components/datadog/fakeintake.ImageURL(image)` (used
  by every fakeintake default) returns the `FakeintakeImageOverride` runner
  parameter (`E2E_FAKEINTAKE_IMAGE_OVERRIDE`) if set, else `<image>:<version.Tag>`.
  It reads the override through the runner parameter store like every other
  `E2E_*` value — never `os.Getenv` directly.
- **Only server changes rebuild the image.** The image is
  `go build cmd/server/main.go`, whose in-module deps are `server/`,
  `aggregator/` and `api/`. So a bump/rebuild/publish is required only for
  `.go` changes under those (plus `go.mod`/`go.sum`/`Dockerfile`) — see
  `.fakeintake_server_paths` in `.gitlab-ci.yml` and `_is_server_file()` in
  `tasks/fakeintake.py`. Changes to `client/`, `cmd/client/` or `docs/` do **not**
  change the image and need no bump, and neither do non-Go files under the server
  paths (`BUILD.bazel`, test fixtures).
- **When you change server-side fakeintake code**, **bump `version/VERSION` in
  the same PR** — a strictly greater integer than the base branch's value (e.g.
  `v1` → `v2`). CI (`fakeintake_check_version_bump`, using
  `dda inv fakeintake.check-version-bump`) enforces this, including in the
  merge queue, so two PRs bumping to the same value can never collide.
- **On your PR**, e2e suites don't need the bump to see a server change: CI sets
  `E2E_FAKEINTAKE_IMAGE_OVERRIDE` to the freshly built `v<sha>` image for server
  changes, and every suite honors that override globally. A client/CLI change
  runs e2e against the pinned image (no override, no rebuild) so it is still
  exercised.
- **On merge to main**, `publish_fakeintake_pinned` publishes the image under
  the tag in `VERSION`. The pinned tag is a release artifact — it is published
  authoritatively from `main` only, never from feature branches. Other branches
  keep using their own pinned tag until they rebase onto a main that bumped it —
  no branch is affected by a fakeintake change until it explicitly picks up the
  new pin. On the main pipeline, e2e waits for `publish_fakeintake_pinned` (via
  the optional need in `.needs_fakeintake_publish`) so it never runs against a
  not-yet-published tag.
- **Known limitation — cross-pipeline publish window.** Because the pinned tag
  is published only after the bump merges to main, there is a window (the main
  pipeline's fakeintake build + publish, up to ~10-20 min) during which the new
  `vN` does not yet exist. A *different* PR that rebases onto the just-bumped
  main during this window resolves to `vN` and its e2e will fail to pull the
  image until the publish completes. This is rare (fakeintake changes are
  infrequent) and self-healing: re-run the affected e2e once main has finished
  publishing. Eliminating it entirely would require publishing at the
  merge-queue gate; that trade-off was intentionally declined in favor of a
  simpler main-only publish.
- `:latest` is still published (`publish_fakeintake_latest`) for external/manual
  consumers, but no test references it anymore.

## Key files

- `client/client.go` — main client API (filter methods, match options)
- `aggregator/common.go` — generic `Aggregator[T]` base (compression, storage)
- `server/server.go` — HTTP handler routing
- `server/serverstore/in_memory.go` — payload storage
- `api/api.go` — shared types (`Payload`, `ResponseOverride`)

## Keeping this file accurate

This file is part of the `AGENTS.md` hierarchy (see root `AGENTS.md` §
"Keeping AI context accurate"). Update it when endpoints, client methods, or
extension patterns change. AI agents should fix inaccuracies they encounter
during tasks.
