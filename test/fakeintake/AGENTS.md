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
| `/api/v1/check_run` | CheckRunAggregator | `FilterCheckRuns()` |
| `/api/v2/logs` | LogAggregator | `FilterLogs()` |
| `/intake/` | EventAggregator | `FilterEvents()` |
| `/api/v0.2/traces` | TraceAggregator | `GetTraces()` |
| `/api/v0.2/stats` | APMStatsAggregator | `GetAPMStats()` |
| `/api/v1/collector` | ProcessAggregator | `GetProcesses()` |
| `/api/v1/connections` | ConnectionsAggregator | `GetConnections()` |
| `/api/v1/container` | ContainerAggregator | `GetContainers()` |
| `/api/v2/contimage` | ContainerImageAggregator | `GetContainerImages()` |
| `/api/v2/contlcycle` | ContainerLifecycleAggregator | `GetContainerLifecycleEvents()` |
| `/api/v2/sbom` | SBOMAggregator | `GetSBOMs()` |
| `/api/v2/orch` | OrchestratorAggregator | `GetOrchestratorResources()` |
| `/api/v2/ndmflow` | NDMFlowAggregator | GetNDMFlows() |
| `/api/v2/netpath` | NetpathAggregator | `GetLatestNetpathEvents()` |
| `/api/v2/agenthealth` | AgentHealthAggregator | GetAgentHealth() |
| `/support/flare` | Flare parser | `GetLatestFlare()` |

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

4. **Add tests** for the new aggregator and client methods

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
