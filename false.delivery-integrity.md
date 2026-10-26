## Review — Anika Rao

### 1. Correct: three verified observations

1. **Queue acceptance is not destination delivery.** The worker seeks reliable acceptance, then buffers skipped reliable copies and tries unreliable copies (`comp/logs-library/sender/worker.go:120-180`). `DestinationSender.Send` accepts into a channel; disabled MRF can return success without sending (`comp/logs-library/sender/destination_sender.go:100-131`). This cannot establish completion for each selected org.

2. **Batching comes before endpoint distribution.** Payload bytes and message metadata are separate fields (`pkg/logs/message/message.go:33-41`). MRF assumes all messages in a payload share eligibility (`pkg/logs/message/message.go:538-544`). Candidate A extends this batching model, but destination-set classification alone does not isolate dispatch or checkpoints.

3. **Auditor progress is not delivery proof.** Registry entries store offsets and timestamps, not per-org completion (`comp/logs/auditor/impl/auditor.go:37-50`). HTTP handling forwards payloads after non-retryable errors as well as success (`comp/logs-library/client/http/destination.go:339-355,458-465`). Current behavior is neither all-selected-destination delivery nor durable per-org spooling.

These observations concern the inspected checkout, using research baseline `9d8ea371d02`; I did not independently verify the Git revision.

### 2. Note: recommended architecture and isolation boundary

**Proposal:** Prefer Candidate B. Use the destination as the queue, retry, and transport isolation unit. Use the source as the recovery-accounting unit. A processor lane is neither.

```text
processed, encoded record + selected destination IDs
                    |
           bounded independent admission
              /                     \
      A queue -> batch/compress   B queue -> batch/compress
              -> HTTP A                  -> HTTP B
              \                         /
           per-record destination outcomes
                         |
              source progress accounting
```

Use stable destination IDs, separate from API keys. Record success, pending work, and explicit drops separately. A success at A must never mark B delivered. Whether a drop permits checkpoint advancement remains a contract decision.

Give destinations byte budgets within an aggregate memory limit. A stalled B must not consume A's entire allowance. A dispatcher that blocks while enqueueing B still blocks later A-only records, despite separate retry state. Independent admission must therefore stop short of silently choosing what happens when B fills.

With B down, A-only and the A side of `{A,B}` can progress **temporarily**. B-only and the B side remain pending. This does not establish fanout completion.

For one file, consider record end offsets:

```text
100:{A}, 200:{B}, 300:{A}, 400:{A,B}
```

Under a proposed all-selected-success rule, B's outage holds the safe contiguous checkpoint at 100, even if A delivers 300 and its copy of 400. Advancing to 300 would skip B's missing record on restart. Restarting from 100 can replay 300 to A, causing duplicates. Tracking destination outcomes helps explain these holes; an in-memory tracker does not survive a crash.

Holding a checkpoint does not itself stop file reading. Continued collection needs bounded storage for pending work. Once that storage fills, source backpressure can also stop later A-only records in the same file. The application may continue writing and rotating files. Source files are not a guaranteed spool.

### 3. Strongest objection to my position

B duplicates compression and can produce smaller batches. A can compress each exact destination class once and share immutable bytes. That is a real efficiency advantage.

My recovery-first bias can also imply retention that the Agent does not provide. Neither candidate can combine bounded memory, no durable spool, lossless stalled-route delivery, and indefinite healthy-route progress. At exhaustion, the contract must permit blocking, explicit loss, or another storage mechanism.

### 4. What another persona may miss

A transport-focused persona may see independent clients and retries as sufficient isolation. They may miss shared admission blocking and source checkpoint holes across interleaved routes. Conversely, I must not present stricter recovery accounting as an existing guarantee.

### 5. Decisive open delivery-contract question

When B exhausts its buffer, must B's selected obligations remain pending and eventually backpressure the source, or may explicit best-effort drops release those obligations and permit checkpoint advancement?