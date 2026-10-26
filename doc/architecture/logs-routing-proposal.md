# Log routing: implementation handoff

## 1. Status and authority

**Agreed direction, ready for implementation handoff.** No routing runtime implementation or runtime validation exists from this planning work.

This document separates three categories:

- **Agreed contract:** behavior and scope from the discussion.
- **Code evidence and derived constraints:** existing behavior and the safety consequences of the contract.
- **Implementation proposals:** suggested wiring, representations, and tests for the receiving agent to verify.

We did not discuss every implementation detail as a separate decision. Proposed details do not authorize new product behavior.

Use this document as the primary contract. Use [delivery research](logs-delivery-research.md) as background, with its older baseline.

Code evidence uses local revision `3dc2ba2efb9`. Recheck the receiving checkout before changing code.

The implementer owns technical details and validation. Ask the user before changing delivery guarantees, expanding scope, or accepting a material resource tradeoff.

## 2. Agreed contract

A delivery group has one primary endpoint and optional extra endpoints. Each group needs at least one reliable endpoint.

Independent groups carry independent delivery obligations. Optional endpoints inside a group keep existing copy semantics.

### Scope and recipients

- **R1:** Deliver one focused mechanism PR, not separate PRs for each interface.
- **R2:** Use injected group definitions and resolved selections, without real routing policy or production activation.
- **R3:** Make the maximum group count configurable, with four as the default. Support explicit selections without implicit recipients.
- **R4:** Use delivery groups as the routing unit. A single reliable endpoint needs no backup.
- **R5:** Keep extra endpoints optional and preserve their existing within-group copy semantics.
- **R6:** Do not redirect records to unselected groups after failure or treat extra endpoints as standby failover.

The group-count setting is the explicit exception to excluding customer configuration from this PR.

Routing configuration, DSLs, activation switches, remote rule distribution, hot updates, and default-selection policy remain outside scope.

Groups may contain the same destination. A record selecting both groups can produce two copies at that destination.

Do not add cross-group network deduplication. Different org credentials on one intake hostname do not establish duplicate destinations.

Reject invalid definitions before starting workers. Report invalid selections without implicit fallback or acknowledgement.

Repeated group IDs must not cause duplicate admission within one selection.

An intentional exclusion must be explicit. An empty or failed selection must not silently become exclusion.

### Processing and delivery

- **R7:** Share processing and encoding rather than create a decoder or processor per group.
- **R8:** Select recipients before batching. Share batches only for declared identical streams with compatible settings.
- **R9:** Preserve the batcher → Sender → HTTP chain and reuse existing delivery components.
- **R10:** Compress each batch once and share immutable payload bytes where the declared streams match.
- **R11:** Bound group and aggregate resources. Do not use unbounded queues or blocked-record goroutines.
- **R12:** Let available groups progress during another group's outage within the documented capacity limits.
- **R13:** Preserve existing sender behavior inside each group, without promising delivery to every optional endpoint.

Use **one existing `Sender` per independent group**. Keep optional extra endpoints inside that sender.

Single-endpoint groups give one sender per endpoint. Multi-endpoint groups do not need a top-level sender for every extra endpoint.

Declared identical streams share batching and compression, but retain separate group senders and completion obligations.

Do not discover equality through content hashing, completed-batch comparison, or dynamic recipient-set batching. Partial overlap may repeat compression.

Do not import MRF activation, skipping, or cross-group overflow-drop behavior merely because the streams match.

### Progress, recovery, and cleanup

- **R14:** Keep admission separate from completion and advance checkpoints only through the safe source prefix.
- **R15:** Preserve the default path and unrelated callers. Do not activate production routing in this PR.
- **R16:** Preserve source metadata and distinguish original records from destination copies.
- **R17:** Handle shutdown, cancellation, flushing, and credential waiting without false completion.
- **R18:** Reuse existing cleanup ownership and keep resource release separate from acknowledgement.
- **R19:** Retain every selected group's pending obligation instead of dropping its copy on routing overflow.
- **R20:** Bound progress metadata without duplicating log bodies or adding per-record disk writes.

Keep network delivery concurrent. Order checkpoint advancement, not every HTTP request.

Use the existing auditor and periodic persistence. Do not add independent endpoint checkpoints, durable spooling, or an exactly-once promise.

Preserve default startup behavior without a new initial-checkpoint persistence barrier.

Replay needs a usable persisted checkpoint, retained source data, and consistent routing. Duplicate replay is acceptable.

Without an existing checkpoint, default end-mode startup can skip pending records. Preserve and document this inherited limitation.

Invalid registries, startup overrides, deleted sources, and unbuffered network input remain outside the recovery guarantee.

Preserve existing intentional exclusions, permanent HTTP losses, and credential-discard behavior. Retaining obligations does not mean retrying every terminal loss forever.

Every routed divergence needs a contract reason, an existing limitation, a bounded change, and a focused test.

## 3. Architecture

The agreed structure uses shared processing, stream batching, and one sender per group. The precise registration, dispatch, and completion adapters below are implementation proposals.

The current-path diagram describes normal non-serverless HTTP delivery. It does not establish compatibility with other transport modes.

**SAME:** existing logic. **CHANGED:** wiring or narrow hooks. **NEW:** routing-owned state.

These labels do not promise equal instance counts, memory use, or performance.

### Data path

```text
CURRENT DEFAULT PATH                         ROUTED DIRECTION

Tailer + decoder                             [SAME] Tailer + decoder
       |                                            |
       |                                     [NEW] Register source record
       |                                            |
Provider dispatch / failover                  [CHANGED] Provider dispatch
       |                                            |
Processor pool                               [CHANGED] Processor pool
rules + one encoding pass                    same processing + outcome hooks
       |                                            |
       |                                     [NEW] Dispatch selected records
       |                                            |
Batcher/compressor per pipeline               [CHANGED] Stream batchers
       |                                     separate or declared shared
       |                                            |
       |                                     [NEW] Bounded payload dispatch
       |                                            |                |
Shared Sender                                [SAME] Sender A  [SAME] Sender B
       |                                            |                |
Primary HTTP + optional extras               [SAME] HTTP A    [SAME] HTTP B
                                             + optional      + optional
                                             group A extras  group B extras
```

### Declared identical streams

```text
One ordered encoded stream, with compatible payload settings
                         |
              One batch and compression
                         |
               Immutable payload bytes
                         |
                  Bounded dispatch
                    /          \
               Sender A      Sender B
                  |             |
               Group A       Group B
```

Sharing bytes does not merge completion obligations or prevent two groups from targeting the same endpoint.

### Completion path

```text
CURRENT                                      ROUTED PROPOSAL

Reliable terminal output                     Reliable terminal output
          |                                             |
          |                                  Group-bound sink adapter
          |                                             |
          |                                  Source-progress coordinator
          |                                  registration + group outcomes
          |                                  bounded state / safe prefix
          |                                             |
       Auditor                                       Auditor
          |                                             |
  Periodic persistence                       Periodic persistence
```

Local exclusions and losses also need outcomes. Queue admission and reference release must not advance checkpoints.

The coordinator owns registration and completion state. The diagram does not mandate separate components for every box.

One bounded routing owner can handle record and payload dispatch. Do not infer a need for two generic scheduling frameworks.

## 4. Existing behavior and derived safety constraints

### Code evidence

- [Provider](../../comp/logs-library/pipeline/provider.go): `NewProvider` creates a shared sender. Failover can distribute consecutive source records across processors.
- [File tailer](../../pkg/logs/tailers/file/tailer.go): `forwardMessages` emits ordered decoder output, including checkpoint metadata.
- [Message buffer](../../comp/logs-library/sender/message_buffer.go): `AddMessage` copies metadata by value.
- [Processor](../../comp/logs-library/processor/processor.go): filtering, rendering, and encoding can return without downstream output.
- [Batch strategy](../../comp/logs-library/sender/batch_strategy.go): explicit input/output channels and existing flush/stop behavior support rewiring.
- [Batch](../../comp/logs-library/sender/batch.go): serialization, size, and compression failures can discard records without sender output.
- [Sender](../../comp/logs-library/sender/sender.go): `Sink.Channel` carries payloads without group identity or success classification.
- [HTTP sender](../../comp/logs-library/sender/http/http_sender.go): construction accepts endpoints, sink, monitor, and concurrency settings.
- [Worker](../../comp/logs-library/sender/worker.go): reliable-first admission can progress while other destination copies buffer or drop.
- [Destination sender](../../comp/logs-library/sender/destination_sender.go): admission can block before retry-state changes release it.
- [HTTP destination](../../comp/logs-library/client/http/destination.go): terminal output can mean success or permanent rejection, not successful delivery alone.
- [Auditor](../../comp/logs/auditor/impl/auditor.go): source progress has no independent-group pending ledger.
- [Message types](../../pkg/logs/message/message.go): preserve decoder-aware checkpoint metadata rather than infer offsets from raw lengths.
- [Windows events](../../pkg/logs/tailers/windowsevent/tailer.go): `registrySaverWithInitialBookmark.Save` writes directly through `Registry.SetOffset`.
- [File recovery](../../pkg/logs/launchers/file/position.go): missing saved progress can select EOF in end mode.
- [Auditor interface](../../comp/logs/auditor/def/component.go): `Flush` returns no error and cannot prove durable persistence by itself.
- [Agent lifecycle](../../comp/logs/agent/impl/agent.go): existing startup, shutdown, and destination cancellation ordering needs preservation.

### Consequences the implementation must address

Register before processor distribution can reorder source records. Carry stable record identities through metadata copies without serializing internal tokens or credentials.

Every checkpoint writer for a coordinated source must honor its pending state. Sender-sink interception alone does not cover direct bookmark writes.

Old completions must not advance a replacement source generation. Empty identifiers must not share one accidental checkpoint ledger.

Batch completion applies only to its record identities and reporting group. Repeated notifications must not resolve another group's obligation.

Unreliable endpoints do not resolve group obligations.

Capture identities before batch resets or compressor cleanup discard them. A shared-batch loss affects dependent groups, not unrelated records.

### Outcome mapping

This table translates the agreed default-policy rule into engineering checks. It does not represent twelve separate user policy decisions.

| Event | Routed treatment |
| --- | --- |
| Processing-rule exclusion | Explicit intentional exclusion for the registered record |
| Render or encoding failure | Terminal local loss for the current record |
| Serialization or `Finish` failure | Terminal local loss for every discarded buffered record in the affected stream |
| Compressor `Close` failure | Terminal local loss for every record in the discarded payload |
| Existing batch-size rejection | Existing terminal local loss for the affected record and stream |
| New router budget rejection | Caller retains ownership, with no acknowledgement from rejection alone |
| Reliable HTTP success | Completion of the included records' obligation for that group |
| Permanent HTTP rejection | Existing terminal output and loss accounting, not proof of success |
| Retryable HTTP failure | Pending obligation during existing retry behavior |
| Credentials expected later | Waiting or retrying without acknowledgement |
| Credentials permanently unavailable | Destination copy discard without invented group completion |
| Cancellation or shutdown | Resource release without resolving pending obligations |

Before fanout, a local loss resolves the registered record once. After fanout, it resolves only affected stream obligations.

Another reliable endpoint in a group can complete an obligation after one endpoint discards its credential-less copy.

Without such completion, the group stays pending and bounded state can eventually stop the source.

## 5. Implementation choices, not new user policy

The receiving agent should resolve these details using existing patterns. No exact queue sizes, configuration key, token type, or monitor identity has user approval.

### Resource budgets and configuration

Choose numeric limits for ingress, record references, payload references, progress entries, destination queues, and HTTP requests.

Account for extra endpoints and declared sharing. Group count alone does not bound these costs.

The normal HTTP provider uses one queue and one sender worker. Each endpoint supports concurrent HTTP requests.

Current default HTTP concurrency is pipeline count multiplied by ten per endpoint. A non-default `batch_max_concurrent_send` replaces ten.

The current normal path sets equal minimum and maximum concurrency. One shared sender does not make its per-endpoint limits global.

Define aggregate bounds and their assumptions without promising an exact process-memory ceiling. Do not claim measured performance parity.

Proposed configuration behavior: read the group limit at startup and validate it before workers start. Hot updates remain outside scope.

Choose the key and validation using existing configuration conventions. Support limits above four without fixed four-group masks or arrays.

Specify which exhausted limits block which sources. A shared-source outage can eventually stop healthy-group traffic from that source.

Aggregate exhaustion can also affect unrelated sources. More sender instances do not remove this finite-capacity tradeoff.

Distinguish temporary pressure from a record that can never fit. Reject impossible admission before ownership transfer rather than wait forever.

### Interfaces and ownership

Define explicit exclusion separately from invalid selections. Validate identical-stream declarations against compatible payload settings and resolved recipients.

Choose source-generation ownership, stable token representation, synchronization, and group monitor identities.

Keep shared payload bytes immutable until consumers release them. Reuse existing lifecycle methods instead of replacing component cleanup.

Stage accounting can end at transfer to another bounded stage. That release must not resolve delivery progress.

Proposed shutdown order: stop admission, drain processors, flush batchers with dispatch active, then drain senders with completion consumers active.

Drain safe progress before stopping the auditor. Forced cancellation releases unresolved state without acknowledgement.

The researched integration target is normal non-serverless HTTP. Do not silently expand transport support or exclude source types to avoid safety work.

Classify source-owned initialization and recovery writes before claiming routed safety, especially for Windows bookmarks.

## 6. Execution and acceptance

These are work items within one PR. The implementer should complete technical checks rather than wait for another policy discussion.

1. **Confirm code and contracts.** Recheck the target revision, declare resource bounds, and choose the small internal representations.
2. **Prove source safety.** Audit registration order, generation changes, empty identifiers, and every checkpoint writer for routed sources.
3. **Write focused tests.** Cover the contract and outcome mapping before changing behavior where practical.
4. **Implement the mechanism.** Reuse batching, group senders, HTTP destinations, bounded dispatch, and coordinated source progress.
5. **Verify lifecycle and default behavior.** Cover blocked admission, flush, cancellation, credentials, and safe cleanup.
6. **Validate the result.** Report commands, outcomes, skipped checks, resource limits, and remaining risks.

### Acceptance evidence

- Exact recipients for exclusive and overlapping selections, without implicit broadcast or fallback.
- Shared endpoint membership without merging group obligations or adding network deduplication.
- Default, lower, and higher group limits, including more than four groups and invalid or over-limit rejection.
- One compression operation per declared shared batch, with correct decoded contents and separate completion outcomes.
- Conflicting declarations and selections without unauthorized delivery or false acknowledgement.
- Partial admission, bounded saturation, and healthy-group progress within documented limits.
- Reordered and repeated completions, processor failover, metadata copying, rotation, and all outcome-table cases.
- Flush and cancellation without deadlock, leaked reservations, or false completion.
- Abrupt restart without graceful flush, using both an existing persisted checkpoint and no checkpoint.

With an existing checkpoint, pending B records must replay even if later A records finished. Duplicate A delivery is acceptable.

Without a checkpoint, assert the inherited default startup limitation. Do not hide it by seeding every recovery test.

Use local HTTP servers with injected groups and selections. Exercise the real batcher, sender, and destinations with test credentials only.

Check exact recipients, credential headers, and decoded payload contents. Do not contact production intake.

Compare representative throughput, allocations, retained memory, and slow-recipient behavior. A dedicated microbenchmark project is not necessary.

Run Go validation through `dda inv`, not raw Go commands. Preserve these existing regression anchors:

- [Sender tests](../../comp/logs-library/sender/worker_test.go): `TestSenderDualReliableDestination` and `TestSenderReliableContinuseWhenOneFails`.
- [Batch tests](../../comp/logs-library/sender/batch_strategy_test.go): `TestBatchStrategyShouldNotBlockWhenStoppingGracefully`.
- [Provider tests](../../comp/logs-library/pipeline/provider_failover_test.go): `TestFailoverRouterDistribution`.

These tests do not prove the new routing contract by themselves. Add focused routed tests and report any platform validation gaps.
