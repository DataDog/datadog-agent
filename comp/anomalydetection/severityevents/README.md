# severityevents

Shared contract and implementation for anomaly scorer severity transitions
(`Low`/`Medium`/`High`), decoupled from the observer package so consumers
don't need to depend on the full `observer.Component` surface.

- `def/` — `Subscriber` interface, `SeverityEvent`, `SeverityLevel`, and the
  `SeverityEventsConfiguration`/`SeverityEventFilter`/`SeverityEventListener`
  types passed to `SubscribeSeverityEvents`.
- `impl/` — `Dispatcher`, the concrete push implementation: owns one
  listener and one fixed cooldown/filter state machine per subscription.
  `SeverityReader` is a pull-based alternative: it implements
  `SeverityEventListener` itself and subscribes via `Subscriber.SubscribeSeverityEvents`,
  keeping a lock-free `GetSeverity()` snapshot up to date for callers that
  want the latest severity level without owning a listener callback.

`Subscriber` has two methods: `SubscribeSeverityEvents(cfg, listener)` for
push consumers, and `SubscribeSeverityEventsReader(cfg)` — a convenience that
wires its own internal listener and returns a `SeverityEventsReaderSubscription`
(a ready `Reader` plus its `Unsubscribe` function) — for pull-only consumers.
Both take the same `SeverityEventsConfiguration` (filter/cooldown only; the
listener is passed separately) and create one dedicated `Dispatcher` per call.

The anomaly scorer (`comp/anomalydetection/observer/impl/anomaly_scorer.go`)
owns a plain list of `Dispatcher` instances and feeds each one the same raw
severity level on every tick; it does not implement subscription logic itself.
Each `SubscribeSeverityEvents` call creates one new dispatcher configured from
that subscription's filter/cooldown.

A subscription added mid-stream is bootstrapped before
`SubscribeSeverityEvents` returns. If the current level is `Medium` or `High`,
the first event is delivered as `Low -> current level`; if the current level is
already `Low`, no initial event is emitted.

Before the scorer knows its current level, a new dispatcher starts at `Low` by
default, so the first observed `Medium`/`High` level emits a real escalation
instead of being treated as a pure seed.

See `../AGENTS.md` for the full anomaly detection subsystem overview.
