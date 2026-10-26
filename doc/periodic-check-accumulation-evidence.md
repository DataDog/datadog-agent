# Periodic check accumulation: review evidence

This is a draft design comparison, not production release approval. The measured
implementation is commit `d0b816d491558c17b2e52b433e97ccbf5c3fb481`,
based on `73f0a2f757d31e6a566722c9103754f73138125d`. Publication adds
documentation, a release note, and review cleanup; it does not substitute a new
performance run. See [behavior and limitations](periodic-check-accumulation.md).

## Provenance and validation

The original artifacts remain on `workspace-jmp-sqlserver-periodic` under
`/home/bits/ddscale/periodic-advocate-20261019/counter-rfc-corrections`.
They are workspace artifacts, not public CI artifacts.

- Latest conclusions: `PRODUCTION-PARITY.md`, `DECISIONS.md`,
  `v7-baseline/BASELINE-CORRECTION.md`. Their correction supersedes earlier
  claims that the enabled four-build comparison included batching-off controls.
- Measured source diff: `v6/production-from-common-parent.patch`, SHA256
  `7be8c4d7e5cfb7319cebe754d2e17002ddcb7c4b01ff6c7f379e514d8a39b992`.
- Measured Agent SHA256:
  `cc8149e0c5313635ea438a960fe7647e5862e8212be0778f413cbf486aa472f8`.
  Bridge hashes and source identities: `v6/resume-source-binary-audit.json`.
- Full aggregator/metrics ordinary and race suites, native Linux amd64 build,
  five native diagnostics, and histogram-value audit passed at this revision:
  `v6/{tests,race,build,diagnostics,native-histogram-audit}.exit` are all zero.
- Performance/accounting: `v6/sql-equal-work-analysis.json`,
  `v6/sql-comparison-audit.json`,
  `v7-baseline/combined-analysis/baseline-cpu-comparison.json`,
  `v7-baseline/no-batching-receipt-audit.json`, and
  `v7-baseline/final-analysis-inputs.sha256`.
  Reproduction scripts include `v6/validate-analysis.py`,
  `v7-baseline/combine-analysis.py`, and
  `v7-baseline/audit-no-batching-receipts.py`.

## Matched SQL Server fixture

300 databases, 600,000 indexes, three heavy collectors; native amd64 Agent,
4 CPUs, 4 GiB/no swap, accepting local HTTP intake. Runtime image, integration,
query cadence, cardinality, probe configuration, and limits match across arms.
Size-enabled arms use the batching map; corrected size OFF uses the exact same
binary with an empty map, disabling size commits and submission GIL release.

| Build / mode | First sweep CPU-s | Two sweeps CPU-s | Whole-trial peak cgroup memory / outcome |
| --- | ---: | ---: | --- |
| Original parent, no batching | 121.06–126.61 | 283.29–290.01 | 4 GiB limit; OOM |
| Corrected size OFF (`8984ad46`) | 120.90–122.76 | 284.97–296.18 | 4 GiB limit; OOM |
| Corrected size ON (`8984ad46`) | 161.30–162.62 | 375.47–376.86 | 1.135 GB; normal-intake PASS |
| Periodic v6 (`d0b816d4`) | 139.81–140.85 | 313.15–313.92 | 1.627 GB; normal-intake PASS |
| Pinned #56549 ON (`8e64aae6`) | 168.72–170.12 | 382.44–383.83 | 1.145 GB; normal-intake PASS |
| Reference build (`e50e2592`) | 170.86–173.37 | 387.66–390.08 | 1.227 GB; FAIL final probe |

GB is decimal bytes/1e9; GiB is binary. OOM peaks are censored by the limit,
not estimates of the memory needed to finish. Original and corrected OFF exited
137 with Docker OOMKilled=true after 919.28/931.37 seconds. Neither reached
three sweeps.

CPU is Agent-process user+system time from the first heavy-collection start to
the named completed collection boundary, including concurrent probe/background
work and idle intervals between sweeps, excluding SQL Server CPU. One sweep is
4,836,900 submissions; two are 9,673,800. Equal work is supported by source query
cadence, immutable configuration/cardinality, fixed positive query contributions,
and zero check errors/warnings, not counts alone. Bounds account for resource
sampling and truncated CPU counters; they are **not confidence intervals**.
One trial per arm in fixed order, with off controls appended later, does not
quantify variability or order effects.

At two sweeps, periodic uses 5.7–10.2% more CPU than corrected size OFF and
8.0–10.8% more than the original parent; size ON uses 26.8–32.2% and 29.5–33.0%
more respectively. Overlapping off/original bounds do not prove zero disabled
overhead universally.

**Equal collected work is not equal delivered work.** Both off controls lost
first-output points before OOM: 201,488/201,510 missing contexts/points exactly
match PointsDroppedCount. Each recorded 20 dropped transactions, 133
HighPriorityQueueFull events, and 113 retries, with zero HTTP/ACK errors.
Raw receipt replay confirms the losses. There is no complete first-delivery
endpoint for either off control, and no lossless delivered-work CPU comparison.

## Enabled-design tradeoff

| Measurement | Periodic v6 | Corrected size ON |
| --- | ---: | ---: |
| Three collected sweeps CPU-s | 479.59–480.70 | 591.19–592.58 |
| First collection plus expected output receipt CPU-s | 140.41–141.20 | 161.30–162.62 |
| First collection plus expected output receipt elapsed-s | 326.32–327.32 | 316.60–318.52 |
| Peak cgroup memory, GB | 1.627 | 1.135 |
| Process RSS high-water mark, GB | 1.717 | 1.244 |
| Maximum unrelated probe scheduling gap, s | 4.041 | 1.015 |
| Probe delivery delay p95 / maximum, s | 14.757 / 23.287 | 10.446 / 14.446 |

Periodic uses about 19% less CPU through three collected sweeps and about 13%
less through complete first-output receipt, with roughly 9 seconds longer first
delivery, 43% more peak cgroup memory, and longer probe pauses. Peaks for both
were reached by the same three-sweep boundary. All 13 first-output context
fingerprints match, including 900 histogram operation observations. Later-sweep
delivery is not inferred from collection endpoints; unequal whole-trial CPU
totals are not used as equal-work results. Individual SQL collection timestamps
were not recorded. Probe delivery delay measures the probe, not SQL freshness.

## Failed gates and release questions

Both enabled designs fail sustained two-second-response intake/shutdown delivery
(`v6/continuous-fault-review.json`). Periodic receives all 2,000,000 count
contributions but misses 726,713 final gauges and 15 probes; corrected size
receives 650,000 counts and misses 1,345,095 gauges and 55 probes, also failing
completion. Bounded recovery passes both designs. The shared HTTP/1 connection
queue, 20-second client timeout, retries, and 10-second forwarder shutdown
deadline support the observed backlog/loss mechanism; they do not waive failures
or prove timestamp collisions. Handoff to the forwarder is not delivery.

The customer arm's missing final probe is independently confirmed from raw
receipts and remains a failure; its latency quantiles exclude that missing probe.
Normal-intake success for periodic does not establish lossless shutdown under
sustained backlog. Large clock rollback can produce future-dated generated
numeric points. Real-intake/query acceptance, automatic global rollout and
rollback policy, broader integrations, and Windows/macOS coverage remain open.
For memory protection first, the recorded recommendation remains corrected
size; periodic is a credible alternative when its CPU/automatic-operation
benefits justify these memory and responsiveness costs.
