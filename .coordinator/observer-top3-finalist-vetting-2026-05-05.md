# Observer Top-3 Finalist Vetting - 2026-05-05

## Scope

Baseline for all deltas is corrected to `bocpd + time_cluster` with `rrcf` disabled. The top-three candidates were:

- `tukey_biweight`
- `holt_residual`
- `acorrshift`

Important correction:

- Combo rows are invalid for promotion if production must never run two metric detectors on the same data point. The eval harness can enable multiple detectors at once, and the engine runs each enabled detector over the same storage snapshot. Therefore pair/all-three rows are retained only as diagnostic evidence about overlap/noise, not as candidate stacks.

Artifacts:

- Singles/all-proposals matrix: `.coordinator/observer-incremental-eval-all-proposals.json`
- Top-3 pair matrix: `.coordinator/observer-top3-combo-eval.json`
- Top-3 pair matrix plus explicit all-three run: `.coordinator/observer-top3-combo-eval-with-all-three.json`
- Corrected replacement/tuning matrix: `.coordinator/observer-replacement-top2-tuning-2026-05-05.json`

## Replacement Matrix

This is the decision matrix under the one-detector-per-point constraint: each candidate replaces BOCPD rather than running alongside it. `time_cluster` remains enabled for every row.

| Rank | Candidate | Score | Delta | Median F1 | Worst delta | Pred | Base FP | FP delta |
|---:|---|---:|---:|---:|---:|---:|---:|---:|
| 1 | `tukey_biweight` default | 0.2932 | +0.1772 | 0.0617 | -0.6050 | 399 | 36 | -17 |
| 2 | `tukey_biweight` z=5.5 | 0.2429 | +0.1268 | 0.0617 | -0.6050 | 400 | 36 | -17 |
| 3 | `holt_residual` default | 0.2314 | +0.1154 | 0.0766 | -0.6204 | 289 | 61 | +8 |
| 4 | `holt_residual` z=5.0/min_dev=3.5 | 0.1847 | +0.0687 | 0.0730 | -0.4822 | 270 | 51 | -2 |
| 5 | `bocpd` baseline | 0.1160 | 0.0000 | 0.0173 | 0.0000 | 302 | 53 | 0 |

## Invalid Combo Matrix

| Rank | Candidate stack | Score | Delta | Median F1 | Worst delta | Pred | Base FP | FP delta |
|---:|---|---:|---:|---:|---:|---:|---:|---:|
| 1 | `tukey_biweight + holt_residual` | 0.2595 | +0.1435 | 0.1871 | -0.6422 | 329 | 99 | +46 |
| 2 | `tukey_biweight` | 0.2422 | +0.1262 | 0.1023 | -0.6173 | 421 | 68 | +15 |
| 3 | `tukey_biweight + acorrshift` | 0.2394 | +0.1233 | 0.1557 | -0.6549 | 473 | 100 | +47 |
| 4 | `all_three` | 0.2183 | +0.1023 | 0.1569 | -0.6549 | 352 | 119 | +66 |
| 5 | `holt_residual` | 0.2105 | +0.0945 | 0.1183 | -0.6461 | 357 | 98 | +45 |
| 6 | `holt_residual + acorrshift` | 0.1858 | +0.0698 | 0.0721 | -0.6549 | 401 | 122 | +69 |
| 7 | `acorrshift` | 0.1644 | +0.0484 | 0.0942 | -0.5404 | 537 | 97 | +44 |
| 8 | `default` | 0.1160 | 0.0000 | 0.0173 | 0.0000 | 302 | 53 | 0 |

## Per-Scenario Findings

`tukey_biweight`

- Best replacement candidate: +0.1772 score over BOCPD.
- Reduces baseline FPs by 17 relative to BOCPD.
- Small z=5.5 tuning lowered score to 0.2429 without improving baseline FPs, so keep default z=5.0.

`holt_residual`

- Strong replacement candidate: +0.1154 score over BOCPD.
- Default adds only +8 baseline FPs in true replacement mode, much less than the invalid BOCPD+Holt add-on row suggested.
- Conservative z=5.0/min_dev=3.5 tuning reduces baseline FPs below BOCPD (-2) and improves worst regression, but score drops from 0.2314 to 0.1847. Keep default for score; keep tuned variant only if FP budget dominates.

`acorrshift`

- Best wins: `063_twilio` +0.815, `food_delivery_redis` +0.276, `221_base` +0.185.
- Worst regressions: `703_shopify` -0.540, `ehr_pgbouncer` -0.164, `casino_postgresql` -0.062.
- Adds +44 baseline FPs and 537 predictions. This is too noisy for the marginal score lift.

`tukey_biweight + holt_residual`

- Best combo by score and median F1.
- Marginal lift over Tukey alone is only +0.0173 score while adding +31 more baseline FPs.
- Invalid as a merge/harness candidate under the one-detector-per-point constraint.

`all_three`

- Worse than `tukey_biweight + holt_residual`.
- Adds +66 baseline FPs.
- Invalid as a merge/harness candidate under the one-detector-per-point constraint.

## Source-Level Vetting

Catalog/default state:

- `acorrshift`, `holt_residual`, and `tukey_biweight` are registered and default-disabled in `comp/observer/impl/component_catalog.go`.
- Existing catalog tests cover the detector teardown contract in `comp/observer/impl/component_catalog_test.go`.
- Added a catalog test that explicitly asserts the finalist detectors stay default-disabled.
- Added typed testbench config parsing for `tukey_biweight` and `holt_residual`.

Score consistency:

- `tukey_biweight` emits `Score`.
- `holt_residual` emits `Score`.
- `acorrshift` does not emit `Score`; this is a merge blocker unless intentionally documented.

Test coverage:

- `tukey_biweight` has stable Gaussian, level shift, outlier robustness, linear trend, `RemoveSeries`, amortization, replay-skip, and incremental-vs-batch coverage.
- `holt_residual` has constant/ramp no-fire, spike/step behavior, refractory behavior, reset/default/interface, `RemoveSeries`, and incremental-vs-batch coverage.
- `acorrshift` has white-noise no-fire, stationary AR(1) no-fire, autocorrelation shift positive cases, `RemoveSeries`, and helper tests.

Doc/comment drift:

- Cleaned the stale `metrics_detector_tukey_biweight_test.go` comment claiming a prior default-enabled state and saying default-false is out of scope.
- `metrics_detector_tukey_biweight.go` documents a plan deviation from the original biweight gate to a `glitchZCap` rule. This is acceptable for finalist vetting, but must be captured in the final proposal/README if kept.

Performance:

- On heavy `213_pagerduty`, all-three detector costs were roughly 49-63ms avg per detector invocation for `acorrshift`, `tukey_biweight`, `holt_residual`, and `bocpd`. This scenario is expensive for all stateful detectors.
- `acorrshift` has the weakest value/cost profile because its score lift is lowest while prediction and FP deltas are high.

## Decision

Keep:

- `tukey_biweight` as the primary finalist.
- `holt_residual` as a secondary finalist. Use default parameters for score, or the z=5.0/min_dev=3.5 variant if the priority is reducing baseline FPs.

Drop/archive:

- `acorrshift` from the main finalist set. It can be archived as an orthogonal-signal idea, but current eval does not justify the FP/prediction cost.
- All combo/all-three stacks as invalid under the one-detector-per-point constraint.

Merge blockers before accepting any detector:

- Document `glitchZCap` as intentional in the final proposal/README.
- Either add `Score` to `acorrshift` or keep it archived.
