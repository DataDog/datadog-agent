# Observer PR #50127 Run Summary

Recorded on 2026-05-05 from <https://github.com/DataDog/datadog-agent/pull/50127>.

## Current State

PR #50127 is a coordinator run-log for branch `claude/observer-full-20260429T1816`.
The comment stream spans 353 comments from 2026-04-29T18:21:47Z through
2026-05-05T13:08:03Z.

Latest terminal state:

- The resumed run reached iter 89 on 2026-05-05.
- Iter 88, `cross-source-onset-quorum-emitter`, was rejected by the pre-eval
  gate because it registered a no-op detector while the real correlator was
  unreachable.
- Iter 89, `kleinberg-2state-additive-burst-emitter`, was rejected by the
  pre-eval gate because the implementation changed the proposed correlator into
  a metric detector and therefore evaluated a different mechanism.
- The coordinator halted immediately after iter 89 because sync from
  `origin/q-branch-observer` conflicted in `.gitignore`.

## Outcomes

| Outcome | Count | Notes |
| --- | ---: | --- |
| `iter_shipped` | 1 | `bocpd-warmup-zfallback`, commit `4f2c275054` |
| `iter_archived` | 1 | `hellinger-histogram-changepoint`, commit `d5b96f5751`; research corpus only |
| `iter_rejected` | 96 | Most failed review gates after deterministic eval |
| `strict_regression` | 15 | Auto-rejected on catastrophe filters |
| `pre_eval_gate_rejected` | 28 | Mostly dead-code, miswired catalog, or plan-fidelity failures |
| `iter_eval_failed` | 1 | `burst-significance-correlator` timed out |
| `iter_impl_failed` | 5 | Implementation agent crashed |
| `iter_review_failed` | 4 | Review agent crashed |
| `eval_env_drift` | 1 | Sentinel sanity check failed early in the run |
| `eval_silent_failure` | 1 | All-zero eval output treated as unevaluable |
| `upstream_conflict` | 1 | Final halt on `.gitignore` conflict |

## Shipped Candidate

| Iter | Candidate | Commit | Mean F1 | FP Delta | Decision |
| ---: | --- | --- | ---: | ---: | --- |
| 0 | `bocpd-warmup-zfallback` | `4f2c275054` | 0.1160 -> 0.1802 (Delta +0.0642) | +40 | Shipped in PR #50127 run-log; later represented and intentionally reverted on the manual eval branch. |

Top scenario wins reported by the coordinator:

| Scenario | F1 Before | F1 After | Delta |
| --- | ---: | ---: | ---: |
| `food_delivery_redis` | 0.000 | 0.479 | +0.479 |
| `211_doordash` | 0.000 | 0.321 | +0.321 |
| `546_cloudflare` | 0.000 | 0.170 | +0.170 |
| `221_base` | 0.000 | 0.169 | +0.169 |
| `353_postmark` | 0.035 | 0.095 | +0.060 |

Source: <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4346536816>

## Archived Research Candidate

| Iter | Candidate | Commit | Mean F1 vs Original Baseline | FP Delta | Decision |
| ---: | --- | --- | ---: | ---: | --- |
| 95 | `hellinger-histogram-changepoint` | `d5b96f5751` | 0.1160 -> 0.2426 (Delta +0.1266) | +42 | Archived as research corpus; not shipped and did not update effective baseline. |

The archive note says the mechanism had research merit, but the candidate was
not accepted as a shipped branch component. Treat it as a source for possible
future isolated evaluation, not as part of the current survivor set.

Source: <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4361060303>

## Highest Scoring Rejected Candidates

These are worth remembering only as rejected leads. They should not be counted
as accepted branch content.

| Iter | Candidate | Mean F1 | Rejection reason |
| ---: | --- | ---: | --- |
| 87 | `median-of-means-robust-z` | 0.2165 -> 0.2702 (Delta +0.0537) | Rejected for plan/default-enabled/test guardrail violations despite score lift. |
| 27 | `forecast-residual-streaming` | 0.2165 -> 0.2387 (Delta +0.0222) | Rejected for single-scenario artifact, FP/recall regressions, and forbidden default-enabled wiring. |
| 11 | `holtres-forecast-residual` | 0.2165 -> 0.2322 (Delta +0.0157) | Rejected for tier-2 regression signals and forbidden default-enabled/config omissions. |
| 93 | `hodges-lehmann-shift-detector` | 0.2165 -> 0.2192 (Delta +0.0028) | Rejected under tier-2 override; small aggregate lift with scenario regressions and FP growth. |
| 83 | `bocpd-hbos-density-postfilter` | 0.2165 -> 0.2178 (Delta +0.0014) | Rejected under tier-2 override; suppresses too many true positives. |

Sources:

- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4359898117>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4351216648>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4348415909>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4360757214>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4359434880>

## Latest Resumed Run Notes

The 2026-05-04/2026-05-05 resumed run did not add accepted candidates. It mostly
tested postfilter/correlator ideas against an effective baseline around 0.3248,
and the scored candidates collapsed back near 0.116 or lower.

Important late failures:

| Iter | Candidate | Status | Reason |
| ---: | --- | --- | --- |
| 88 | `cross-source-onset-quorum-emitter` | `pre_eval_gate_rejected` | Registered a no-op detector; intended correlator was dead code. |
| 89 | `kleinberg-2state-additive-burst-emitter` | `pre_eval_gate_rejected` | Implemented a metric detector instead of the proposed additive correlator. |

Sources:

- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4379398775>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4379522460>
- <https://github.com/DataDog/datadog-agent/pull/50127#issuecomment-4379526807>

## Manual Branch Implication

No new PR #50127 candidate should be copied into
`ella/observer-ad-manual-eval-20260430` from the latest resumed run as-is.

Keep PR #50127 open for traceability until final cleanup:

- It contains the original shipped BOCPD warmup fallback history.
- It contains the archived Hellinger research commit.
- It documents a repeated failure mode in later correlator proposals:
  component-kind mismatch, no-op detector stubs, and unreachable correlator
  implementations.
