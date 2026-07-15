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

`NewScraperFromYAML` accepts generic `openmetrics` instance YAML and returns an
unsupported-config error for options that still require the Python
implementation. The public wrapper keeps the configuration surface data-driven
so future core checks can share the parser, scraper, and compatibility tests.

## Validation Map

```mermaid
flowchart LR
    Upstream[integrations-core<br/>OpenMetrics tests] --> Ledger[Migration inventory]
    Ledger --> Unit[Focused Go behavior tests]
    Fixtures[upstream benchmark payloads] --> Smoke[Go fixture smoke test]
    Fixtures --> Bench[Go parser benchmarks]
    Kind[Kind e2e] --> Default[Python default loader]
    Kind --> Core[Go core loader + Python fallback]
    Unit --> CI[new-e2e-amp / unit CI]
    Smoke --> CI
    Default --> CI
    Core --> CI
```

The migration inventory is bookkeeping, not executable proof of one-to-one
assertions. Behavioral confidence comes from the focused Go unit tests and the
Python-default/core-loader fakeintake e2e suites. Python-only constructor,
subclass, decorator, and method-mutation seams are excluded from the inventory.
