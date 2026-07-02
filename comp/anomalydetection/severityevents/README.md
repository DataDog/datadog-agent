# severityevents

Shared contract and implementation for anomaly scorer severity transitions
(`Low`/`Medium`/`High`), decoupled from the observer package so consumers
don't need to depend on the full `observer.Component` surface.

- `def/` — `Subscriber` interface, `SeverityEvent`, `SeverityLevel`, and the
  `SeverityEventsConfiguration`/`SeverityEventFilter`/`SeverityEventListener`
  types passed to `SubscribeScorer`.
- `impl/` — `Dispatcher`, the concrete push implementation: owns
  subscriptions, per-subscription cooldown/filter state, and delivery.

The anomaly scorer (`comp/anomalydetection/observer/impl/anomaly_scorer.go`)
owns a `Dispatcher` instance and feeds it a derived severity level on every
tick; it does not implement subscription logic itself.

A subscription added mid-stream is delivered a synthetic initial event
(`FromLevel == ToLevel`, `Direction == SeverityEventBoth`) before
`SubscribeScorer` returns, so late subscribers learn the current state
immediately instead of only future transitions.

See `../AGENTS.md` for the full anomaly detection subsystem overview.
