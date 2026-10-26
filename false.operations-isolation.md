## 1. Verified current behavior: three observations

**Mira Chen — streaming operations**

1. **Shared batching creates shared pressure.** Processors encode records before handing them downstream (`comp/logs-library/processor/processor.go:224–231`). Pipeline strategies use the sender’s input (`comp/logs-library/pipeline/pipeline.go:60`). Batches finish compression before their blocking payload send (`comp/logs-library/sender/batch.go:168–195`). Separate HTTP retry state does not isolate this shared path.

2. **“Reliable” is not an all-destinations guarantee.** The worker seeks reliable acceptance, buffers skipped reliable copies, then attempts unreliable copies; full copy buffers cause drops (`comp/logs-library/sender/worker.go:119–180`). MRF skips can satisfy local `sent` without delivery. MRF uses eligibility metadata, not a general routing contract: the processor sets `IsMRFAllow`, and payload eligibility reads the first message (`comp/logs-library/processor/processor.go:238–251`; `pkg/logs/message/message.go:538–544`). It adds selected secondary forwarding while retaining primary delivery.

3. **Checkpoints describe sources, not org delivery.** Auditor updates use each message’s source identifier and offset (`comp/logs/auditor/impl/auditor.go:289–296`). HTTP completion forwards payloads after non-retried errors too (`comp/logs-library/client/http/destination.go:335–355`). Queue acceptance, completion, and successful intake delivery are different events.

## 2. Proposal: endpoint admission and scheduling

I favor **Candidate B**. Make a stable logical destination ID—not its API key—the admission and scheduling boundary.

```text
processors → encoded immutable records + destination IDs
                          |
                 bounded admission
                  /              \
          endpoint A queue    endpoint B queue
          batch/compress      batch/compress
          HTTP + retries      HTTP + retries

source completion accounting remains a separate concern
```

Use byte-bounded endpoint queues, with a total memory cap covering records, batches, metadata, and in-flight requests. Schedule runnable endpoints fairly within bounded CPU and concurrency budgets. This does not require one new worker per endpoint per processor.

With A unavailable, B-exclusive traffic should not wait behind A’s retry work. For `{A,B}`, B’s copy can progress while A’s copy waits **only while admission capacity permits**. A synchronous “enqueue A, then B” loop would recreate shared blocking.

Endpoint is the correct transport boundary. A source remains the checkpoint and possible collection-backpressure boundary. A processor lane is a compute boundary, not a tenant boundary.

Candidate A saves compression by batching `{A}`, `{B}`, and `{A,B}` separately. But destination-set batching alone gives no isolation: shared dispatch can still block. Even with independent dispatch, `{A,B}` payloads remain retained for A after B finishes. More destinations can also create many active set classes.

**Finite capacity is the hard limit.** Once A’s allowance fills, the system must stop admission, drop according to an explicit contract, or obtain durable storage. Bounded memory, no durable spool, lossless stalled-route delivery, and indefinite healthy-route progress cannot all hold.

For one file containing `A-only, B-only, A-only, B-only`, withholding checkpoints does not stop live reading. It eventually requires bounded retention or collection backpressure. Holding the checkpoint can replay B records after restart; advancing it past an unfinished A record can lose A’s replay opportunity. Replay also depends on source retention. The current auditor cannot track those per-destination obligations.

## 3. Strongest objection to my position

My outage history biases me toward machinery. Candidate B duplicates compression work for fanout, fragments batches, and adds accounting. Small Agents may hit CPU or total memory limits earlier. Endpoint queues offer temporary isolation, not durable delivery. A global cap eventually reconnects their pressure.

## 4. What another persona may miss

A throughput-first engineer may treat independent HTTP clients and retries as sufficient isolation. They are not sufficient if admission blocks before healthy work reaches them. Conversely, my position can hide the source-retention cost of “just pause collection.” Pausing a tailer does not pause the application writing its file.

## 5. Decisive delivery-contract question

When A’s buffer fills, must its unfinished copy prevent checkpoint advancement and eventually pause an interleaved source, or may that copy be explicitly dropped so B can continue?