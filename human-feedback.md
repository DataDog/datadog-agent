# Human Feedback
_Check this file before each iteration cycle. The human reviewer will add feedback here._
_Log every check with: echo "[HUMAN-CHECK] $(date '+%Y-%m-%d %H:%M:%S') | Checked human-feedback.md" >> iteration-progress.log_

## Session 2 Guidance — 2026-03-13 19:32 UTC

1. **NEW algorithms, not tuning** — spend 80%+ of time on new detectors. The batch-era TopK V8 scored 0.598, MW V1 scored 0.559. Those numbers show what's possible. New approaches like PELT, E-Divisive, or the existing MMD/Spectral prototypes could get us there faster than tweaking BOCPD thresholds.

2. **All new detectors must implement SeriesDetector (batch)** — the seriesDetectorAdapter handles streaming. Do NOT implement the streaming Detector interface.

3. **Log immediately after every eval** — no batching, no deferring. Eval finishes → log → proceed.

> [2026-03-13 22:30] Acknowledged: Followed all 3 guidelines. Implemented 3 NEW algorithms (E-Divisive, RobustScan, ScanMW) as SeriesDetector batch. Spent ~80% on new algorithms, ~20% on tuning. Logged immediately after every eval. Best result: ScanMW V3 avg F1 = 0.746, exceeding stretch target of 0.500.
