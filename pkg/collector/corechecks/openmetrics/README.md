# OpenMetrics Core Check

This package contains the Go implementation of the generic `openmetrics` check.
The migration is Agent-gated by `openmetrics.use_core_loader`, which defaults to
`false`.

## Loader Path

```mermaid
flowchart TD
    AD[Autodiscovery config<br/>check name: openmetrics] --> Loader[collector loader]
    Loader --> Flag{openmetrics.use_core_loader}
    Flag -->|false| Python[Python openmetrics check]
    Flag -->|true| Core[Go openmetrics core check]
    Core --> Parse[parse instance YAML]
    Parse --> Unsupported{unsupported Python-only option}
    Unsupported -->|yes| Skip[ErrSkipCheckInstance]
    Skip --> Python
    Unsupported -->|no| Scraper[reusable OpenMetrics scraper]
    Scraper --> Sender[Datadog sender]
```

The Agent-level flag is intentionally separate from OpenMetrics instance
configuration. Users do not need to edit pod annotations or other AD templates
to participate in the migration.

## Reusable Scraper

Core checks that expose a Prometheus or OpenMetrics endpoint can reuse the
generic scraper instead of reimplementing config parsing, HTTP, label handling,
metric transformers, and metadata submission.

```go
scraper, err := openmetrics.NewScraperFromYAML(instanceYAML, string(c.ID()))
if err != nil {
    if openmetrics.IsUnsupportedConfig(err) {
        return fmt.Errorf("%w: %v", check.ErrSkipCheckInstance, err)
    }
    return err
}

return scraper.Scrape(sender)
```

`NewScraperFromYAML` accepts the same instance YAML as the generic
`openmetrics` integration. The public wrapper deliberately keeps the
configuration surface data-driven, so future core checks share the same parity
suite and behavior.

## Validation Map

```mermaid
flowchart LR
    Upstream[integrations-core<br/>OpenMetrics tests] --> Ledger[Go parity ledger]
    Ledger --> Unit[package unit tests]
    Fixtures[upstream benchmark payloads] --> Smoke[fixture smoke test]
    Fixtures --> Bench[Go vs Python benchmark]
    Kind[Kind e2e] --> Default[Python default loader]
    Kind --> Core[Go core loader + Python fallback]
    Unit --> CI[new-e2e-amp / unit CI]
    Smoke --> CI
    Default --> CI
    Core --> CI
```

The parity ledger only tracks tests applicable to the generic check behavior.
Python-only constructor, subclass, decorator, and method-mutation seams are
excluded from that ledger rather than counted as skips.
