# Injected log routing: expected behavior

The PR adds an HTTP routing mechanism. Agent startup does not enable it. A caller supplies delivery groups and selects groups for each record.

## What is a routing group?

A routing group contains a **reliable endpoint set** and optional unreliable endpoints. The reliable set includes the primary and any reliable extras. Each group uses one existing `Sender` and its normal reliable-endpoint behavior.

```text
Group A → organization A
Group B → organization B

Record 1 selects A   → A only
Record 2 selects A+B → both groups
Record 3 selects B   → B only
```

Every selected group must reach a terminal outcome. Within a group, any one endpoint in its reliable set can complete that obligation.

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

- The reliable set can contain **multiple endpoints**. The API designates one as primary.
- **Any one reliable endpoint’s terminal outcome** completes the group. Unreliable endpoints cannot complete it.
- If one reliable endpoint fails, another can satisfy the group. This includes failure of the primary.
- If the entire reliable set stays unavailable, the group remains pending. Healthy unreliable endpoints cannot prevent its source from eventually pausing.
- The sender offers unreliable copies only after reliable admission. Healthy unreliable endpoints might not receive later records during that outage.
- Full buffers can discard unreliable copies. Blocked reliable endpoints, including the primary, can also lose copies when another reliable endpoint accepts work.
- Use **one group** if either reliable endpoint can satisfy delivery. Select **separate groups** if both must finish.

Groups with the same nonempty `stream` share batching and compressed bytes. Every selection must include all groups in that stream.

Delivery uses the existing processor, batch strategy, and HTTP sender. The routing coordinator tracks selected groups and safe source progress.

The caller also owns source startup, replacement, and shutdown. See the [provider API](../../comp/logs-library/pipeline/routed_provider.go) for integration details.

## Selection behavior

Only selected groups receive the record. There is no fallback.

- **Explicit exclusion:** set `Excluded: true` with no groups. The record completes locally. Its checkpoint can pass once earlier records finish.
- **Missing or invalid selection:** report the error. Hold the source checkpoint without delivery or acknowledgement.

Invalid cases include unknown groups or exclusion combined with recipients. No recipients without explicit exclusion is also invalid.

This prevents a broken decision from silently skipping a log. Recovery needs corrected selection and replay of retained input from a safe checkpoint.

**Queue acceptance is not delivery.** Retryable failures, credential waits, and cancellation leave selected groups pending.

Existing permanent HTTP rejection and local processing-loss policies remain terminal. A terminal outcome does not always mean successful delivery.

## Outages and restart

Both paths keep one checkpoint per source. Unlike the default path, routed delivery waits for every selected group before advancing past a record.

File X selects A+B. File Y selects B only.

Here, **A stalls** means no reliable endpoint in group A can complete the pending deliveries.

- **A stalls:** B receives X while capacity remains. The checkpoint for X waits for A.
- **X reaches its limit:** pause admission from X to both groups. Existing retries continue without overflow drops.
- **Y remains healthy:** continue delivery through B and advance its checkpoint while aggregate capacity remains.
- **A recovers:** completed progress frees capacity and X resumes automatically.
- **Aggregate capacity fills:** other sources can pause too.
- **Agent restarts:** B can receive duplicates because A held the checkpoint.

Retries and file replay are existing behavior. Source-specific admission and capacity release are new routed behavior.

Replay needs retained input and an older saved checkpoint. Without one, existing end-mode behavior can skip pending data.

## New limits introduced by this PR

These bounds apply only to routed delivery. They do not change the default path.

- **Configurable groups:** `logs_config.routing.max_groups` defaults to 4.
- **Retained records per checkpointed source:** 64.
- **Aggregate progress:** 4096 records, 4096 source owners, and 4096 generation-history entries.
- **Endpoints, including extras:** 32.
- **Processing lanes:** 16.
- **Concurrent HTTP requests across all endpoints:** 64.

Only the group limit is customer-configurable. The other bounds are code constants.

Capacity exhaustion applies backpressure, not new selected-group drops. These bounds count references and concurrent work, not total memory in bytes.

Definitions: [progress limits](../../comp/logs-library/routing/progress.go), [endpoint limit](../../comp/logs-library/routing/definitions.go), and [provider limits and queues](../../comp/logs-library/pipeline/routed_provider.go). Existing sender and batch limits remain separate.

There is no durable spool, per-endpoint saved checkpoint, or exactly-once guarantee.
