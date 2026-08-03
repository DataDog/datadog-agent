# Timestamp-aware log count view POC

## Result

A five-second timestamp-aware count view is the strongest configuration tested.
It improves mean F1 by 17.79 percentage points over sparse event points and by
9.34 points over the earlier materialized one-second regularization, while
keeping baseline false positives much closer to the sparse control.

| Detector | Sparse | Materialized 1s | View 1s | View 5s | View 10s |
|---|---:|---:|---:|---:|---:|
| Holt | 21.88% | 27.67% | 14.52% | **40.01%** | 7.99% |
| ScanMW | **34.02%** | 25.72% | 15.80% | 31.84% | 24.11% |
| ScanWelch | 20.58% | 30.73% | 23.70% | **44.37%** | 23.00% |
| Tukey | 16.91% | 17.52% | 15.04% | **38.08%** | 4.40% |
| BOCPD | 13.41% | **47.42%** | 34.18% | 41.45% | 0.00% |
| **Mean** | **21.36%** | **29.81%** | **20.65%** | **39.15%** | **11.90%** |

Baseline false positives across the 12 scenarios:

| Detector | Sparse | Materialized 1s | View 1s | View 5s | View 10s |
|---|---:|---:|---:|---:|---:|
| Holt | 17 | 72 | 67 | 31 | 0 |
| ScanMW | 4 | 10 | 12 | 16 | 17 |
| ScanWelch | 1 | 2 | 3 | 2 | 6 |
| Tukey | 8 | 6 | 8 | 3 | 1 |
| BOCPD | 23 | 53 | 48 | 15 | 2 |
| **Total** | **53** | **143** | **138** | **67** | **26** |

The five-second view is not uniformly better. Kafka regresses substantially
and Cassandra is slightly worse than the sparse control. The no-runtime-log
control remains effectively zero with the view, whereas materialized
regularization scored 15.76% there. That control is useful evidence that
avoiding historical zero backfill reduces unrelated near-onset detections.

| Scenario | Sparse mean F1 | Materialized 1s | View 5s |
|---|---:|---:|---:|
| Block-building outage | 25.24% | 3.62% | **49.37%** |
| Byzantine-fault cascade | 0.00% | 9.23% | **23.18%** |
| Cascading payment failure | 0.00% | 2.76% | **28.80%** |
| Cassandra repair degradation | **39.17%** | 27.66% | 32.65% |
| DNS upstream outage | 3.71% | 2.22% | **32.63%** |
| Kafka partition saturation | 49.84% | **58.47%** | 16.18% |
| Lock contention | 50.92% | 38.08% | **64.43%** |
| Memcached saturation | 52.43% | 73.20% | **75.78%** |
| Pool saturation | 7.46% | 56.46% | **64.08%** |
| Redis billing cascade | 18.57% | 42.52% | **64.42%** |
| Redis CPU saturation | 8.97% | **27.77%** | 18.28% |
| Tiered-cache control | 0.00% | 15.76% | **0.00%** |

## What the view does

The implementation leaves shared Observer storage sparse. For log-extractor
count series only, it exposes completed fixed-duration buckets to detectors:

- sum observations in each bucket;
- expose a zero for an empty bucket after a series has first appeared;
- never backfill zeros before discovery;
- stop forwarding zeros after 300 seconds of inactivity; and
- reactivate when a new observation arrives.

Ordinary metric series retain their existing missing-data semantics. The POC
implements this as a testbench-only `StorageReader` adapter so the experiment
does not change production ingestion.

This helps because the detectors receive an elapsed-time count/rate view rather
than interpreting irregular event arrivals as evenly spaced values. A five
second bucket also aggregates bursts and avoids the extreme zero inflation of a
one-second view.

## Why cadence matters

The current detector hyperparameters are expressed in points, not elapsed
time. At ten-second cadence, for example:

- BOCPD's 120-point warmup becomes 20 minutes;
- Tukey's 80 points become 13.3 minutes; and
- a 30-point scan minimum becomes 5 minutes.

That explains much of the ten-second collapse. The POC makes log counts
timestamp-aware at the storage boundary, but a production design should also
express detector warmups, windows, and confirmation periods in seconds.

## Cost

The view adds no synthetic points to shared storage. Across the 12 scenarios,
the five-second runs retained roughly 110k–124k raw stored points, depending on
detector muting. The earlier materialized implementation generated 2,437,582
points from 737,519 extractor inputs before storage.

This does not mean the POC is free:

- detector-local buffers can retain virtual buckets;
- the adapter currently reconstructs buckets on every read;
- ScanMW and ScanWelch reread overlapping ranges, producing roughly 33 million
  logical observations across the evaluation; and
- logical-observation counts are reads, not retained-memory counts.

The implementation therefore proves the accuracy and shared-storage shape, not
production CPU or heap cost. Before production, cache bucketized ranges,
remove evaluation-only full-series instrumentation, and benchmark the selected
view under production-like cardinality.

## Experiment setup

- 747,359 logs from 12 scenarios
- fuzzy Logs-tokenizer matching at 0.5
- logs only
- five matching logs required before series emission
- Holt, ScanMW, ScanWelch, Tukey, and BOCPD
- five-minute baseline with noisy-series muting
- `time_cluster` correlation
- Gaussian temporal F1 with sigma 30 seconds
- bucket widths of 1, 5, and 10 seconds
- idle TTL of 300 seconds

Raw scorer outputs are retained on the local stacked experiment branch rather
than committed here. Sparse and materialized-control values in the comparison
tables came from that paired tokenizer/regularization experiment; those control
implementations are intentionally not included in this focused PR.

## Recommendation

Continue with the timestamp-aware read view, using five seconds as the current
candidate. The next experiment should:

1. convert detector point-count hyperparameters to elapsed-time durations;
2. explain the Kafka and Cassandra regressions at the dominant-series level;
3. cache or incrementally maintain virtual buckets to bound detector read work;
4. sweep a shorter idle TTL; and
5. benchmark CPU, heap, retained detector state, and false positives before
   considering production integration.
