# severityevents

Shared contract and implementation for anomaly scorer severity transitions
(`Low`/`Medium`/`High`), decoupled from the observer package so consumers
don't need to depend on the full `observer.Component` surface.

- `def/` — `Subscriber` interface, `SeverityEvent`, `SeverityLevel`, and the
  `SeverityEventsConfiguration`/`SeverityEventFilter`/`SeverityEventListener`
  types passed to `SubscribeSeverityEvents`.
- `impl/` — `Dispatcher`, the concrete push implementation: owns one listener,
  one fixed cooldown/filter state machine, and delivery.

The anomaly scorer (`comp/anomalydetection/observer/impl/anomaly_scorer.go`)
owns a plain list of `Dispatcher` instances and feeds each one the same raw
severity level on every tick; it does not implement subscription logic itself.
Each `SubscribeSeverityEvents` call creates one new dispatcher configured from
that subscription's filter/cooldown.

A subscription added mid-stream is delivered a synthetic initial event
(`FromLevel == ToLevel`, `Direction == SeverityEventBoth`) before
`SubscribeSeverityEvents` returns, so late subscribers learn the current state
immediately instead of only future transitions.

Before the scorer knows its current level, a new dispatcher starts at `Low` by
default, so the first observed `Medium`/`High` level emits a real escalation
instead of being treated as a pure seed.

See `../AGENTS.md` for the full anomaly detection subsystem overview.
