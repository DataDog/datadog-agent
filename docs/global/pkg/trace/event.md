# pkg/trace/event

## Purpose

Implements APM event extraction and sampling. APM events are individual spans extracted from a trace and fed into Trace Search (Datadog's analytics/indexing feature). Because they represent single spans rather than full trace trees, they can be retained at a much higher rate than full traces, enabling high-cardinality search over properties such as customer ID, HTTP endpoint, or status code.

When a trace survives sampling independently, the link between that trace and its extracted events is preserved throughout the pipeline.

## Key elements

### `Extractor` (interface, `extractor.go`)

```go
type Extractor interface {
    Extract(span *pb.Span, priority sampler.SamplingPriority) (rate float64, ok bool)
    ExtractV1(span *idx.InternalSpan, priority sampler.SamplingPriority) (rate float64, ok bool)
}
```

Decides whether a span should become an APM event and at what extraction rate. Returns `(rate, true)` if the span matches; `(0, false)` otherwise. Three implementations are provided:

| Constructor | Matching logic |
|---|---|
| `NewMetricBasedExtractor()` | Reads the extraction rate from a span metric set by the tracer (`_dd1.sr.eausr`). This is the preferred mechanism. |
| `NewFixedRateExtractor(rateByServiceAndName)` | Matches `(service, operation)` pairs to a configured rate map. Service/operation keys are compared case-insensitively. |
| `NewLegacyExtractor(rateByService)` | Matches only top-level spans by service name. Kept for backwards compatibility with older agent configurations. |

Extractors are tried in order; the first match stops the chain.

### `Processor` (`processor.go`)

```go
type Processor struct { ... }

func NewProcessor(extractors []Extractor, maxEPS float64, statsd statsd.ClientInterface) *Processor
func (p *Processor) Start()
func (p *Processor) Stop()
func (p *Processor) Process(pt *traceutil.ProcessedTrace) (numEvents, numExtracted int64, events []*pb.Span)
func (p *Processor) ProcessV1(pt *traceutil.ProcessedTraceV1) (numEvents, numExtracted int64, events []*idx.InternalSpan)
```

Orchestrates the full extraction + sampling pipeline for a single processed trace:

1. Iterates over every span in the trace chunk.
2. Runs each configured `Extractor` in order to obtain an extraction rate.
3. Probabilistically drops the span based on that rate (`SampleByRate`).
4. Applies a max-events-per-second (`maxEPS`) rate limiter to all remaining events that do not carry `PriorityUserKeep`. Events with `PriorityUserKeep` bypass the EPS cap.
5. Tags surviving events with the various sampling rates used (EPS rate, client rate, pre-sample rate, extraction rate) and marks them as analyzed spans.
6. Returns spans only when the parent trace was dropped (`DroppedTrace == true`); otherwise the span is already embedded in the kept trace.

### `sampler_max_eps.go`

Implements the token-bucket EPS rate limiter used internally by `Processor`. Runs in its own goroutine (started/stopped via `Start`/`Stop`).

## Usage

`pkg/trace/agent` creates the processor at startup via a helper:

```go
func newEventProcessor(conf *config.AgentConfig, statsd statsd.ClientInterface) *event.Processor {
    extractors := []event.Extractor{event.NewMetricBasedExtractor()}
    if len(conf.AnalyzedSpansByService) > 0 {
        extractors = append(extractors, event.NewFixedRateExtractor(conf.AnalyzedSpansByService))
    }
    if len(conf.AnalyzedRateByServiceLegacy) > 0 {
        extractors = append(extractors, event.NewLegacyExtractor(conf.AnalyzedRateByServiceLegacy))
    }
    return event.NewProcessor(extractors, conf.MaxEPS, statsd)
}
```

`agent.Agent` calls `EventProcessor.Process(pt)` for each `ProcessedTrace` and routes the resulting event spans to the writer pipeline.
