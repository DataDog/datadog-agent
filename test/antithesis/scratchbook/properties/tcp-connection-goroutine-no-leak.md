# tcp-connection-goroutine-no-leak

**Added:** evaluation gap-fill, B-TRANSPORT decision (2026-05-29).

## What led here

The SUT-discovery failure-modes pass and the coverage-balance lens (GAP-2) flagged
two TCP connection-management leak surfaces with no property coverage:

1. **`handleServerClose` goroutine** (`comp/logs-library/client/tcp/connection_manager.go`,
   ~`:125`): spawned per connection to watch for server-side close; under repeated
   connection churn (partition/reset cycles) these can accumulate if not reliably
   joined on teardown.
2. **In-loop `defer cancel()` accumulation** (`connection_manager.go:102-103`):
   `dctx, cancel := context.WithTimeout(ctx, connectionTimeout)` is created inside
   the reconnect `for` loop, but `defer cancel()` fires only at function return — so
   during a long reconnect sequence the cancel funcs (and their contexts/timers)
   accumulate on the defer stack until `NewConnection` returns. A slow resource leak
   during prolonged outages.

## The property

TCP connection churn under sustained failures does not leak `handleServerClose`
goroutines or accumulate cancel-contexts unboundedly.

## Assertion choice

- SUT-side `Reachable("tcp-handleServerClose-goroutine-exited")` to confirm the
  teardown path runs.
- SUT-side `Always(goroutine count + pending-context count bounded)` under sustained
  TCP failure (via `runtime.NumGoroutine()` in an SDK assertion — the expvar/pprof
  server binds loopback and is unreachable cross-container).
- All instrumentation is **missing** (clean slate).

## Antithesis angle

Repeated network-partition cycles against the TCP intake container force many
reconnect attempts and server-close events — exactly the churn that exposes both
leaks. CPU throttling widens the windows. Antithesis long-run mode surfaces slow
unbounded growth that a short unit test would miss.

## Why it matters

A goroutine/context leak in a long-running agent causes gradual memory and FD
growth, eventually degrading or OOM-ing the agent. The TCP path has no leak
property today, and the `defer`-in-loop pattern is a classic, easily-reintroduced
bug.

## Open questions

- Does the topology run a TCP variant? `(needs human input)`
- Is the `defer cancel()` accumulation bounded in practice by a max-retry cap before
  `NewConnection` returns, or is the reconnect loop effectively unbounded during a
  sustained outage? `(partial: TCP backoff caps at n=7 then re-caps; whether
  NewConnection returns between cycles determines accumulation — needs confirmation)`
