> **TL;DR:** A `confmap.Converter` that automatically enriches a user-supplied OpenTelemetry Collector configuration with Datadog-specific extensions, processors, and receivers before the Collector validates it.

# comp/otelcol/converter — OTel config converter component

## Purpose

`comp/otelcol/converter` is a `confmap.Converter` that automatically enhances
a user-supplied OpenTelemetry Collector configuration before the Collector
validates and applies it. It bridges the gap between a minimal OTel config
written by the user and the full set of Datadog-specific components that the
Agent expects to be present.

The converter runs during the OTel Collector's config resolution phase (after
providers load the raw config and before the Collector validates the result). It
is opt-in: activated by setting `otelcollector.converter.enabled: true` in
`datadog.yaml`.

## Key elements

### Key interfaces

#### Interface

```go
// comp/otelcol/converter/def
type Component interface {
    confmap.Converter  // Convert(ctx, *confmap.Conf) error
}
```

The standard `confmap.Converter` interface has a single method:

```go
Convert(ctx context.Context, conf *confmap.Conf) error
```

### Key types

#### Implementation: `ddConverter`

Located in `comp/otelcol/converter/impl/converter.go`.

```go
type ddConverter struct {
    coreConfig config.Component      // may be nil (e.g. in ocb builds)
    hostname   hostnameinterface.Component
    logger     *zap.Logger
}
```

`Convert` calls `enhanceConfig`, which applies a set of features controlled by
`otelcollector.converter.features` (a string slice). When `coreConfig` is nil
all features are assumed enabled (useful for OpenTelemetry Collector Builder
integration tests).

Default features: `infraattributes`, `prometheus`, `pprof`, `zpages`,
`health_check`, `ddflare`, `datadog`.

### Key functions

#### Enhancement operations

Each feature is idempotent: it is only applied if the relevant component or
extension is not already present in the user config.

#### Extensions (feature-gated)

| Feature name | Extension added | Purpose |
|-------------|-----------------|---------|
| `pprof` | `pprof/dd-autoconfigured` | Go pprof endpoint for the Collector process. |
| `zpages` | `zpages/dd-autoconfigured` | zPages debugging UI on `localhost:55679`. |
| `health_check` | `health_check/dd-autoconfigured` | Health check HTTP endpoint. |
| `ddflare` | `ddflare/dd-autoconfigured` | Integrates OTel config into Agent flare. |
| `datadog` | `datadog/dd-autoconfigured` | Datadog extension populated with `api_key`, `site`, `deployment_type`, and `hostname` from the core Agent config. |

All auto-added extensions are registered under the `dd-autoconfigured` suffix to
avoid collisions with user-defined instances of the same type.

#### Processors (`infraattributes` feature)

Adds the `infraattributes` processor to every pipeline that contains a Datadog
exporter. This enriches all signals with host-level infrastructure attributes
from the Tagger.

#### Receivers (`prometheus` feature)

Adds a `prometheus/dd-autoconfigured` receiver that scrapes the Collector's own
internal telemetry metrics endpoint (default `0.0.0.0:8888`, detected from
`service.telemetry.metrics.readers`). A synthetic metrics pipeline is created
for each Datadog exporter that does not already have such a receiver, ensuring
Collector health metrics are forwarded to Datadog. A `filter` processor
(`filter/drop-prometheus-internal-metrics`) is inserted to suppress
`scrape_*`, `up`, and `promhttp_*` metrics from that pipeline.

#### Core Agent config injection (`addCoreAgentConfig`)

- **API key / site**: If a `datadog` exporter is present and its `api.key` is
  empty (or uses a secret backend reference like `ENC[...]`), the key is
  backfilled from `api_key` in `datadog.yaml`. The `api.site` field is
  similarly defaulted to `datadoghq.com` if absent.
- **Profiler env**: If a `ddprofiling` extension is configured, the `env` field
  of `profiler_options` is populated from the Agent's `env` config key.

### Configuration and build flags

#### Constructors

| Function | When to use |
|----------|-------------|
| `NewConverterForAgent(Requires)` | Standard fx path — receives `config.Component` and `hostnameinterface.Component`. |
| `NewFactory()` | For use with the OpenTelemetry Collector Builder (`ocb`), where no Agent dependencies are available. |

### fx module

```go
// comp/otelcol/converter/fx
converterfx.Module()
// Provides: option.Option[converter.Component]
```

The module calls `fxutil.ProvideOptional[converter.Component]()` so consumers
can depend on `option.Option[converter.Component]` without a hard requirement.

## Usage

### Where it is used

| Consumer | How |
|----------|-----|
| `comp/otelcol/collector/impl` (OTel Agent) | Receives `confmap.Converter` in `Requires`. Wraps it in a `converterFactory` and passes it to `otelcol.ConfigProviderSettings.ResolverSettings.ConverterFactories` when `otelcollector.converter.enabled: true`. |
| `cmd/otel-agent/subcommands/run/command.go` | Adds `converterfx.Module()` to the fx app. |

### Activating the converter

```yaml
# datadog.yaml
otelcollector:
  enabled: true
  converter:
    enabled: true
    features:
      - infraattributes
      - prometheus
      - pprof
      - zpages
      - health_check
      - ddflare
      - datadog
```

Omitting `features` enables all features by default.

### Adding a new enhancement

1. Add a new feature name to the `enabledFeatures` check in `enhanceConfig`
   (`impl/autoconfigure.go`).
2. Implement the mutation against `*confmap.Conf` using the helpers
   `addComponentToConfig`, `addComponentToPipeline`, and
   `addExtensionToPipeline`.
3. Add the feature to the default list in the `coreConfig == nil` branch so
   that `ocb` integration tests exercise it.

## Related packages and components

| Package / Component | Doc | Relationship |
|---|---|---|
| `comp/otelcol/collector` | [collector.md](collector.md) | The primary consumer of this component. `impl/collector.go` wraps the converter in a `converterFactory` and passes it to `otelcol.ConfigProviderSettings.ResolverSettings.ConverterFactories` when `otelcollector.converter.enabled: true`. The converter runs before the Collector validates the resolved config. |
| `comp/otelcol/otlp` | [otlp.md](otlp.md) | The in-process OTLP ingestion pipeline for the core Agent. The converter is not used in this path (the `fx-pipeline` collector variant uses a hardcoded pipeline topology instead of a user-supplied config file), but understanding both components clarifies the distinction between the two OTel deployment modes. |
| `pkg/opentelemetry-mapping-go` | [../../pkg/opentelemetry-mapping-go.md](../../pkg/opentelemetry-mapping-go.md) | Provides the attribute and metric translation layer consumed by the `datadog` exporter and `infraattributes` processor that the converter injects. The `infraattributes` feature added by the converter enriches signals with infrastructure tags before they reach the exporter, which then uses `attributes.Translator` to map OTel resource attributes to Datadog hostnames and tags. |
| `comp/core/config` | [../core/config.md](../core/config.md) | Injected as `coreConfig config.Component` into `ddConverter`. The converter reads `api_key`, `site`, `env`, and `otelcollector.converter.*` keys from this component. When `coreConfig` is nil (OCB standalone mode), all features are enabled by default without consulting the config. |
