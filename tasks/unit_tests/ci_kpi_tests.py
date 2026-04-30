import unittest
from datetime import datetime, timedelta, timezone

from tasks.libs.pipeline.ci_kpi import (
    PipelineEvent,
    PRSnapshot,
    compute_metrics,
    compute_pr_metric,
    find_trustworthy_green,
    is_eligible,
)

UTC = timezone.utc


def _ts(minutes_from_epoch: int) -> datetime:
    return datetime(2026, 4, 1, 0, 0, tzinfo=UTC) + timedelta(minutes=minutes_from_epoch)


def _pipeline(
    pid: int,
    sha: str,
    *,
    source: str = "push",
    status: str = "success",
    started: int = 0,
    finished: int | None = 30,
    manual_retry: bool = False,
) -> PipelineEvent:
    return PipelineEvent(
        pipeline_id=pid,
        sha=sha,
        source=source,
        status=status,
        created_at=_ts(started),
        finished_at=_ts(finished) if finished is not None else None,
        has_manually_retried_job=manual_retry,
    )


def _snapshot(
    *,
    pipelines: list[PipelineEvent],
    n_pushes: int = 1,
    branch_kind: str = "dev",
    skip_ci: bool = False,
    is_draft_only: bool = False,
    clock_offset: int = 0,
) -> PRSnapshot:
    return PRSnapshot(
        pr_id=42,
        branch="feature-branch",
        branch_kind=branch_kind,
        clock_start=_ts(clock_offset),
        pipelines=tuple(pipelines),
        n_pushes=n_pushes,
        skip_ci=skip_ci,
        is_draft_only=is_draft_only,
    )


class TestTrustworthyGreen(unittest.TestCase):
    def test_happy_path_first_push_goes_green_no_retries(self):
        snap = _snapshot(pipelines=[_pipeline(1, "sha1", started=2, finished=22)])

        green = find_trustworthy_green(snap)
        metric = compute_pr_metric(snap)

        self.assertIsNotNone(green)
        self.assertEqual(green.pipeline_id, 1)
        self.assertTrue(metric.reached_trustworthy_green)
        # Clock starts at 0, candidate finishes at minute 22 → 22 * 60s.
        self.assertEqual(metric.duration_s, 22 * 60)
        self.assertFalse(metric.had_manual_retry)

    def test_manual_rerun_taints_first_success_so_clock_extends_to_next_clean_push(self):
        # Push 1 needed a manual rerun; push 2 went green naturally.
        first_push_failed = _pipeline(1, "sha1", status="failed", started=0, finished=10)
        first_push_rerun_green = _pipeline(2, "sha1", source="web", status="success", started=11, finished=21)
        second_push_green = _pipeline(3, "sha2", started=30, finished=50)

        snap = _snapshot(
            pipelines=[first_push_failed, first_push_rerun_green, second_push_green],
            n_pushes=2,
        )
        metric = compute_pr_metric(snap)

        self.assertTrue(metric.reached_trustworthy_green)
        # The second push's natural-source success is the trustworthy green.
        self.assertEqual(metric.duration_s, 50 * 60)
        self.assertTrue(metric.had_manual_retry)

    def test_manually_retried_job_within_green_pipeline_disqualifies_it(self):
        bad_green = _pipeline(1, "sha1", started=0, finished=20, manual_retry=True)
        clean_green_later = _pipeline(2, "sha2", started=30, finished=40)

        snap = _snapshot(pipelines=[bad_green, clean_green_later], n_pushes=2)
        metric = compute_pr_metric(snap)

        self.assertTrue(metric.reached_trustworthy_green)
        self.assertEqual(metric.duration_s, 40 * 60)
        self.assertTrue(metric.had_manual_retry)

    def test_force_push_invalidates_old_pipelines_but_new_sha_can_still_go_green(self):
        # Old SHA had a manual rerun then was force-pushed away; new SHA is clean.
        old_sha_failed = _pipeline(1, "sha-old", status="failed", started=0, finished=8)
        old_sha_manual = _pipeline(2, "sha-old", source="web", status="success", started=9, finished=15)
        new_sha_green = _pipeline(3, "sha-new", started=20, finished=35)

        snap = _snapshot(pipelines=[old_sha_failed, old_sha_manual, new_sha_green], n_pushes=2)
        metric = compute_pr_metric(snap)

        # The trustworthy green is the natural-source success on the new SHA;
        # the manual on the old SHA only taints the old SHA's lineage.
        self.assertTrue(metric.reached_trustworthy_green)
        self.assertEqual(metric.duration_s, 35 * 60)
        self.assertTrue(metric.had_manual_retry)

    def test_pr_with_only_failures_never_reaches_trustworthy_green(self):
        snap = _snapshot(
            pipelines=[
                _pipeline(1, "sha1", status="failed", started=0, finished=10),
                _pipeline(2, "sha2", status="failed", started=20, finished=30),
            ],
            n_pushes=2,
        )
        metric = compute_pr_metric(snap)

        self.assertFalse(metric.reached_trustworthy_green)
        self.assertIsNone(metric.duration_s)

    def test_pipeline_still_running_is_ignored(self):
        running = _pipeline(1, "sha1", status="running", started=0, finished=None)
        green = _pipeline(2, "sha1", started=5, finished=25)

        snap = _snapshot(pipelines=[running, green])
        metric = compute_pr_metric(snap)

        self.assertTrue(metric.reached_trustworthy_green)
        self.assertEqual(metric.duration_s, 25 * 60)

    def test_clock_start_after_pipeline_yields_clamped_zero(self):
        # Defensive: if PR was opened after the green pipeline finished
        # (rare race in data: PR converted from a long-lived branch), the
        # duration should clamp to zero rather than go negative.
        green = _pipeline(1, "sha1", started=0, finished=10)
        snap = _snapshot(pipelines=[green], clock_offset=20)

        metric = compute_pr_metric(snap)

        self.assertTrue(metric.reached_trustworthy_green)
        self.assertEqual(metric.duration_s, 0.0)


class TestEligibility(unittest.TestCase):
    def test_skip_ci_label_excludes_pr(self):
        snap = _snapshot(pipelines=[_pipeline(1, "sha1")], skip_ci=True)
        self.assertFalse(is_eligible(snap))

    def test_draft_only_pr_excluded(self):
        snap = _snapshot(pipelines=[_pipeline(1, "sha1")], is_draft_only=True)
        self.assertFalse(is_eligible(snap))

    def test_compute_metrics_filters_ineligible(self):
        ok = _snapshot(pipelines=[_pipeline(1, "sha1", finished=10)])
        skipped = PRSnapshot(
            pr_id=99,
            branch="b",
            branch_kind="dev",
            clock_start=_ts(0),
            pipelines=(_pipeline(2, "sha2"),),
            skip_ci=True,
        )

        results = compute_metrics([ok, skipped])

        self.assertEqual(len(results), 1)
        self.assertEqual(results[0].pr_id, 42)


class TestMergeQueueTagging(unittest.TestCase):
    def test_branch_kind_propagates_to_metric(self):
        snap = _snapshot(
            pipelines=[_pipeline(1, "sha1", source="merge_request_event", finished=10)],
            branch_kind="merge_queue",
        )
        metric = compute_pr_metric(snap)

        self.assertEqual(metric.branch_kind, "merge_queue")
        self.assertTrue(metric.reached_trustworthy_green)


if __name__ == "__main__":
    unittest.main()
