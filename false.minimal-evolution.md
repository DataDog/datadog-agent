## Review — Jules Martin

### 1. Top three observations: verified current behavior

- **Correct: MRF is a useful extension point, not a general routing contract.** Selection sets `IsMRFAllow` before encoding (`comp/logs-library/processor/processor.go:220-231,244-251`). The batcher separates eligibility classes. Primary delivery remains. Reusing this classification-and-batching pattern does not require copying whole processing pipelines.

- **Note: independent retries do not remove shared blocking.** Each HTTP destination has its own client, worker pool, and backoff (`comp/logs-library/client/http/destination.go:176-187`). But the worker seeks reliable queue acceptance before buffering skipped copies and trying unreliable destinations (`comp/logs-library/sender/worker.go:117-180`). Acceptance is not HTTP completion. MRF skip paths can also satisfy local “sent” state without delivery. This is not all-reliable delivery.

- **Note: checkpoints belong to sources, not orgs.** The auditor consumes each message’s source identifier and offset (`comp/logs/auditor/impl/auditor.go:290-296`). Its registry update replaces one source entry and rejects older ingestion timestamps (`comp/logs/auditor/impl/auditor.go:384-396`). HTTP completion can follow a permanent drop. A checkpoint proves neither successful intake delivery nor delivery to every selected org.

### 2. Recommendation: a small hybrid, with explicit limits

**Proposal:** keep processing and record encoding shared. Extend MRF-style batching to exact destination sets. Add bounded class-ready slots and independent endpoint queues. Dispatch payload references without waiting on a full endpoint queue.

```text
shared processors / encoding
             |
     exact-set batch classes
       {A}   {B}   {A,B}
             |
    bounded class-ready slots
    nonblocking dispatch + copy state
          /             \
 bounded A queue     bounded B queue
 existing HTTP A     existing HTTP B
```

A pending payload records which endpoint queues accepted its references. This prevents repeated enqueue to A while B waits. Treat payload bytes as immutable. Use stable destination IDs, separate from API keys.

**Endpoint is the retry and queue-budget boundary. Destination-set class is the batching and pending-work boundary. Source remains the collection and replay boundary.** Processor lanes are throughput units, not dependable outage boundaries.

Reserve endpoint budgets so B cannot consume A’s queue capacity. Account for active batches, ready payloads, queued references, and in-flight payloads. Shared bytes still consume memory until the last reference is released. Bound active classes too: destination-set combinations can grow quickly.

**Challenge to A:** exact-set batches alone do not isolate anything. A blocking flush or one common worker can let a B-only payload stop A-only work. Ready slots and nonblocking dispatch address that shared blocking only while capacity remains.

**Challenge to B:** endpoint-specific batching can combine A’s exclusive and overlapping records efficiently, but repeats compression for fanout and adds batch lifecycle state. A sequential fanout loop can still block on B’s full queue before reaching A. Separate queues are necessary, not sufficient.

With B down, this hybrid lets A accept `{A}` and the A copy of `{A,B}` while B’s pending work fits. It does not promise indefinite progress.

### 3. Strongest objection to my position

I favor reuse and can underestimate outage isolation.

If B’s retained input rate is \(r\) bytes/second and its remaining budget is \(M\), its buffer lasts roughly \(M/r\). Then admission must stop, copies must drop, or storage must grow. Bounded memory, no durable spool, lossless stalled-route delivery, and indefinite healthy-route progress cannot all hold.

A lossless stop must propagate to collection, not merely park the common dispatcher. That can stop healthy traffic sharing the source.

### 4. What another persona will miss

A full-pipeline-isolation advocate may miss the file boundary. Suppose one file interleaves B-only and A-only logs. Pausing its reader also stops later A logs. Continuing requires retaining B’s records somewhere.

Holding a checkpoint does not itself pause collection. Advancing it past undelivered B records prevents ordinary restart replay of those records. Holding it back can replay already-delivered A records, if the file remains available. Rotation and retention limit that recovery. Separate pipelines do not create durable per-org progress.

### 5. Decisive open delivery-contract question

When B’s finite budget fills, must collection pause for affected sources—including their A-only records—or may B copies be dropped so collection continues?