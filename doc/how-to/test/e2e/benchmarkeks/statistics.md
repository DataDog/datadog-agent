# How the test decides

The benchmark makes 11 comparisons today: three metrics for each of the three components, and the run times of two checks. For each one, it has the values of both versions over the two phases, and must answer three questions:

1. How much did the comparison version change, without the bias of the machines?
1. Is the change real, or is it noise?
1. Is the change large enough to matter?

The code is in <<<repo("test/new-e2e/tests/containers/benchmark/compare.go")>>> and <<<repo("test/new-e2e/tests/containers/benchmark/stats.go")>>>.

## Blocks

To tell a real change from noise, the test needs several roughly independent measurements of the difference between the versions. The points of the metrics don't qualify: there is one per minute, and the memory usage of a minute is nearly that of the minute before. So the test cuts each phase of 20 minutes into 4 blocks of 5 minutes, and computes the mean of each version over each block: `bₖ` for the baseline, `cₖ` for the comparison. The means of two blocks depend much less on one another than two consecutive points, and both means of a block are measured at the same time, with the same workload.

Longer blocks would depend even less on one another, but there would be fewer of them to estimate the noise from: blocks of 5 minutes leave 4 per phase, one more than the minimum of 3. A shorter phase gets shorter blocks, down to 1 minute, so that it still holds at least 3.

The run times of the checks are measured per block directly: see [Check run times](how-it-works.md#check-run-times).

## Cancelling out the machines

For each block, the test computes the log ratio of the comparison to the baseline, then averages these ratios over each phase, then over both phases:

```text
 block k:    rₖ = ln(cₖ / bₖ)

 phase 1:    m₁ = mean of the rₖ of phase 1   ≈  ln(effect) + ln(β)
 phase 2:    m₂ = mean of the rₖ of phase 2   ≈  ln(effect) − ln(β)

 both:       e  = (m₁ + m₂) / 2               ≈  ln(effect)

 change = exp(e) − 1
```

`effect` is what the test is after: how much more, or less, the comparison version uses than the baseline. `β` is how much more the machines of installation b use than those of installation a for the same work, for instance 1.1 when they use 10% more. In the first phase, the comparison runs on installation b, so the ratio measured in each block is the effect times `β`. Once swapped, the comparison runs on installation a, and the ratio is the effect divided by `β`. With logarithms, `β` turns into a term that the second phase subtracts as much as the first one adds. For instance, with a real effect of +3%, and machines of installation b using 10% more CPU for the same work:

```text
 phase 1, comparison on installation b:   1.03 × 1.10 = 1.133   →  +13.3%
 phase 2, comparison on installation a:   1.03 / 1.10 = 0.936   →   −6.4%
 both phases:   exp((ln 1.133 + ln 0.936) / 2) − 1 = +3.0%
```

Without the swap, this run would have reported a regression of 13.3%.

Ratios also make the comparison independent of the scale: a 3% regression is 3% whether the Agent uses 100 or 500 millicores. And the logarithm treats increases and decreases alike: doubling is `+ln 2`, halving `−ln 2`.

## Is the change real?

If both versions cost the same, `e` would only differ from 0 by noise. The test estimates this noise from how much the log ratios `rₖ` vary from one block to the next in each phase, and computes `t`, the ratio of `e` to its standard error `SE(e)`:

```text
 SE(e) = ½ √(s₁²/n₁ + s₂²/n₂)

        e              m₁ + m₂
 t = ───────  =  ────────────────────
      SE(e)       √(s₁²/n₁ + s₂²/n₂)
```

`s₁²` and `s₂²` are the variances of the `rₖ` of each phase, and `n₁` and `n₂` their numbers of blocks.

The farther `t` is from 0, the less likely the change is due to noise alone. As in Welch's t-test, `t` follows Student's t-distribution, with degrees of freedom given by the Welch–Satterthwaite equation: between 3 and 6 with 4 blocks per phase. The **p-value** is the probability, if both versions cost the same, to get a `t` at least this far from 0 by chance alone. A change is significant when its p-value is below the significance level.

### Many comparisons at once

At a significance level of 5%, each comparison of two identical versions has a 5% chance to come out significant by chance. Over 11 comparisons, the chance that at least one does is 43%: nearly half of the runs comparing a version with itself would report a change.

So the test divides the level by the number of comparisons, which is the Bonferroni correction: each comparison is significant below 0.05 / 11 ≈ 0.0045, and the chance of any false verdict in a run stays below 5%. With 4 blocks per phase, a change then needs a `t` beyond 4.4 to 7.7, depending on the degrees of freedom: it must stand well clear of the noise.

## Is the change large enough?

A significant change can still be too small to matter: with very stable data, a change of 1% can be significant. So a change must also be at least as large as two thresholds:

- a **relative** threshold, on the change;
- an **absolute** threshold, on the difference between the averages of both versions: a large relative change of a tiny value, such as 20% of 10 millicores, doesn't matter.

| Comparison | Relative | Absolute |
|---|--:|--:|
| `container.cpu.usage` | 5% | 5 millicores |
| `container.memory.working_set` | 5% | 10 MiB |
| `container.memory.rss` | 5% | 10 MiB |
| `kubelet` check run time | 20% | 5 ms |
| `kubernetes_state_core` check run time | 10% | 5 ms |

The `kubelet` check gets a larger threshold because its run time varies from one Agent pod to another: see [the limits](#limits). The thresholds are set in <<<repo("test/new-e2e/tests/containers/benchmarkeks_test.go", match="cpuThresholds +=")>>>, and each report states those it applied.

## The verdict

```text
 p-value ≥ 0.05 / 11 ? ─────────── yes ──► unchanged: it could be noise
     │ no
     ▼
 |Δ| < absolute threshold ? ────── yes ──► unchanged: too small to matter
     │ no
     ▼
 change ≥ +relative threshold ? ── yes ──► regression
     │ no
     ▼
 change ≤ −relative threshold ? ── yes ──► improvement
     │ no
     ▼
 unchanged: significant, but too small to matter
```

`Δ` is the difference between the **Comparison** and **Baseline** columns of [the report](index.md#read-the-result).

A comparison is an **error** when the test couldn't make it: a block without data for a version, a mean that isn't positive, an Agent status that couldn't be read. Missing data is never taken for a lack of change, as it could hide a regression. Errors and regressions fail the test.

## Validation

Before the benchmark was merged, it ran on pairs of releases whose differences were known:

| Baseline → comparison | Expected | Result |
|---|---|---|
| 7.84.0 → 7.84.0, A/A | No change | All unchanged |
| 7.77.3 → 7.78.0, fix of a leak of the KSM check ([#48680](https://github.com/DataDog/datadog-agent/pull/48680)) | Lower memory of the cluster checks runner | Improvement: memory −28%, CPU −11% |
| 7.78.0 → 7.77.3, the same reversed | A regression | Regression, the test failed: memory +38%, CPU +12% |
| 7.73.3 → 7.78.0, several optimizations of the Cluster Agent and of the KSM check | Lower CPU usage of the Cluster Agent and of the cluster checks runner | Improvements: Cluster Agent CPU −35%, cluster checks runner CPU −24%, KSM check run time −16%. But a regression failed the test: node Agent memory +6.6%, a growth accumulated over 5 minor versions |
| 7.74.1 → 7.75.0, faster parsing of the kubelet metrics ([#44216](https://github.com/DataDog/datadog-agent/pull/44216)) | A faster `kubelet` check | Unchanged: run time −10%, below its threshold of 20% |
| 7.75.4 → 7.76.0 ([#43389](https://github.com/DataDog/datadog-agent/pull/43389)) | Lower CPU usage of the Cluster Agent | Regression, the test failed: CPU +17.5%, a real regression the profiles confirmed |

These runs compared the memory through `container.memory.usage`, which the benchmark has since replaced with `container.memory.working_set`.

## Limits

- **Some noise escapes the p-value.** The p-value only accounts for the noise from one block to the next. Some differences last as long as an Agent process, and are drawn anew at each start: for instance, the run time of the `kubelet` check varies from one Agent pod to another. In the A/A run of [the example report](index.md#read-the-result), it differed by 13.4% with a p-value of 0.001: significant, but below its threshold of 20%. The thresholds guard against such differences, which is why they are calibrated with A/A runs.
- **Small changes pass.** A change below the thresholds is unchanged, however real: the benchmark catches large regressions, not slow drifts. Small regressions can add up over several versions, one at a time, as the memory of the node Agent did from 7.73.3 to 7.78.0.
- **Compare versions within a run, not results across runs.** The node images of EKS and the version of KWOK aren't pinned, so two runs don't run on exactly the same system. Each run compares its two versions on the same system.
