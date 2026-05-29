---
slug: queued-payloads-eventually-sent
focus: "3 — Failure Recovery"
commit: 8ff8f30e10b4514874a6cbe9e927b88692afcfe2
updated: 2026-05-28
---

# Property: queued-payloads-eventually-sent

## What led to this property

sut-analysis.md §5 (L2) states: "queued payloads eventually sent once destination
recovers." The HTTP destination retries indefinitely on retryable errors (5xx and
network errors), using exponential backoff: base 1s, max 120s, factor 2. The
recovery signal is `cancelSendChan` in `DestinationSender`: when a retrying
destination transitions from `isRetrying=true` to `isRetrying=false`, the worker's
blocked `Send()` call returns and the next payload can be dispatched.

The liveness concern: if a network partition to the intake is injected and then
lifted, are all queued payloads guaranteed to eventually be transmitted, given
enough time? Known violations:
- **Permanent 4xx** (400, 401, 403, 413): treated as permanent errors, payload is
  dropped, auditor offset advances. The offset advances even though the data was
  not stored. This is by design but relevant to property scope.
- **Context cancellation at shutdown**: `DestinationsContext.Stop()` cancels the
  context. HTTP destinations check `ctx.Err() == context.Canceled` and return
  immediately (http/destination.go:298), abandoning in-flight payloads.
- **`cancelSendChan` race at shutdown** (H4): the `forwardWithFailover` goroutine
  may be blocked on `pipeline.InputChan <- msg` when `routerChannels` is closed.
  The blocked send does not observe the close signal.

The catch-up problem (external ref #2): after a long partition, both replayed
(registry-based) and live log lines compete for the same pipeline channels. The
backpressure model (100-slot channels) means replayed payloads can crowd out live
data or vice versa.

## Key code locations

- `comp/logs-library/sender/worker.go:120-148` — `for !sent` loop with 100ms
  sleep when all reliable destinations are retrying.
- `comp/logs-library/sender/destination_sender.go:100-129` — `Send()`: blocks
  until `isRetrying=false` or `cancelSendChan` fires.
- `comp/logs-library/client/http/destination.go:256-320` — retry loop with
  backoff. Line 298: `context.Canceled` → return without updating output.
- `comp/logs-library/client/http/destination.go:200-203` — error classification:
  retryable vs. non-retryable.
- `comp/logs/agent/agentimpl/agent.go:319-332` — `stop()`: stops
  `destinationsCtx` which cancels all in-flight HTTP requests.

## What fault triggers it

**Network partition to intake** (Antithesis network fault) followed by recovery.
The property tests that after the network fault clears, queued payloads eventually
reach the intake without manual intervention.

**Verification requires a fault-quiet window** (`ANTITHESIS_STOP_FAULTS` or
equivalent) after partition recovery, long enough for all buffered payloads to
drain through the retry path. Given max backoff of 120s, the quiet window should
be at least 3 minutes post-recovery.

## Why it matters

In production, brief network blips (TLS renegotiation, load-balancer drains,
DNS flaps) should not cause permanent data loss. The retry mechanism is the
primary defense. If it fails to drain after recovery, the only signal is the
`RetryCount`/`RetryTimeSpent` metric, which operators may not monitor.

## Assertions needed (all net-new SUT instrumentation)

1. **`Sometimes(destination transitions from retrying to not-retrying)`** — SUT-
   side in `destination_sender.go` `startRetryReader` goroutine: a `Sometimes`
   assertion when `!v` (isRetrying=false) fires after `v` (isRetrying=true) was
   seen. This confirms recovery is observed at least once during the run.
2. **Workload `Always(fakeintake_received_count >= injected_count - permanent_drops)`**
   — after fault-quiet window, the number of log lines at fakeintake equals the
   number injected minus the count of intentionally permanent-dropped lines
   (4xx responses). Any shortfall indicates payloads were lost despite recovery.
3. **`Reachable(HTTP retry backoff sleep entered)`** — SUT-side in
   `http/destination.go` backoff path: confirms the retry mechanism is actually
   engaged during the fault injection, not just avoided.

## Recovery window requirement

Fault-quiet window of at least 3 minutes required after network partition clears,
to allow max-backoff retry to fire.

## Open questions

- When `DestinationsContext.Stop()` is called during graceful shutdown, are
  in-flight payloads that were already `output <- payload`'d to the auditor but
  not yet written to the registry treated as re-deliverable on next start?
  `(partial: yes — these are in the auditor inputChan and will not be drained by
  Stop(), so they are replayed on restart — confirmed by H5 in sut-analysis.md §3)`
- Is the `close_timeout` for file rotation configurable in the test topology?
  The property's outcome depends entirely on whether the partition duration
  exceeds this timeout.
- For the 429/Retry-After gap: does the Datadog intake ever actually return 429
  to the agent? If yes, the missing Retry-After support means the agent can
  exceed the intake's rate limit during retry storms. `(needs human input)`
- Does the TCP destination have the same retry semantics as HTTP? The SUT
  analysis mentions different backoff constants for TCP (binary exponential,
  cap n=7 → 64-128s). If the test topology uses TCP, the retry window differs.
- Does the health check (`a.health.C` in `auditor.go:284`) cause the liveness
  monitor to alert before the pipeline recovers? If yes, it is an existing
  liveness signal and this property may be redundant with production alerting.

### Investigation Log

#### Does the 100ms busy-sleep in `worker.go:146` add a full interval of delay on every recovery?

- Examined: `comp/logs-library/sender/worker.go:120-148`, `comp/logs-library/sender/destination_sender.go:55-68` (`startRetryReader`), `destination_sender.go:100-130` (`Send`).
- Found: The 100ms sleep is inside the `for !sent` loop in `worker.go`. When a reliable destination transitions from retrying to not-retrying, `cancelSendChan` fires (in `startRetryReader`), which unblocks any pending `Send()` call. However, the 100ms sleep runs when ALL destinations return `false` from `Send()` — i.e., when all are retrying. On the recovery iteration, if `cancelSendChan` unblocks `Send()` (which returns `false` because isRetrying was true when `Send()` started), the `sent=false` branch is taken, the sleep fires, and on the next loop iteration the destination is now not-retrying and `Send()` succeeds. Net: recovery can add one 100ms sleep interval. This is not a liveness hazard — the loop will converge — but it adds ≤100ms latency to the first successful send post-recovery. Under Antithesis thread-pause faults, this window is observable.
- Not found: any case where the busy-sleep causes indefinite stall after recovery; the loop is bounded by the recovery signal.
- Conclusion: resolved. The 100ms sleep adds ≤100ms latency to recovery (one iteration). Not a liveness hazard. The post-recovery test window should account for this by using ≥ max_backoff (120s) + buffer, not just max_backoff.

#### What is the exact `RecoveryInterval` and `RecoveryReset` for the HTTP backoff policy?

- Examined: `pkg/config/setup/config.go` (constants), `pkg/util/backoff/backoff.go` (`ExpBackoffPolicy`), `comp/logs/agent/config/config_keys.go` and `endpoints.go`.
- Found: Default constants in `pkg/config/setup/config.go`: `DefaultLogsSenderBackoffFactor = 2.0`, `DefaultLogsSenderBackoffBase = 1.0`, `DefaultLogsSenderBackoffMax = 120.0`, `DefaultLogsSenderBackoffRecoveryInterval = 2`. `sender_recovery_reset` defaults to `false` (line `config.BindEnvAndSetDefault(prefix+"sender_recovery_reset", false)`). `RecoveryInterval=2` means each successful send decrements `nbErrors` by 2. `RecoveryReset=false` means full recovery to zero errors requires `ceiling(maxErrors / 2)` successful sends; `maxErrors = floor(log2(120/1)) + 1 = 7`. So full recovery takes 4 successful sends (errors: 7→5→3→1→0). After the first successful send, backoff drops from max (120s) to a shorter interval immediately.
- Not found: any runtime override of these values in the test topology (would be set via `DD_LOGS_CONFIG_SENDER_BACKOFF_*` env vars).
- Conclusion: resolved. RecoveryInterval=2, RecoveryReset=false (default). Max backoff 120s. Full recovery after 4 consecutive successes. The "fault-quiet window of 3 minutes" in the property is conservative and correct.

## Merged-in evidence (from retry-no-data-loss-on-partition)

The secondary file added the **rotation-under-partition compound failure**
scenario not present in the canonical:

**Compound loss path:** during a partition > `close_timeout` (60s), if the
tailed file rotates, the old tailer's `stopForward()` fires. Unprocessed messages
in the old tailer's output channel are discarded even after the partition clears.
This is the intersection of the at-least-once retry guarantee and the
rotation-loss path — data is lost despite the retry mechanism functioning
correctly, because the tailer drain window expired before the partition cleared.

**Non-reliable destination silent drops** (`worker.go:160-164`): secondary
destinations receive `NonBlockingSend()` — if their buffer is full, the payload
is dropped silently, not retried.

**Workload variant** (from secondary): with partition duration > `close_timeout`,
sequence numbers for lines written during rotation after partition start may be
missing — document this as a known acceptable loss per the SUT's stated caveat.

## Merged-in evidence (from retryable-error-eventually-retried)

The secondary file provided **additional code detail** on the retry mechanism,
particularly the TCP path and the 429 treatment:

**TCP connection-manager retry** (`comp/logs-library/client/tcp/connection_manager.go:71-128`):
the TCP retry loop has `defer cancel()` inside the loop — cancel functions
accumulate until `NewConnection` returns. This is a goroutine-local resource
issue, not a semantic bug, but worth flagging.

**429 classification** (`destination_test.go:109` via `retryTest(t, 429)`):
429 is confirmed retryable (falls into `> StatusBadRequest` branch → retryable
error). The agent applies its own exponential backoff and ignores any `Retry-After`
header.

**Additional assertions (from secondary):**
- `Sometimes(d.nbErrors > 5)` in `sendAndRetry()` — confirm the agent reaches
  significant retry depth, not just one retry.
- Workload `Always`: after fault clears, log bytes received at fakeintake strictly
  increases. If stagnant for > 3× max backoff (360s), the pipeline is stuck.

## Merged-in evidence (from pipeline-makes-progress)

The secondary file addressed the **`cancelSendChan` delivery race** during
recovery and the bounded-delay guarantee:

**`cancelSendChan` delivery window** (`destination_sender.go:59-65`): the signal
is sent only if the current state is non-retrying. There is a window between
`d.lastRetryState = v` (line 65) and the unblocking of `Send()` where the state
has changed but the signal has not been delivered. This is a benign race that adds
≤100ms latency to recovery, but under Antithesis thread-pause faults the window
can be made arbitrarily large.

**Context-cancellation blocks recovery** (`destination.go:298-301`): if the
context is cancelled during a retry (e.g., slow shutdown), `sendAndRetry` exits
without enqueuing to `output`. The payload is dropped and the worker is not
notified — it will hang in the busy-poll until `<-s.done`.

**SUT-side liveness assertion (from secondary):** `Sometimes` confirming
`LogsSent` counter advances during each quiet period (counter never frozen for
> threshold) — currently missing.
