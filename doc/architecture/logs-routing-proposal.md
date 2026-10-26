# Log routing mechanism: requirements and implementation plan

## 1. Requirements brief

**Status: requirements review.** This document records the discussion consensus and separates unresolved policies from agreed requirements.

The requirements below are non-negotiable constraints for the mechanism PR. The user must confirm this brief before architecture design proceeds.

### Outcome

Route each retained log to an explicit set of delivery groups. Preserve the familiar batching, sender, and HTTP delivery chain within each group.

A delivery group contains one primary endpoint and optional extra endpoints. It is a delivery boundary, not a new collection or processing pipeline.

### Non-negotiable requirements

| ID | Requirement | Acceptance evidence |
| --- | --- | --- |
| R1 | Deliver the routing mechanism in one focused PR. Internal contract boundaries do not mandate separate PRs. | One coherent diff and focused tests cover the mechanism. |
| R2 | Exclude the product configuration surface, activation switch, and real selection policy. Use injected inputs and test stubs. | No new customer YAML, environment variable, DSL, or remote-config product. |
| R3 | Support selection of group A only, group B only, or both. Do not implicitly include the primary group. | Tests assert exact recipients for exclusive and overlapping selections. |
| R4 | Use delivery groups as the routing unit. Each group needs at least one reliable endpoint. | A single reliable endpoint works without any backup configuration. |
| R5 | Make extra endpoints optional. Preserve their existing copy semantics within each selected group. | Tests cover groups with one endpoint and groups with optional extra endpoints. |
| R6 | Do not treat extra endpoints as standby failover. Do not redirect logs to unselected groups after failure. | Outage tests show no unauthorized recipients or implicit fallback. |
| R7 | Share processing rules and record encoding. Do not create a decoder or processor for each group. | Each retained input record passes through processing and encoding once. |
| R8 | Split records before group batching. Keep compression within the existing batching machinery. | Each group receives only its selected records before batch construction. |
| R9 | Keep the sender stage. Reuse existing batching, sender, and HTTP implementations where their contracts fit. | No second HTTP retry, authentication, or transport framework. |
| R10 | Compress once per group batch. Share that payload across endpoints within the group. | Group-local extra endpoints reuse the compressed payload. |
| R11 | Bound group resources and aggregate resources. Do not use unbounded queues or blocked-record goroutines. | Saturation tests check bounded pending work and resource release. |
| R12 | Prevent a blocked group enqueue from directly blocking admission to another available group. | A progresses during a B outage within the documented capacity limits. |
| R13 | Preserve existing behavior within each group's sender. Do not promise successful delivery to every endpoint. | Tests retain reliable-first acceptance, retries, bounded duplicate buffering, and terminal-loss behavior. |
| R14 | Keep admission distinct from completion. Unselected groups cannot directly acknowledge a routed record. | Tests distinguish queue acceptance, sender terminal output, and successful HTTP delivery. |
| R15 | Preserve the existing default path and unrelated callers. Do not activate production routing through this PR. | Existing-path tests pass without constructing routed machinery. |
| R16 | Preserve source metadata and safe ownership across fanout. Track original records separately from destination copies. | Race and accounting tests cover shared records, completion, and counters. |
| R17 | Handle flush, shutdown, cancellation, and credential waiting explicitly. Do not invent completion to finish shutdown. | Lifecycle tests cover partial admission, pending delivery, and cancellation. |

### Scope boundaries

Included: internal routing contracts, group construction, bounded admission, group batching, sender integration, completion integration, and focused tests.

Excluded: customer configuration, enablement controls, real routing rules, rule precedence, default-selection policy, hot updates, and true standby failover.

Also excluded: durable spooling, exactly-once delivery, independent per-org replay, and a new contiguous source-checkpoint ledger.

The initial integration targets the normal non-serverless HTTP path. TCP, serverless, MRF composition, and event-platform routing remain outside this mechanism change.

Microbenchmarks are not a prerequisite. The plan must state performance risks without claiming measured performance parity.

## 2. Stub contract boundaries

These contracts describe inputs to the mechanism. They do not implement a configuration surface or either kind of switch logic.

### Group definitions

Input: stable group identities, existing endpoint objects, and internal resource settings.

Responsibility: construct and validate the permitted delivery groups. Validation must reject ambiguous identities and unsupported group definitions.

Non-responsibility: parse product configuration, fetch remote rules, choose activation, or discover fallback destinations.

A group with one reliable primary is valid. Extra endpoints are optional and receive normal copies within that selected group.

### Resolved routing decision

Input: an explicit set of group IDs for one retained record. Tests supply fixed or table-driven decisions.

Responsibility: dispatch the record only to those groups and prevent duplicate admission from repeated IDs.

Non-responsibility: evaluate filters, choose a default, resolve rule precedence, or change recipients after delivery failure.

The production policy can later supply these IDs without changing batching or sender internals. The pre-encoding policy hook remains a separate integration question.

### Record handoff

Input: encoded content, source metadata, and resolved group IDs. Sharing content must not let one lane mutate another lane's record.

Responsibility: define ownership transfer, pending-copy state, cancellation, and resource release.

The mechanism must expose admission separately from delivery completion. A new queue cannot act as an intake acknowledgement.

## 3. Unknowns and decisions before implementation

The requirements review must not convert earlier assistant suggestions into approved delivery policies.

### U1. Cross-group obligations and overflow

Within-group behavior follows the existing sender. Cross-group behavior still needs a precise contract.

If A accepts a shared record, can the router drop B's copy when B exhausts its budget?

The earlier proposal allowed that drop to resemble existing extra-endpoint behavior. Independent groups do not automatically inherit that relationship.

An alternative retains both group obligations and eventually applies upstream backpressure. That changes outage coupling and completion expectations.

Decision needed: define cross-group admission, overflow, and completion together. Identify whether one or all selected groups must reach a terminal outcome.

### U2. Exact admission boundary

Router admission, group-queue admission, sender acceptance, and HTTP completion are different events.

Decision needed: define which event transfers ownership and which event can justify continuing after another copy cannot enter its group.

The implementation must not claim exact parity with existing payload-level acceptance without evidence.

### U3. Invalid and empty decisions

Unknown group IDs, policy errors, and explicit empty selections must remain distinct. None may silently become primary-only delivery or a broadcast.

Decision needed: define rejection, intentional drop, cancellation, and source-progress behavior for each case. Policy defaults remain outside this PR.

### U4. Duplicate destination membership

One endpoint can appear in more than one selected group. That can cause duplicate network delivery even when each group ID appears only once.

Decision needed: reject overlapping membership, allow explicit duplication, or define destination-level deduplication. Deduplication across groups adds ownership complexity.

### U5. Budget enforcement and batch parallelism

Existing sender channels use payload counts. That does not establish a strict byte limit across the entire group.

Decision needed: define count and byte accounting across records, batches, compressed payloads, sender queues, and in-flight requests.

Also define group count, request concurrency, and compressor limits. Determine whether one batcher per group preserves enough compression parallelism.

### U6. Metadata, completion, and observability

Decision needed: verify immutable metadata boundaries and any record-to-payload mapping. Determine whether the existing sink needs a completion adapter.

Current HTTP terminal output includes permanent failures. Do not label every completion as successful delivery.

Define monitor identities and original-record versus copy counters. Separate sender instances must not collide in metrics or lifecycle ownership.

### U7. Integration point and lifecycle

Decision needed: find the smallest processor/provider integration change that accepts resolved group selections without implementing policy or activation logic.

Tests can construct the routed mechanism directly. Production mode selection remains outside scope.

Define flush coverage, shutdown order, drain deadlines, and cancellation propagation. Confirm whether any reused lifecycle contract needs a narrow adapter.

## 4. Explicit risks and limits

| Risk | Consequence | Planned treatment |
| --- | --- | --- |
| Overlap across groups | Shared records incur batching and compression once per selected group. | Document the cost. Keep exact-destination-set batching outside this first design. |
| Full overlap | Duplicate compression brings no reduction in destination copies. | Reconsider the design if expected traffic primarily has complete overlap. |
| Different batch membership | Compression ratio, request count, latency, and ordering can change. | Do not claim performance or payload-boundary parity. |
| Compression concentration | One group batcher may replace work previously spread across several processor batchers. | Review parallelism before fixing lane counts. |
| Shared record lifetime | A stalled group can retain content after another group completes. | Account for retained bytes and bound pending references. |
| Aggregate growth | Group count can multiply buffers, compressors, workers, and HTTP requests. | Establish group and aggregate limits before integration. |
| Shared upstream sources | A full pending window can stop later healthy-group records from the same source. | Promise bounded outage isolation, not unlimited independent progress. |
| Existing checkpoints | Later A completion can supersede an earlier pending B source offset. | State that per-group replay safety is not part of this PR. |
| Permanent HTTP errors | Terminal loss can advance source progress. | Preserve existing semantics and loss metrics. |
| Slow endpoints within a group | A full destination queue can block its shared worker before retry state changes. | Preserve and document existing group-local behavior. |
| API reuse mismatch | Existing helpers may assume global monitors, MRF flags, or payload-level metadata. | Verify contracts and make only narrow integration changes. |
| Scope expansion | Policy, failover, or replay guarantees could turn this into a large redesign. | Resolve unknowns explicitly. Do not hide new guarantees inside adapters. |

## 5. Ordered implementation plan

These are work items within one mechanism PR, not separate PRs. Implementation remains blocked until the requirements and material policies receive approval.

1. **Finalize requirements and delivery contracts.** Resolve U1–U4 and record the accepted behavior matrix in this file.
   Completion check: each selection and failure case has explicit recipients, ownership, overflow, and completion behavior.
2. **Finalize the architecture and integration boundaries.** Resolve U5–U7 and add the side-by-side diagram after requirements approval.
   Completion check: every block identifies existing code, changed integration, or new code, with clear data and completion paths.
3. **Add focused contract tests and injected stubs.** Cover exclusive groups, overlapping groups, optional extras, and invalid decisions.
   Completion check: tests express the approved behavior without product configuration or real policy logic.
4. **Implement bounded routing and group-local delivery.** Reuse batching, sender, and HTTP code with explicit resource ownership.
   Completion check: selected groups receive the correct records without duplicate admission or an unconditional cross-group broadcast.
5. **Integrate completion and lifecycle behavior.** Preserve source metadata, terminal-loss semantics, and the unchanged legacy path.
   Completion check: outage, saturation, credential, flush, cancellation, and shutdown tests match the approved contract.
6. **Validate and document the boundary.** Run focused tests through `dda inv test`, then relevant shared-library regression checks.
   Completion check: report test results, skipped checks, resource limits, and known delivery limitations without claiming stronger guarantees.

## 6. Architecture diagram: approval gate

Do not finalize the diagram before the requirements brief receives user approval.

After approval, add the current and proposed paths side by side here. Mark each component as **SAME**, **CHANGED**, or **NEW**.

The diagram must show processing, record distribution, batching, compression, senders, HTTP destinations, and completion flow at comparable levels.

Show optional extra endpoints within groups. Show stub inputs separately from the data path, without designing policy or configuration surfaces.

SAME means reused behavior and implementation. It does not mean an unchanged instance count, memory budget, or integration topology.

## 7. Codebase evidence

The current-path evidence covers normal non-serverless HTTP delivery. MRF behavior remains a separate compatibility concern.

- [Existing research](logs-delivery-research.md).
- [Pipeline construction](../../comp/logs-library/pipeline/pipeline.go): `NewPipeline` connects processor output to a batch strategy and sender input.
- [Provider construction](../../comp/logs-library/pipeline/provider.go): `NewProvider` and `httpSender` construct the current shared sender and concurrency settings.
- [Sender construction](../../comp/logs-library/sender/sender.go): `NewSender` accepts a destination factory and sink, with explicit queue and worker counts.
- [Distribution](../../comp/logs-library/sender/worker.go): `worker.run` uses reliable-first acceptance and bounded copies for other destinations.
- [Backpressure](../../comp/logs-library/sender/destination_sender.go): `DestinationSender.Send` can block on capacity and stop waiting when retry state changes.
- [Compression](../../comp/logs-library/sender/batch.go): `batch.sendMessages` closes the compressor before sending its payload downstream.
- [HTTP completion](../../comp/logs-library/client/http/destination.go): `Destination.sendAndRetry` emits terminal output for success and some permanent failures.
- [Source progress](../../comp/logs/auditor/impl/auditor.go): `registryAuditor.updateRegistry` tracks source offsets, not per-group obligations.
