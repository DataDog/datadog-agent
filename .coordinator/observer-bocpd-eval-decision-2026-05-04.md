# Observer BOCPD Eval Decision

Recorded on 2026-05-04 for branch `ella/observer-ad-manual-eval-20260430`.

## Decision

Keep these BOCPD detector variants:

| Variant | Mean F1 | Median F1 | Decision | Rationale |
| --- | ---: | ---: | --- | --- |
| `bocpd_student_t` | 0.198666 | 0.0001 | Keep | Best retained BOCPD mean F1. The Student-t likelihood is an isolated robustness knob and is easy to evaluate or remove independently. |
| `bocpd_persistence` | 0.157623 | 0.0233 | Keep | Lower mean F1 than Student-t, but cleaner median behavior and a simple false-positive dampening knob. |

Drop these BOCPD detector variants:

| Variant | Mean F1 | Median F1 | Decision | Rationale |
| --- | ---: | ---: | --- | --- |
| `bocpd_combined` | 0.208009 | 0.0000 | Drop | Highest BOCPD mean, but no median lift and combines persistence, Student-t, and entropy gating, making eval attribution and rollback messy. |
| `bocpd_entropy` | 0.125895 | 0.0176 | Drop | Underperformed the two retained BOCPD variants and added extra detector state/config surface for weak gain. |

Baseline context from the same detailed eval run:

| Detector | Mean F1 | Median F1 |
| --- | ---: | ---: |
| `bocpd` | 0.116046 | 0.0173 |
| `scanwelch` | 0.121680 | 0.0614 |
| `scanmw` | 0.103541 | 0.0238 |

## Code Impact

- Removed `bocpd_entropy` and `bocpd_combined` from the observer component catalog.
- Removed the BOCPD entropy-gate config and posterior entropy bookkeeping.
- Kept `bocpd_persistence` and `bocpd_student_t` as separate detector entries so detailed eval can include or delete them independently.

## Eval Artifacts

Detailed eval artifacts were written under `/tmp/observer-detailed-eval-2a5b836/<component>/` with per-component `report.json` and `run.log` files.
