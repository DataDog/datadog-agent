## Review — Tomas Iversen

### 1. Top three observations: verified current behavior

1. **Compression already precedes distribution.** The processor encodes records before sending them downstream (`comp/logs-library/processor/processor.go:225-231`). The batch closes its compressor and creates a payload before channel delivery (`comp/logs-library/sender/batch.go:160-194`). HTTP requests read `payload.Encoded` (`comp/logs-library/client/http/destination.go:396`). This supports reuse of compressed bytes, not compression per recipient.

2. **Independent retries do not mean independent progress.** Each HTTP destination gets its own client, worker pool, and backoff policy (`comp/logs-library/client/http/destination.go:176-187`). But the shared worker first seeks reliable acceptance, buffers skipped reliable copies, and then tries unreliable destinations (`comp/logs-library/sender/worker.go:119-184`). Acceptance is not intake delivery. MRF skip paths can also satisfy the worker’s local `sent` condition without delivery. This is not an all-reliable delivery guarantee.

3. **Checkpoints do not track each org.** The auditor takes source identifiers and offsets from payload metadata (`comp/logs/auditor/impl/auditor.go:291-305`). Its registry update replaces an entry by source identifier, with an ingestion-time guard (`comp/logs/auditor/impl/auditor.go:382-410`). It does not maintain outstanding destination obligations or durable per-org payload storage.

### 2. Recommendation: shared bytes, separate dispatch

**Proposal:** Favor Candidate A when the number of active destination sets is small. Use the destination set as the **batching unit**, the endpoint as the **dispatch and retry unit**, and the source as the **checkpoint unit**. Processor lanes are execution capacity, not delivery boundaries.

```text
encoded records
       |
bounded set batches: {A}, {B}, {A,B}
       |  serialize/compress once per batch
       v
immutable payload + destination obligations
       |
fair, nonblocking dispatch of ready work
       +--> A queue / requests / retries
       +--> B queue / requests / retries
```

Use stable destination IDs, separate from API keys. Keep payload bytes immutable. Each destination holds a reference and its own completion state. Release bytes after the last obligation is resolved under the approved contract.

Shared compression need not force shared backpressure: **sharing bytes does not require waiting for all recipients before dispatching any recipient.** A can accept `{A,B}` while B retries. The dispatcher must skip blocked work, not wait inside one destination send. Use bounded endpoint and route budgets, with reserved capacity so stalled B references cannot consume the entire shared memory budget.

During a B outage:

- `{A}` can continue while its reserved capacity and source input remain available.
- `{B}` accumulates pending work.
- `{A,B}` can reach A while its B obligation remains pending.

This is **temporary isolation**, not durable delivery. Eventually B’s finite buffer fills. With bounded memory and no durable spool, the system cannot retain B’s data losslessly forever and guarantee indefinite healthy-route progress.

Source collection is another boundary. If A-only and B-only logs alternate in one file, stopping that tailer also stops reading later A logs. Reading ahead can extend isolation only temporarily. Holding the checkpoint before an unresolved B record permits replay if the file remains available, but can duplicate A delivery. Advancing beyond it removes that replay protection. Neither queue topology resolves this contract.

### 3. Strongest objection to my position

Candidate A moves cost into shared ownership and scheduling—my blind spot. A slow recipient pins payload bytes. Reference lifetimes, route fairness, admission, and checkpoint obligations become harder to reason about.

With \(n\) destinations, there can be \(2^n-1\) nonempty destination sets. Many small classes mean more compressors, partial batches, timers, metadata, and weaker compression. Multiple processor lanes multiply this state.

Candidate B is simpler at each endpoint but compresses overlapping records repeatedly. A concrete hybrid is shared-set batching for bounded, common sets and endpoint batching for rare sets, with both paths charged to the same memory limits.

### 4. What another persona may miss

An endpoint-isolation advocate may count queue entries but miss repeated compression CPU and compressor memory. Conversely, my design must count bytes retained by slow references—not merely bytes allocated once.

### 5. Decisive open delivery-contract question

When one destination exhausts its budget, must its unresolved obligation stop source collection and checkpoint advance, or may that destination’s copy be dropped while healthy delivery continues?