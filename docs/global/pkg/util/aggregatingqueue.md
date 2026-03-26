# pkg/util/aggregatingqueue

**Import path:** `github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue`

## Purpose

`pkg/util/aggregatingqueue` provides a generic, buffering queue that accumulates individual items and flushes them as a batch when either a maximum item count is reached or a maximum retention time has elapsed. This pattern is used to batch small, frequent events (e.g. SBOM or container image payloads) before sending them over an event-platform forwarder, reducing overhead and serialization cost per payload.

The queue runs a single background goroutine and is driven entirely through a channel, so it is safe to enqueue from multiple goroutines concurrently.

## Key Elements

### `NewQueue[T any]`

```go
func NewQueue[T any](
    maxNbItem        int,
    maxRetentionTime clock.Duration,
    flushCB          func([]T),
) chan T
```

Creates the queue and returns an **enqueue channel**. Callers send items to this channel; closing the channel terminates the background goroutine.

**Parameters:**
- `maxNbItem` — flush immediately when this many items have accumulated since the last flush.
- `maxRetentionTime` — flush at most this long after the first item arrives after a flush, even if `maxNbItem` has not been reached.
- `flushCB` — called synchronously in the background goroutine with a slice of accumulated items. The slice must not be retained after `flushCB` returns.

**Flush semantics:**
- Flush happens on whichever condition fires first: item count reaches `maxNbItem`, or the retention timer fires.
- The timer is started when the first item arrives after a flush, and stopped on every flush, so idle queues do not fire empty flushes.
- After a flush the backing slice is reset to a fresh allocation with capacity `maxNbItem`.

## Usage

### SBOM processor (`pkg/collector/corechecks/sbom/processor.go`)

```go
import queue "github.com/DataDog/datadog-agent/pkg/util/aggregatingqueue"

p.queue = queue.NewQueue(maxNbItem, maxRetentionTime, func(entities []*model.SBOMEntity) {
    encoded, err := proto.Marshal(&model.SBOMPayload{Entities: entities, ...})
    if err != nil {
        log.Errorf("Unable to encode message: %+v", err)
        return
    }
    sender.EventPlatformEvent(encoded, eventplatform.EventTypeContainerSBOM)
})

// Enqueue an entity from any goroutine:
p.queue <- entity
```

### Container image processor (`pkg/collector/corechecks/containerimage/processor.go`)

Uses the same pattern to batch `*model.ContainerImagePayload` entities before forwarding them to the event platform.

### Shutdown

Close the channel returned by `NewQueue` to terminate the background goroutine cleanly. Any items buffered at the time of closure are **not** flushed — the caller is responsible for draining or discarding them before closing.

## Cross-references

| Topic | See also |
|-------|----------|
| SBOM check that is the primary consumer of this queue — batches `*model.SBOMEntity` before forwarding to the event-platform forwarder | [pkg/sbom](../sbom.md) |
| Container-image check that uses the same batching pattern for `*model.ContainerImagePayload` | [pkg/collector/corechecks](../collector/corechecks.md) (see `containerimage` sub-package) |
| How the encoded payload arrives at the Datadog intake after `sender.EventPlatformEvent` is called | [comp/forwarder/eventplatform](../../comp/forwarder/eventplatform.md) |
