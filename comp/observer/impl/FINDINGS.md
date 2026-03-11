# Engine Review Findings

Findings from structured review of PR #47739 (shared engine for unified live/testbench execution).
Each item includes reproducibility rating, validation status, and fix SHA once addressed.

Reproducibility scale: **Easy** (unit test covers it directly), **Moderate** (needs a targeted test with specific setup), **Hard** (requires concurrency tooling or long-running scenario).

---

## HIGH

### H1: Storage methods missing RLock -- data race on `s.series` map

- **File:** `storage.go:346,374,405,450,271`
- **Description:** `Namespaces`, `TimeBounds`, `MaxTimestamp`, `ListAllSeriesCompact`, and `DroppedValueStats` iterate `s.series` without acquiring `s.mu`. Concurrent `Add()` calls create a data race on the map.
- **Reproducing:** Easy -- `go test -race` with a goroutine calling `Add` and another calling any of the five methods.
- [x] Validated -- 5 race tests in `findings_test.go` all fail under `-race`
- **Fix SHA:** 6d13f983a66

### H2: `MinVariance=0` re-enables constant-series false positives

- **File:** `metrics_detector_bocpd.go:261,394`
- **Description:** `ensureDefaults` has no guard against `MinVariance <= 0`. Setting it to zero defeats the MinVariance floor added in commit f3892b to fix constant-series false positives.
- **Reproducing:** Easy -- construct `BOCPDDetector{MinVariance: 0}`, feed 200 constant-value points, assert no anomalies.
- [x] Validated -- `ensureDefaults` passes through both 0 and negative values
- **Fix SHA:** 6d13f983a66

### H3: Changepoint mass uses prior predictive instead of sum over run-length predictives

- **File:** `metrics_detector_bocpd.go:300-305`
- **Description:** `newRunProbs[0] = hazard * predPrior` deviates from standard BOCPD. Should be `hazard * sum_r(runProbs[r] * pred(x|r))`. After a sustained level shift, the prior mean stays at warmup baseline while posterior means track the new level, causing incorrect cpProb: inflated for reversion to baseline, deflated for further shift away.
- **Reproducing:** Moderate -- feed a series with a level shift at T=150 sustained through T=300, then a second shift at T=301. Compare cpProb from this implementation against a reference BOCPD. Assert cpProb at T=301 matches the standard recurrence.
- [ ] Validated
- **Fix SHA:**

---

## MEDIUM

### M1: Dedup key too coarse -- drops distinct anomalies on same series+detector+timestamp

- **File:** `engine.go:306-312`
- **Description:** Dedup key is `{seriesID, detectorName, timestamp}`. Two anomalies with different severity, title, or description but same key collide. The second is silently dropped from `rawAnomalies`.
- **Reproducing:** Easy -- inject a detector that returns two anomalies with same seriesID+timestamp but different titles. Assert both appear in `rawAnomalies`.
- [x] Validated -- only 1 of 2 distinct anomalies survives dedup
- **Fix SHA:** 6d13f983a66

### M2: Log anomalies with empty `SourceSeriesID` collide on dedup key

- **File:** `engine.go:306-309`
- **Description:** Log anomalies leave `SourceSeriesID` empty. Two log anomalies from the same detector in the same second share dedup key `{"", detectorName, ts}`, and the second is dropped.
- **Reproducing:** Easy -- call `Advance` with a detector that emits two log-type anomalies at the same timestamp with empty SourceSeriesID. Assert both survive in `rawAnomalies`.
- [x] Validated -- only 1 of 2 log anomalies survives; empty SourceSeriesID causes collision
- **Fix SHA:** 6d13f983a66

### M3: Dedup asymmetry -- display store deduped but correlator/reporter pipeline is not

- **File:** `engine.go:239-242`
- **Description:** `captureRawAnomaly` deduplicates, but `processAnomaly` and the `allAnomalies` slice passed to reporters run unconditionally. Reporters receive duplicates that the display store filtered out.
- **Reproducing:** Easy -- subscribe a `collectingSink`, inject a detector that returns duplicates. Assert `anomalyCreated` event count matches `rawAnomalies` count.
- [x] Validated -- 2 events emitted but only 1 in rawAnomalies
- **Fix SHA:** 6d13f983a66

### M4: Unbounded growth of `uniqueAnomalySources` and `accumulatedCorrelations`

- **File:** `engine.go:57,64`
- **Description:** These maps grow without eviction. In a long-running live agent with dynamic metric cardinality, this is a slow memory leak.
- **Reproducing:** Moderate -- run engine in a loop with unique metric names per iteration. Assert map size stays bounded (or document expected growth).
- [ ] Validated
- **Fix SHA:**

### M5: `-math.MaxFloat64` not filtered in storage

- **File:** `storage.go:193-196`
- **Description:** Positive `math.MaxFloat64` is filtered, but negative is not. Two `-MaxFloat64` values in one bucket produce `-Inf` sum, corrupting aggregation results fed to detectors.
- **Reproducing:** Easy -- `storage.Add` two `-math.MaxFloat64` values at the same timestamp, query the bucket, assert no `-Inf`.
- [x] Validated -- sum of two `-MaxFloat64` produces `-Inf`
- **Fix SHA:** 6d13f983a66

### M6: BOCPD skips same-bucket value merges

- **File:** `metrics_detector_bocpd.go:150-172`
- **Description:** When multiple values arrive in the same second, storage merges them into one bucket. `PointCountUpTo` doesn't change, so BOCPD's cache check skips the series. The detector operates on partial data until the next new bucket arrives.
- **Reproducing:** Moderate -- add two values at the same timestamp, call `Detect` after each. Assert the detector sees the merged aggregate, not just the first value.
- [ ] Validated
- **Fix SHA:**

### M7: `WarmupPoints=1` causes NaN variance via division by zero

- **File:** `metrics_detector_bocpd.go:258,395`
- **Description:** `warmupM2 / (warmupCount - 1)` with `warmupCount=1` produces `0/0 = NaN`. `ensureDefaults` guards `<= 0` but not `< 2`. The detector silently degrades to garbage output.
- **Reproducing:** Easy -- construct `BOCPDDetector{WarmupPoints: 1}`, feed data, assert no NaN in internal state or that `ensureDefaults` rejects it.
- [x] Validated -- NaN cascades through baselineStddev, obsVar, priorPrecision, DeviationSigma
- **Fix SHA:** 6d13f983a66

### M8: `shortRunMass` includes cpProb making trigger conditions non-independent

- **File:** `metrics_detector_bocpd.go:430-440,341`
- **Description:** `shortRunLengthMass` sums `runProbs[0]` (the changepoint probability) through `runProbs[ShortRunLength]`. A high cpProb that misses `CPThreshold` can still push `shortRunMass` over `CPMassThreshold`, making the two triggers non-independent with potentially misleading alert descriptions.
- **Reproducing:** Moderate -- craft a series where cpProb is 0.55 (below CPThreshold=0.6) but short-run mass including r=0 exceeds CPMassThreshold=0.7. Assert which trigger fires and whether the description is accurate.
- [ ] Validated
- **Fix SHA:**

### M9: `SetDetectors`/`SetCorrelators` have no synchronization

- **File:** `engine.go:416-429`
- **Description:** These methods replace engine slices without any lock. Safe today (testbench serializes via `tb.mu`, live never calls post-init), but the public API is unprotected. Any future concurrent caller introduces a data race.
- **Reproducing:** Moderate -- `go test -race` with goroutines calling `SetDetectors` and `Advance` concurrently.
- [ ] Validated
- **Fix SHA:**

### M10: `Reset()` has no lock

- **File:** `engine.go:433-446`
- **Description:** Zeroes `lastAnalyzedDataTime`, `latestDataTime`, and calls Reset on detectors/correlators without synchronization. Same "safe today, fragile tomorrow" pattern as M9.
- **Reproducing:** Moderate -- `go test -race` with concurrent `Reset` and `Advance`.
- [ ] Validated
- **Fix SHA:**

### M11: StateView reads unprotected engine slices

- **File:** `stateview.go:67-76,124-142`
- **Description:** `ListDetectors`, `ListCorrelators`, `ActiveCorrelations` iterate engine fields without locks. Safe in current single-goroutine live path, but any new consumer on another goroutine hits a race.
- **Reproducing:** Moderate -- `go test -race` with goroutine reading stateView while another calls `Advance`.
- [ ] Validated
- **Fix SHA:**

### M12: Log-only timestamps skipped in replay advance sequence

- **File:** `engine.go:491-499`
- **Description:** `DataTimestamps()` only includes metric timestamps. Logs that produce no virtual metrics don't get their own advance step in replay, unlike live where every log triggers `onObservation`. The advance event sequence diverges between live and replay.
- **Reproducing:** Moderate -- ingest metrics at [100,101,102,105] and a log at 103 that produces no virtual metrics. Compare advance sequences between live-style ingestion and `ReplayStoredData`.
- [ ] Validated
- **Fix SHA:**

### M13: `latestDataTime` pre-contaminated before replay

- **File:** `testbench.go:561-570`
- **Description:** Log ingestion before `ReplayStoredData` sets `latestDataTime` to the max log timestamp. Harmless under `currentBehaviorPolicy` but breaks the "same timing semantics as live" invariant for any future stateful scheduler.
- **Reproducing:** Hard -- requires implementing or mocking a stateful scheduler that uses `latestDataTime` in `onObservation`, then comparing live vs replay behavior.
- [ ] Validated
- **Fix SHA:**
