# tcp-permanent-error-no-offset-advance

**Added:** evaluation gap-fill, B-TRANSPORT decision (2026-05-29).

## What led here

The evaluation coverage-balance lens (GAP-2) and the auditor-offset investigation
found a **real HTTP/TCP asymmetry** in how a permanent send failure interacts with
the auditor offset:

- **HTTP destination** (`comp/logs-library/client/http/destination.go`, ~`:318`):
  `output <- payload` runs after the retry loop for *both* success and permanent
  (4xx) failure — so a permanently-rejected payload **advances** the auditor offset
  (the data is intentionally dropped, never retried).
- **TCP destination** (`comp/logs-library/client/tcp/destination.go`, ~`:116-120`):
  on a permanent failure the payload is **not** sent to `output`, so the offset is
  **not** advanced — the data is retried / re-read.

This means at-least-once durability differs by transport on the permanent-error
path. It is undocumented.

## The property

On a TCP-path permanent send failure, the auditor offset must NOT advance — the
payload is retried/re-read, never silently skipped. This is the *correct* TCP
behavior; the property guards against a refactor that aligns TCP to HTTP's
advance-on-permanent-drop semantics (which would introduce silent loss for TCP
users).

## Assertion choice

- `Always`: on the TCP permanent-failure path, `output <- payload` is NOT called
  (so the registry offset for that payload is not advanced). Workload reconciles
  fakeintake delivery against the recovered offset after a TCP-variant run.
- All instrumentation is **missing** (no Antithesis SDK in repo).

## Antithesis angle

Network partition / connection reset against the TCP intake drives the permanent
vs retryable classification. The divergence from HTTP is the interesting risk
surface — Antithesis code-path exploration plus a code-change would catch an
accidental alignment.

## Why it matters

A TCP user silently gets different durability than an HTTP user on permanent
errors. If a future change made TCP advance the offset on permanent failure
(matching HTTP), TCP would start silently dropping data with no signal.

## Open questions

- Is the HTTP-vs-TCP offset-advance asymmetry intentional? `(needs human input)`
  (shared with `auditor-offset-safety`)
- Does the topology run a TCP variant (required to exercise this)? `(needs human input)`
- What counts as a "permanent" failure on the TCP path (vs the indefinite reconnect
  loop)? `(partial: TCP mostly retries connection indefinitely; the permanent path
  is narrow — needs confirmation of which conditions reach it)`
