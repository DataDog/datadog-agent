# extmetrics-backoff-cap-stays-serving — External-metrics backoff reaches its cap and DCA keeps serving

**Type:** Reachability · **Assertion:** `Reachable` · **Priority:** P2 · **Intent:** invariant

**Provenance:** merged from 1 discovery agent(s): external-metrics-backoff-cap-stays-serving

## Property

Under a prolonged Datadog-backend outage, the external-metrics retriever's per-query exponential backoff reaches its 1800s cap and the DCA keeps serving (stale-marked) metric values rather than crashing or dropping out of the Service.


## Invariant / assertion

`assert.Reachable(backoff_reached_cap)` plus a `Sometimes(metric_marked_stale_but_served)`. Reachable fits the primary goal — confirming the max-backoff branch is actually hit under a long outage (a state deterministic tests rarely reach). The 'stays serving' aspect verifies the claimed 'serving stale data is better than no data' guarantee.


## Antithesis angle

metrics_retriever uses NewExpBackoffPolicy(2,30,1800,...) (metrics_retriever.go:29); a metric in error backoff is skipped until RetryAfter, up to 1800s. Sustained partition DCA<->Datadog backend, long duration; assert the cap is reached and a recovered metric is eventually re-queried, and that the DCA stays Ready and marks metrics stale (command.go:389-392).


## Why it matters

Confirms the documented degraded-mode guarantee (stay Ready, mark stale) actually holds, and that backoff neither wedges permanently nor drops the DCA from HPA service. Otherwise HPAs silently stop receiving metrics.


## Mechanism refinement (from open-question investigation)

Two corrections. (1) SCOPE: the 1800s cap is UNREACHABLE under default config — external_metrics_provider.split_batches_with_backoff defaults to false (common_settings.go:569) and Retries is incremented only in that mode (metrics_retriever.go:165). The Reachable(backoff==1800s) goal requires the harness to set split_batches_with_backoff=true; also rate-limit (429) errors never increment Retries, so a pure-429 outage never advances the backoff. (2) ASSERTION: the 'Sometimes(metric_marked_stale_but_served)' sub-assertion is contradicted on the CRD/HPA path this retriever feeds — ToExternalMetricFormat returns an error for Valid=false (datadogmetricinternal.go:267-276; provider.go:176), so the HPA gets an error, not a stale value. On this path only 'DCA process stays alive/Ready and backoff stays bounded' holds; 'serving stale values' applies to the WPA/ConfigMap bundle path instead.


## Fault dependencies

- sustained network partition DCA<->Datadog metrics backend, long duration (enabled by default)
- requires external_metrics_provider enabled + leader


## SUT-side instrumentation (all MISSING — zero existing SDK usage)

MISSING. `assert.Reachable` at the backoff-cap branch; `assert.Sometimes` on stale-but-served. Long timeline needed for 1800s cap — flag Antithesis timeline-length constraint.


## Open questions (post-investigation)

- Does reaching the 1800s cap (~15-30+ min of sustained outage) fit a single Antithesis timeline? The backoff constants (2,30,1800,2) are a hardcoded package var at metrics_retriever.go:29 and are NOT config-driven — only external_metrics_provider.refresh_period (30s) is configurable, so shortening the cap for the test requires a code change, not config. `(needs human input)`


### Investigation Log

#### Q1: default refreshPeriod; do rate-limit vs generic errors increment Retries differently?

Examined common_settings.go:552 (refresh_period default 30s), :569 (split_batches_with_backoff default FALSE), and metrics_retriever.go:87-88,147-148,165,183-188. Found: incrementRetries runs ONLY when splitBatchBackoffOnErrors==true AND the error is NOT RateLimitExceededError; rate-limit errors are batched with valid queries and never increment Retries; on a valid result in split mode Retries is reset to 0 (:148). CRITICAL: with the default config (split_batches_with_backoff=false) incrementRetries is never called, so Retries stays 0 and the backoff never grows. Conclusion: RESOLVED -> reaching the cap requires split_batches_with_backoff=true (non-default). See property_change.

#### Q2: is a Valid=false metric still returned by the provider or filtered before the HPA?

Examined datadogmetricinternal.go ToExternalMetricFormat:267-276 (returns d.Error / a 'stale' error when !Valid) and provider.go getExternalMetric:176-179 (propagates that error). Found: on the CRD/HPA provider path a Valid=false (stale/error) metric is NOT served as a value — the HPA receives an error. The 'serve stale data is better than none' comment (command.go:389-392) is about the WPA-controller/custommetrics path serving last-stored values flagged stale, a different path. Conclusion: RESOLVED -> the 'stale-marked but served' sub-assertion does not hold on the CRD path. See property_change.

#### Q3: does the 1800s cap fit a single timeline or must constants be shortened?

Examined backoff schedule + RetryAfter gating (metrics_retriever.go:101,186-188): each errored metric is re-queried only after its backoff elapses, so cumulative wait to Retries>5 ~= 30+60+120+240+480 ~= 15.5 min then 1800s spacing. Found: backoff params are a hardcoded package var (metrics_retriever.go:29), not config-driven. Conclusion: whether ~15-30+ min fits a run is a harness/duration decision -> needs-human; noted that constants cannot be shortened via config.


---

## Source discovery evidence (raw, per contributing agent)


### from `external-metrics-backoff-cap-stays-serving`

## Property
When the Datadog metrics backend is unreachable for a long time, the retriever's per-metric backoff climbs and saturates at the 1800s cap, and throughout the DCA keeps running and keeps returning the last (stale, Valid=false) values to the HPA path rather than crashing or dropping out of the external-metrics APIService.

## Where (code paths)
- Backoff policy: `pkg/clusteragent/autoscaling/externalmetrics/metrics_retriever.go:29` `backoffPolicy = backoff.NewExpBackoffPolicy(2, 30, 1800, 2, false)` with the comment (:26-28): for R<6 random(2^(R-1)*30, 2^R*30), otherwise 1800s.
- Backoff applied: `metrics_retriever.go:186-187` `backoffDuration := backoffPolicy.GetBackoffDuration(metricsInternal.Retries); metricsInternal.RetryAfter = timeNow.Add(backoffDuration)`.
- Retry increment on non-rate-limit errors: `metrics_retriever.go:164` region.
- Refresh loop: `pkg/clusteragent/autoscaling/externalmetrics/autoscaler_watcher.go:155` ticker on `refreshPeriod`.
- Leader-gated: retriever only refreshes on the leader; stale values are marked and still served (SUT analysis section 7.2 and command.go:389-392 'stays Ready and marks external metrics stale').
- Existing test confirms the cap semantics: `metrics_retriever_backoff_test.go:1087-1089` 'when retries > 5, backoff capped at 1800sec'.

## Failure scenario (degradation to verify, not a crash)
1. DCA leader, external metrics enabled, at least one DatadogMetric with active HPA.
2. Antithesis partitions the DCA from the Datadog backend for tens of minutes.
3. Each refresh fails; Retries increments; backoff spacing grows 30->60->...->1800 and caps.
4. Assert Reachable: a metric reaches Retries>5 and backoffDuration==1800 while the process is alive.
5. Assert Sometimes: the HPA provider path returns the metric with Valid=false (stale) rather than an error that drops it from the APIService.

## Assertion (net-new)
In `metrics_retriever.go` after computing `backoffDuration`: `assert.Reachable(backoffDuration == 1800*time.Second, "external metrics backoff reached cap")`. In the provider/store read path: `assert.Sometimes(!metric.Valid && served, "stale external metric still served")`.

## Key observations
- This is a capacity/backpressure ceiling property: the value being tested is that retries are bounded (no hot loop against the backend) AND service continuity is preserved.
- The fragile string-based error typing (SUT analysis 7.2) means a backend error-format change could collapse all errors to 'unknown' and alter retry accounting - a secondary target, lower confidence.

## Timing window
Reaching the cap takes >~30 minutes of continuous failure given the exponential schedule; the run must be long or the injected outage sustained.
