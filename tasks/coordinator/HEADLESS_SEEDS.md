# Headless Observer-AD Seeds

These are headless-specific restart seeds from the 2026-04-29 and 2026-04-30
run audit.

The SDK harness runs already own the `.coordinator/candidates/*.yaml` queue.
Do not duplicate those candidates here just to get another copy of the same
experiment. Use this file for complementary exploration, rubric diagnosis, and
ideas that need a different implementation strategy than the harness is likely
to try.

Bias toward seeds with a direct path to scored output changes. A good headless
seed should name the component boundary it changes, the false-positive failure
mode it targets, and the recall guard it will use. Prefer one concrete
implementation over a broad survey unless the survey is needed to avoid wasting
the run on the wrong boundary.

## Current flow painpoints

- The pipeline optimizes detector-local threshold behavior even though eval
  scores emitted correlation periods. A detector can reduce raw anomalies and
  still be a no-op if `time_cluster` emits the same periods.
- The current detector set is too narrow. BOCPD, ScanMW, and ScanWelch all
  spend most of their effort deciding local changepoints on one series at a
  time, then correlation tries to recover incident structure after the fact.
- Threshold changes are brittle. Global tightening repeatedly creates recall
  cliffs on a small number of scenarios while helping noisy scenarios.
- Cross-series evidence is underused. The most useful signal may be "several
  related weak anomalies co-fire" rather than "one series exceeds a strong
  threshold."
- Detector confidence is not a first-class pipeline signal. `Score` and
  `DeviationSigma` exist in places, but the emission/scoring boundary can lose
  that strength information.
- The current flow has weak abstention. It tends to emit or suppress, but does
  not explicitly model "insufficient evidence yet" with delayed confirmation.
- Component boundaries can make good ideas look bad. A filter at the wrong
  stage may not affect `CorrelationHistory()`, and a correlator accidentally
  registered as a detector is effectively testing the wrong thing.
- The harness is better at exploring small local edits than architectural
  replacements. The headless run should spend more time on pipeline shape,
  alternative component boundaries, and evidence aggregation.

## H-rubric-concentrated-lift-audit

```yaml
target_detector: bocpd
approach_family: rubric-concentrated-lift-audit
```

Reproduce one minimal recall-safe concentrated-lift change, then report it as a
rubric audit even if it fails the ship rule. The prior headless run saw BOCPD
threshold tightening produce real lift concentrated on a subset of scenarios,
but the variance-aware threshold rejected it. Keep the patch tiny, in-place,
and explicitly tag the result as `blocked-concentrated-lift` if applicable.

## H-cluster-score-observability

```yaml
target_detector: time_cluster
approach_family: cluster-score-observability
```

Explore whether detector-provided anomaly strength is visible at the emitted
cluster/scoring boundary. If not, make the smallest instrumentation or
confidence propagation change that lets a future candidate use `Score` or
`DeviationSigma` without imposing a hard fixed gate. This is about exposing a
useful signal and validating the pipeline boundary, not immediately optimizing a
threshold.

## H-residual-or-cascade-survey

```yaml
target_detector: residual_changepoint
approach_family: new-family-survey
```

Before implementing a full detector, survey the existing engine interfaces and
decide whether residual-first detection or two-stage cascade detection is the
better fit for the current component model. Implement only one small candidate
from that survey. The goal is a structurally different family, not another
global threshold tweak.

## H-incident-first-graph-correlator

```yaml
target_detector: graph_incident_correlator
approach_family: incident-first-cross-series-correlation
```

Think from the incident outward instead of from per-series detector outputs
inward. Build or prototype a correlator that treats anomalies as evidence nodes
and emits an incident only when enough related evidence accumulates across
series, namespaces, aggregations, or log-derived metrics. The implementation may
replace `time_cluster` for eval purposes if that is cleaner than bolting graph
logic onto it.

Primary question: can weak but coherent multi-series evidence beat strong
single-series thresholds? Prefer bounded online state: a sliding time window,
stable source keys, decayed edge weights, and a minimum evidence mass. Include
tests proving that scattered unrelated anomalies do not emit while related
co-firing anomalies do.

## H-ensemble-arbiter

```yaml
target_detector: ensemble_arbiter
approach_family: detector-replacement-ensemble
```

Stop assuming one detector family must win globally. Prototype a single
component that runs multiple cheap evidence functions internally and emits only
when independent views agree: residual deviation, rank/quantile surprise,
change magnitude, and short-term persistence. This can replace the existing
detector set for eval if it is cleaner than modifying BOCPD/ScanMW/ScanWelch.

The arbiter should avoid hard scenario-shaped rules. The point is not "run all
existing detectors and vote"; it is to combine simple orthogonal evidence in
one bounded component so threshold cliffs in one view do not dominate. Add tests
for disagreement/abstention, agreement/emission, and persistent low-amplitude
anomalies.

## H-abstain-then-confirm-flow

```yaml
target_detector: delayed_confirmation_pipeline
approach_family: abstention-and-confirmation
```

Explore an explicit "candidate anomaly" state between raw detector output and
final correlation emission. The current flow lacks a principled way to hold
weak evidence until a nearby point, sibling series, or later persistence
confirms it. Implement a small bounded confirmation layer if the component model
allows it; otherwise document exactly where the boundary blocks this and make
the smallest enabling change.

This should target false positives without the usual recall cliff: early weak
signals are not discarded, they are buffered briefly and promoted only when
confirmed. Keep latency bounded and include tests for expiry without emit,
promotion after repeated evidence, and promotion after cross-series support.

## H-learned-normalizer-before-detection

```yaml
target_detector: normalized_evidence_detector
approach_family: online-normalization-before-evidence
```

Investigate whether the main problem is that detectors see poorly normalized
series. Prototype a detector that first transforms each series into a stable
online residual or percentile score, then runs a very simple evidence rule on
that normalized stream. This can replace existing detectors; do not preserve
BOCPD/ScanMW/ScanWelch unless they are genuinely useful after normalization.

The key constraint is streaming bounded state: no offline training, no scenario
inspection, no full-series smoothing. Prefer robust EWMA/MAD, online quantiles,
or rolling rank transforms. The success condition is fewer per-scenario
threshold cliffs because all series are compared in a normalized evidence
space.

## H-output-aware-emission-contract

```yaml
target_detector: pipeline_contract
approach_family: output-aware-component-contract
```

Audit the contract between detector anomalies, correlator state, output JSON,
and `CorrelationHistory()`. Then implement the smallest change that makes the
pipeline optimize what eval actually scores. This may be a new emitted
correlation confidence, an output ranking field, or a stricter rule that only
correlation-level emissions count as final anomalies.

This seed is valid even if the result is mostly diagnostic. The deliverable is
a concrete component-boundary finding with a patch or test proving that a
candidate changes scored anomaly periods, not just raw anomaly counts.

## H-score-mass-cluster-emitter

```yaml
target_detector: time_cluster
approach_family: score-mass-correlation-emission
```

Replace count-only cluster strength with evidence mass. A cluster with three
weak anomalies should not automatically outrank one with two very strong
anomalies, and a cluster with many tiny duplicate aggregation variants should
not emit just because count is high. Compute bounded cluster evidence from
unique source count, detector `Score`, `DeviationSigma`, source diversity, and
time compactness.

Expected improvement: reduce false-positive anomaly periods where noisy
low-severity sources happen to co-fire, while preserving recall for smaller but
high-confidence incident clusters. Recall guard: never drop a cluster solely
because it has few members if at least one member has very high score or
deviation.

## H-source-family-dedup-correlator

```yaml
target_detector: source_family_correlator
approach_family: aggregation-and-source-deduplication
```

Many false positives may be amplified by multiple aggregations or near-duplicate
series representing the same underlying source. Add a correlation-stage
deduper that groups anomalies by source family before computing cluster support:
metric name without aggregation suffix, stable tag subset, namespace, and
log-derived source identity.

Expected improvement: reduce FP periods caused by one noisy source appearing as
many independent votes. Recall guard: dedup only for evidence weighting, not
for removing the underlying anomalies from emitted context. Tests should show
that duplicate aggregation variants count as one vote while unrelated services
still increase evidence.

## H-topk-budgeted-emission

```yaml
target_detector: budgeted_emitter
approach_family: confidence-ranked-output-budget
```

Add a bounded per-window emission budget at the final correlation/emission
stage. When a window has many possible anomaly periods, emit the highest
evidence clusters first and suppress low-confidence tails. This directly targets
scored false positives because it changes `CorrelationHistory()` rather than
raw detector output.

Expected improvement: large FP reductions in noisy scenarios with limited
recall loss if the ranker uses evidence mass and source diversity. Recall guard:
the budget must be soft: high-confidence clusters always pass, and the cap only
applies to weak clusters within a sliding time window.

## H-robust-ewma-mad-detector

```yaml
target_detector: robust_ewma_mad
approach_family: robust-online-baseline-detector
```

Implement a simple replacement detector using robust online baseline tracking:
EWMA median-like level, EWMA absolute deviation, and a persistence rule over
normalized residuals. This is deliberately less clever than BOCPD/ScanMW and
may win because it has fewer brittle state transitions.

Expected improvement: reduce threshold cliffs from changepoint-specific state
machines while preserving obvious spikes and shifts. Recall guard: emit on both
sudden residual jumps and sustained residual elevation. Tests should cover slow
drift ignored, abrupt jump detected, and transient one-point spike suppressed
unless extremely large.

## H-online-quantile-shift-detector

```yaml
target_detector: quantile_shift
approach_family: streaming-quantile-surprise
```

Use online quantile/rank surprise instead of Gaussian assumptions or windowed
t-tests. Maintain a bounded sketch or compact reservoir per series and emit when
new values remain in an extreme tail for a short confirmation period. This is a
good fit for skewed metrics where mean/stddev detectors overreact.

Expected improvement: fewer FPs on heavy-tailed or skewed series while keeping
recall on sustained distribution shifts. Recall guard: allow immediate emission
for extreme rank surprise plus delayed confirmation for moderate surprise.

## H-lag-difference-seasonal-detector

```yaml
target_detector: lag_difference
approach_family: seasonal-and-trend-invariant-detection
```

Prototype a detector that detects changes in short-lag differences rather than
raw values. Many observer metrics may have level, trend, or periodic baseline
movement; detecting on `x[t] - x[t-k]` or a small set of lag residuals can make
the anomaly boundary less dependent on absolute scale.

Expected improvement: reduce false positives from baseline movement while
preserving spikes, drops, and abrupt slope changes. Recall guard: fall back to
raw residual evidence when history is too short or lag residuals are unstable.

## H-cofire-before-singleton

```yaml
target_detector: cofire_first_detector
approach_family: cross-series-first-detection
```

Invert the normal order: collect weak per-series evidence continuously, but only
promote it to anomalies when several related series co-fire in a short window.
This can be implemented as a detector/correlator hybrid if necessary; the point
is to use cross-series support before final anomaly creation, not after a large
set of singleton anomalies has already been emitted.

Expected improvement: suppress isolated noisy singleton detections that become
FP periods, while preserving incidents that manifest as coordinated weak
changes. Recall guard: keep an escape hatch for one extremely strong singleton
with high normalized evidence.

## H-multi-resolution-windows

```yaml
target_detector: multi_resolution_detector
approach_family: short-and-long-window-evidence
```

Several failures may come from using one effective time scale. Add a detector
that computes evidence at two or three bounded time resolutions: short spikes,
medium shifts, and longer sustained changes. Emit only when the active
resolution agrees with persistence expectations for that anomaly shape.

Expected improvement: reduce FPs where a short window overreacts to noise, and
recover recall where a long window misses abrupt onsets. Recall guard: each
resolution must have an independent minimum evidence rule, not a global
threshold bump.

## H-negative-evidence-suppressor

```yaml
target_detector: negative_evidence_suppressor
approach_family: explicit-non-incident-evidence
```

Add an emission-stage suppressor that looks for evidence a candidate is probably
not an incident: no source diversity, low score mass, repeated emissions from
the same source family, no persistence, or cluster spread too wide in time. This
is different from a raw cooldown because it reasons over final cluster
features.

Expected improvement: targeted FP reduction with less recall damage than global
thresholds. Recall guard: suppression must require multiple negative features,
and any strong positive evidence feature should veto suppression.

## H-component-replacement-smokeout

```yaml
target_detector: replacement_smokeout
approach_family: full-pipeline-ablation
```

Run a small number of intentional ablations to identify which legacy components
are helping versus hurting: detector-only output, `time_cluster` replaced by a
minimal pass-through, one new detector without legacy detectors, and one new
correlator with legacy detectors. Then implement the smallest replacement that
the ablation suggests.

Expected improvement: avoid spending the run tuning components that the scoring
path barely uses or that mostly add FPs. Deliverable can be a patch plus a
short ablation matrix; this is valuable even if the first replacement is not
shippable.

## Avoid for this restart

- raw-anomaly cooldown in `captureRawAnomaly`
- global ScanMW or ScanWelch threshold bumps
- Mann-Kendall trend detector
- Student-t BOCPD likelihood swap
- fixed `time_cluster` severity gate
- direct reruns of the SDK harness YAML queue unless the implementation or
  rubric question is genuinely different
- any correlator implementation that cannot prove it is registered as
  `componentCorrelator`
- preserving the current detector/correlator set by default when a replacement
  component gives a cleaner test of the idea
- pure diagnostics that do not end in either a patch, an ablation matrix, or a
  concrete "do not reseed this family" finding
