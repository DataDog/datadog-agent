# Observer hourly seasonality POC

Can the Agent stop alerting on expected recurring workloads while still detecting
an unexpected change? This Innovation Week experiment adds a seasonal input layer
in front of BOCPD and measures the detection benefit and resource cost.

This directory is a **reproducible research POC**, not production integration.
It contains the tested source, prerequisites, tests and aggregate results. Nothing
here is compiled into the Agent. The runner creates a separate detached checkout
at the recorded Agent revision; it never installs the experiment into your working
checkout. A production implementation would be a separate change.

## What to review

- `source/seasonal_real_model_research.go.txt`: bounded history, periodic profile,
  prediction-error and lag-correlation qualification.
- `source/seasonal_real_adapter_research.go.txt`: causal slot aggregation,
  normalization, BOCPD calibration and shared temporary buffers/readers.
- `source/seasonal_admission_research.go.txt`: FIFO learner budget and inactivity
  release. Qualification is separate from admission to a learner slot.
- `source/*_test.go.txt`: causality, raw fallback, storage corrections, eviction,
  history compression, learner limits and detector-state preservation.
- `results/`: aggregate measurements and validation receipts, without recordings.

The `.go.txt` suffix keeps experiment sources out of recursive Go builds. The
runner verifies their hashes and copies them into the pinned research checkout.
Historical comparison modes remain because the preservation tests exercise them;
the configuration below selects the six-hour automatic-discovery experiment.

## Algorithm

1. Average observations into 15-second positions. An hourly template has 240
   positions. Fit a median at each position across three training hours.
2. Validate the whole periodic profile on the following two hours. Require at
   least 50% lower squared prediction error than the better constant predictor
   (the training mean or the last training-hour mean), and lag correlation of at
   least 0.6 across the five-hour history. Candidates are 30 minutes and one hour.
3. Freeze the profile and discard training history. Use one more hour of actual
   observations to calibrate and warm BOCPD on adjusted inputs.
4. After six hours, subtract the phase median and divide by the learned residual
   scale before detection. The scale combines phase-specific residual variation
   with a pooled noise floor. Until activation, ordinary detection continues.

History is bounded and constant histories have an exact compact representation.
The 1,200-series resource experiment also allows 1,200 concurrent learners.
Rejected series do not automatically retry, and activated profiles do not adapt
to later pattern changes. Those are limitations, not production policies.

## Findings

The workload used the GenSim SessionCache backend with a custom driver. Every
hour it created 500 Redis/Valkey sessions with a 90-second TTL. In the fault
recording, the 14th burst lost sessions prematurely. Healthy and fault recordings
each ran for 20 hours and learned independently; no healthy-trained model was
transferred to the fault recording. Seven counter series were converted to rates
in both replay arms before detection.

After the six-hour learning/calibration period:

| Recording | Raw high-severity episodes | Seasonal high-severity episodes |
|---|---:|---:|
| Healthy | 4 | 0 |
| Fault | 7 | 1 |

The remaining seasonal episode detected the fault, after 14.1 seconds versus
4.1 seconds for raw detection. Medium-severity time increased. This demonstrates
the targeted suppression behavior on this workload, not generally better anomaly
detection. Raw detection also sometimes fell below the high-severity threshold
on later healthy bursts; it did not stop emitting detector anomalies.

### Resource cost

The same recorded 11-hour fixture contains 258 source series, replicated
deterministically to 1,200. These are storage-and-detection measurements, not
whole-Agent CPU or RSS. Heap is retained Go heap after GC, excluding the preloaded
input fixture. CPU is process user+system time, including profiling overhead.

| Stage | Additional heap | CPU relative to raw |
|---|---:|---:|
| Learning | 8.56 MiB (about 1.81× total heap) | 2.06× |
| Calibration | 3.46 MiB (about 1.33× total heap) | 1.88× |
| After activation | 2.74 MiB (about 1.25× total heap) | 1.88× |

These updated CPU ratios use three matched runs of every variant in balanced
order. They supersede earlier comparisons against a separately measured baseline.
At maturity, 98 of the 1,200 replicated series qualified. Cost will depend on the
workload and qualification rate; these ratios are not universal scaling laws.

An ablation keeps the prototype's per-series detectors and readers but disables
seasonal processing. Its detector emissions exactly match raw detection across
the fixture. After activation, this integration accounts for about 62% of the
CPU increase; enabling seasonal behavior accounts for about 38%. The latter also
includes changes in BOCPD computation and GC caused by the adjusted inputs. It is
not a pure measurement of seasonal arithmetic or a cost of calibration alone.

## Reproduce

Use Linux with the Agent development environment and `dda`. The pinned revision
is `83a564c77e936dd5255a5a2e319742c0be2c144f`; it must exist in your local Git
object database. Fetch that revision if needed. Commands below assume this
directory is the current directory and that `/tmp/observer-seasonality` is new.

```sh
python3 run.py prepare --checkout /tmp/observer-seasonality
python3 run.py test --checkout /tmp/observer-seasonality
python3 run.py build --checkout /tmp/observer-seasonality
```

Tests run without the recorded workload. The suite checks the original ten-hour
schedule where older tests use fixed boundaries, then checks the six-hour schedule
explicitly. All Agent compilation/testing goes through `dda inv`.

For a resource comparison, provide the fixture described in `FIXTURE.md`:

```sh
python3 run.py profile --checkout /tmp/observer-seasonality \
  --fixture /path/to/fixture --output /tmp/seasonality-raw-1 --arm raw
python3 run.py profile --checkout /tmp/observer-seasonality \
  --fixture /path/to/fixture --output /tmp/seasonality-structure-1 --arm structure
python3 run.py profile --checkout /tmp/observer-seasonality \
  --fixture /path/to/fixture --output /tmp/seasonality-full-1 --arm automatic
```

Repeat in the orders `raw, structure, automatic`, `structure, automatic, raw`,
and `automatic, raw, structure`. Separate runs with `--validate-emissions` emit
per-stage hashes for raw/structure parity; do not mix those runs into timings.
Each output contains stage CPU/heap measurements and a `whole.cpu.pprof` profile.

For a testbench replay, build as above and set these environment variables when
running the resulting `bin/anomalydetection-testbench` with your scenario/config:

```sh
export OBSERVER_REAL_SEASONAL_MODE=phase_pooled_floor
export OBSERVER_REAL_SEASONAL_DISCOVERY=all
export OBSERVER_REAL_SEASONAL_LEARNING_PLAN=3-2-1
export OBSERVER_REAL_SEASONAL_CORRELATION=on
export OBSERVER_REAL_SEASONAL_LEARNERS=1200
export OBSERVER_REAL_SEASONAL_INTEGRATION=scratch_readers
```

Use `OBSERVER_REAL_SEASONAL_MODE=raw_unwrapped` for the raw arm. The internal
`SEASONAL_ATTRIBUTION_STRUCTURE_ONLY` flag is only for the resource ablation.
Recordings and credentials are not included in this PR. Reproducing the recorded
quality result requires the original scenarios and the same counter-to-rate
preprocessing; the resource fixture alone does not reproduce final scorer alerts.

## Prerequisites and limits

The pinned experiment includes a BOCPD likelihood-underflow fix in
`patches/metrics_detector_bocpd.patch`, applied equally to raw and seasonal arms.
Thus “raw” here is the **pinned research baseline**, not unmodified current main.
The other patches wire the testbench wrapper, bounded streaming storage and
optional scorer tracing. They are only applied to the disposable research checkout.
Tracing is disabled in resource measurements.

Before production work: measure real workload prevalence and alert impact, handle
metric types explicitly, retry/relearn without stale profiles, decide how history
survives restarts, and validate more workloads and whole-Agent cost. Shared scratch
buffers assume the existing serial detector execution model. Daily seasonality,
automatic discovery beyond the two candidate periods, and persistence are not
implemented.
