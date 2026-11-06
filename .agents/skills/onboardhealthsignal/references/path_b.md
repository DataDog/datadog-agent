# Path B owner-edit guide (the owner component reports directly)

Read this before writing the owner edit. Path B means an existing component already knows the condition, and the issue must appear and clear in step with that component's own event or scan, not on a separate polling interval. If a fast periodic check would satisfy the inputs, Path A is a legitimate alternative; say which you chose and why.

## Shape of the change (keep the diff minimal)
- Issue package: `comp/healthplatform/issues/<pkg>/<type>_issue.go` (or `issue.go`), `issue_test.go`, `BUILD.bazel`. No `module.go`, no `init()`, no `bundle.go` import (a missing import cannot break Path B).
- Owner adapter: a small reporter file in the owner directory (the renderer writes `<owner_dir>/<issue_type>_health.go` and its test when the spec has `owner_dir` and `owner_pkg`). The adapter builds the issue with the shared `BuildIssue`, calls `store.ReportIssue`, and calls `store.ResolveIssue` on clear. The generated reporter already provides: an injectable store interface, `ObserveScan(ctx, ...)` that checks `ctx.Err()` first, a hashed per-host Id with a tested hostname fallback, resolution of ids persisted under another Id, `ResolveAll` for the disabled-owner case, and no-op behavior for a nil store or nil reporter. You still write the owner hook that calls it and the wiring test (the recipe is a comment at the end of the generated test file, not generated code).
- Owner hook: one observer or callback invoked from the existing detection point. Define its interface in the owner package (or a `def` package) and inject the store through it. Import `store/def` only, never `impl`, and never from `pkg/` into `comp/` (keep dependency direction owner-package to nothing new).
- The store is an optional dependency in the owner's fx graph (an optional `Requires` field or `option.Option`); every binary that links the owner without a store must still start with it nil (test the nil path). Run the fx graph test of each binary that wires the owner.
- No dead fallback: the adapter builds from `BuildIssue` and, on a builder error, logs at Warn and returns. A hand-built fallback issue ships a different shape from the catalog text and is never tested.
- Reuse an existing hashing or host-id helper from a sibling when one exists, or say in the PR body why not. Log `ReportIssue` errors at Warn.

## Where to evaluate, and what to do on edges
- Evaluate once per scan or event, at the END of the unit of work (after the final state of the scan is known), not inside an inner loop that can flip and flip back within one cycle.
- Edge-triggered: notify only on a state transition, plus always on the first evaluation after start (so an issue persisted before a restart is resolved when the condition is gone). Reason: `ReportIssue` does a synchronous disk write and a metric increment per call; re-reporting every cycle costs a write every cycle. Write the remembered state only after the callback was invoked.
- Cancellation: check `ctx.Err()` immediately before invoking the owner callback and return without notifying if the scan was cancelled. A stale notify after a restart, combined with edge-triggered state, can leave an issue active until the next transition, which may never come. Add a test that cancels from inside a source hook or the observer, not only before the scan.
- Transient source errors: a glob error, a permission blip or a missing path can lower the observed count and clear an edge-triggered issue for one cycle; the next cycle raises it as a NEW issue with a fresh detection time. There is no hysteresis here. Either accept and state the trade-off in the PR body, or skip a clear transition when a source errored with anything other than an expected "no match" in that cycle.
- Mirror the owner's own predicate exactly (same comparison operator, same boundary). If the inputs describe loss only past a boundary but the owner's warning fires at the boundary, state the discrepancy in the issue text (conditional wording) and in the PR open questions.

## Ids, reconcile, shutdown
- `id_scope` has no effect on Path B. Choose the Id yourself: a stable base for a per-host singleton; `base:<instance>` for several instances; hash the hostname when the id must not reveal it (the hostname still appears in the title and Extra).
- Hostname fallback: if the hostname component can return an empty string, fall back to `os.Hostname()` in one helper and unit test THAT helper directly with a hostname fake returning "" (the test must fail when the fallback is removed). A test that only asserts the id is not the bare `base:` is vacuous after hashing: a hash always produces a non-empty suffix.
- Prefer the store's `IssueDiscriminator` where the condition is per workload or per DaemonSet. For a per-node condition do not use it (one node recovering must not resolve the others) and say so in one sentence of the PR body. The backend keying claim stays UNVERIFIED.
- Reconcile once per reporter: resolve active ids for the same issue name that are not the current id (a hostname or scheme change). A hostname change while the agent runs is not reconciled until restart: state it.
- Disabled owner: register a hook that resolves every active id for the issue name when the owner is disabled (the scheduler does this only for Path A). If the owner can fail to start after the store is ready, say the issue stays active until the next successful start.

## Build tags and other binaries
- Owner files referenced only from tagged callers need the same tags, otherwise the `unused` linter flags them under another tag set (serverless, iot, cluster agent builds). Either tag the file, or run `go build` and the linter under each alternate tag set the owner is built with, and list the binaries that import the owner directory (grep importers).
- `deps_delta.sh` must measure the owner packages as well as the issue package: pass them (`--owner-dirs` in `verify_all.sh`). The issue leaf package and `store/def` should add nothing heavy to the other binaries.
- Pass owner directories to gazelle, the package tests and the linter; the root package and the issue package alone are not enough.

## Tests (each needs a mutant that fails it, see proof-tiers.md)
- Issue package: `issue_test.go` with the mandatory assertions.
- Adapter: report on detect; resolve on clear; first evaluation resolves a persisted issue; resolve ids persisted under another id; resolve-all on disable; nil store is a no-op; an injectable store fake.
- Observer or owner hook: transition dedupe, boundary predicate, greedy or multi-source cases that could flip within one cycle, cancellation.
- Wiring test (a REQUIREMENT, not a gap): the one line that connects the owner to the store and the disabled-owner hook line are covered by a test that drives the real owner entry point with a fake store (recipe in proof-tiers.md). It also includes a test that fails when the dependency link from the owner's fx `Requires` field (HealthPlatform) to the store field is removed; a test that only exercises the observer does not catch that (check 34 warns).
- Integration test: drive the adapter with the real store and forwarder and assert NEW and then RESOLVED on the wire (the Path A registration scaffold does not apply; do not call it a registration test).

## Neutral wording in repo code
No backend service names (say "the intake") and no skill or tool names in code, comments, tests or release notes, and no scaffold-voice prose written for the skill's author.

## PR body for Path B
State the evaluation point and why it is edge-triggered; why `IssueDiscriminator` was or was not used; the transient-error trade-off; the disabled and restart behavior; which mutants survived (if any) and why; the exact recommended CODEOWNERS co-owner line (derived from the nearest sibling) even in inputs mode, where you do not add it. List it as an item of a "Review checklist" in the PR body, with the README module-table row. Write the agent diff and the catalog diff out as `final.diff` and `catalog.diff` in the out-dir so the deliverables are not left only in a scratch worktree.
