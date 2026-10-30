# Injected log routing: expected behavior

The PR adds an HTTP routing mechanism. Agent startup does not enable it. A caller supplies delivery groups and selects groups for each record.

## Design decisions

- Keep routing caller-injected in this PR; defer user-facing routing policy and Agent startup activation.
- Reuse processing, batching, compression, HTTP senders, and the auditor. Add coordination for delivery groups and ordered source progress.
- Require every selected group to finish before advancing one persisted source checkpoint. Queue acceptance is not completion.
- Require separate `SourceInput(identifier, generation)` channels for source isolation. Shared `NextPipelineChan` and `NextPipelineChanWithMonitor` ingress can block healthy sources behind a stalled source; existing launchers need explicit integration to gain isolation.
- Reserve producer generation ownership when `SourceInput` is created, before the first record. Allow older, unretired producers to drain after replacement, without reclaiming current checkpoint ownership. Report and discard records from retired producers without adding a source barrier.
- Distinguish explicit exclusion from missing or invalid selection. Exclusion completes locally; a missing or invalid decision holds the checkpoint until correction and replay. Barriers belong to the source, not a producer generation, so producer retirement can still clean up its input.
- Bound pending records and source history. Aggregate exhaustion can pause all sources, even with separate inputs.
- Keep group progress in memory and one saved checkpoint per source. Restart can duplicate delivery; this PR adds no durable spool or per-group persisted checkpoint.

## What is a routing group?

A routing group contains one reliable primary endpoint and optional extra endpoints. It uses one existing `Sender` and its normal reliable-endpoint behavior.

```text
Group A → organization A
Group B → organization B

Record 1 selects A   → A only
Record 2 selects A+B → both groups
Record 3 selects B   → B only
```

Every selected group must reach a terminal outcome. Within each group, any one reliable endpoint’s terminal outcome completes that group’s obligation.

To make both destinations mandatory, place them in separate groups. Unreliable extras cannot complete a group.

## Configuration and caller inputs

This single structure shows the setting and caller inputs. **Only `logs_config.routing.max_groups` is actual YAML configuration.**

The `caller_inputs` section is schematic, not an accepted configuration format. The caller supplies those values through the Go API.

```yaml
logs_config:
  routing:
    max_groups: 4 # New setting. Caps groups but does not activate routing.

caller_inputs: # Schematic representation of new Go API inputs.
  groups:
    - id: a
      endpoints: <validated HTTP endpoint set for organization A>
      stream: "" # Optional. Share only explicitly declared identical streams.
    - id: b
      endpoints: <validated HTTP endpoint set for organization B>
      stream: ""
  selection: # One resolved decision per record, not a routing rule.
    groups: [a, b] # Select A only, B only, or both.
    excluded: false # Use true with no groups for intentional exclusion.
    generation: 1 # Change for a replacement source producer.
```

Definitions: [configuration schema](../../pkg/config/schema/yaml/logs_config.yaml), [`Group` and `Selection`](../../comp/logs-library/routing/definitions.go).

Groups with the same nonempty `stream` share batching and compressed bytes. Every selection must include all groups in that stream.

Delivery uses the existing processor, batch strategy, and HTTP sender. The routing coordinator tracks selected groups and safe source progress.

The caller also owns source startup, replacement, and shutdown. Source isolation requires a separate `SourceInput(identifier, generation)` per producer. Shared `NextPipelineChan` and `NextPipelineChanWithMonitor` ingress can block healthy sources behind a stalled source; this PR does not add isolation to that interface or reject records arriving there. Existing launchers need explicit integration to use isolated inputs. See the [provider API](../../comp/logs-library/pipeline/routed_provider.go) for integration details.

After a producer stops and its output is admitted, call `RetireSource`. Pending records must finish before retirement succeeds. An invalid-selection barrier retains source ownership independently, but does not prevent retirement once the producer’s delivery obligations have completed. This includes a producer whose first record is invalid and records that completed behind an unresolved barrier. Completed records can remain retained to preserve checkpoint ordering after the producer input is retired. Start the auditor before the provider; stop producers before stopping the provider.

## Selection behavior

| Caller decision | Result |
| --- | --- |
| A only or B only | Only the selected group receives the record. No fallback. |
| A+B | Both groups must finish. B may finish while A remains pending. |
| `Excluded: true`, no groups | Intentional exclusion. The record completes locally. Its checkpoint can pass it once earlier records finish. |
| No groups, `Excluded: false` | Missing decision. Hold the checkpointed source at this record without delivery. |
| Invalid selection | Report the error and hold the checkpointed source at this record. Do not infer recipients or acknowledge it. |

An unknown group or exclusion combined with recipients makes a selection invalid.

These cases distinguish intentional exclusion from a missing or broken decision. An unresolved selection barrier has no automatic repair path.

Recovery needs corrected selection and retained input replay from a safe checkpoint.

**Queue acceptance is not delivery.** Retryable failures, credential waits, and cancellation leave selected groups pending.

Existing permanent HTTP rejection and local processing-loss policies remain terminal. A terminal outcome does not always mean successful delivery.

## Checkpoints during endpoint outages

This section answers two questions: when does a source pause, and can other sources continue?

**Existing on both paths:** the auditor saves one checkpoint per source, not per endpoint.

**New on the routed path:** a coordinator holds that checkpoint behind any earlier record with unresolved selected-group obligations.

The default path has no cross-group obligation ledger. One reliable endpoint’s terminal outcome can advance its checkpoint while another endpoint remains unavailable.

```text
File X → groups A and B
File Y → group B only
```

The X/Y cases below describe the routed path with separate `SourceInput` channels for X and Y. A and B are separate selected groups, not extras inside one endpoint set.

| Behavior case | New or inherited? | Delivery and checkpoint result |
| --- | --- | --- |
| A and B stay healthy | New routed coordination | Each source advances after its selected groups finish. |
| A becomes unavailable, B stays healthy | New routed coordination | B receives X’s records while capacity remains. X’s checkpoint stays behind unresolved A deliveries. |
| X reaches its source limit | New routed admission limit | X pauses without overflow drops. B then receives no newly admitted X records. |
| X pauses, Y still has capacity | New routed source isolation | Y continues through B and advances its checkpoint while aggregate capacity remains available. |
| A recovers | Existing HTTP retries, new routed capacity release | Terminal outcomes release X’s completed prefix and capacity. X resumes automatically. |
| Aggregate capacity fills | New routed aggregate limits | Other sources, including Y, can pause too. |
| Agent restarts | Existing replay mechanism, new cross-group replay consequence | B can receive duplicates because A held X’s checkpoint. Replay needs retained input and an older saved checkpoint. |
| No usable saved checkpoint exists | Inherited on both paths | Existing end-mode behavior can skip pending data. Routing adds no initial-checkpoint guarantee. |

X’s checkpoint cannot bypass an earlier pending record, even if a later record selects only healthy B.

Independent checkpoints do not mean unlimited isolation. Aggregate capacity exhaustion can pause otherwise healthy sources too.

## Flush and shutdown

`Flush(ctx)` waits for earlier admitted records to reach batching, not for HTTP delivery. Cancelling the call releases its dispatcher waits so an abandoned flush does not keep healthy sources blocked behind a stalled source.

`Stop` allows accepted work five seconds to finish, then cancels the provider's routing workers and HTTP requests without acknowledging unfinished delivery. Each provider owns its HTTP cancellation context: stopping it does not cancel a caller-supplied context shared with other senders. Cancellation of the caller's context still stops the provider.

## New limits introduced by this PR

**These are new routed-path limits, not inherited default-path limits.** The PR sets these values in code, not customer configuration.

The default path does not gain these limits. The separate `max_groups` setting above is also new and defaults to four.

| Internal limit | Value | Definition |
| --- | --- | --- |
| Pending records per checkpointed source | 64 | `routing.SourcePendingLimit` in [progress.go](../../comp/logs-library/routing/progress.go). Counts retained records, not file size. |
| Aggregate progress | 4096 records, 4096 source owners, and 4096 generation-history entries | `routedProgressCapacity` in [routed_provider.go](../../comp/logs-library/pipeline/routed_provider.go), enforced by `Progress`. |
| Total endpoints, including extras | 32 | `routing.MaxEndpoints` in [definitions.go](../../comp/logs-library/routing/definitions.go). |
| Processing lanes | At most 16 | Validation in `NewRoutedProvider`. |
| Concurrent HTTP requests | At most 64 across all endpoints | `routedHTTPRequests` in `routed_provider.go`. |

These limits bound retained references and concurrent work, not total process memory in bytes. Overflow causes backpressure, not new selected-group drops.

The PR also adds bounded routing queues. Existing sender and batch limits remain separate. See `routed_provider.go` for the new queue constants.

There is no durable spool, per-endpoint saved checkpoint, or exactly-once guarantee.
