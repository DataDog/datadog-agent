"""
Pure-logic helpers for the "Time-to-Trustworthy-Green per PR" KPI.

This module deliberately has no I/O so the computation can be unit-tested
with synthetic fixtures. The invoke task in `tasks/ci_kpi.py` is the
thin wrapper that talks to GitHub and GitLab and feeds these helpers.

KPI definition:
    Wall-clock from PR open (or first push, whichever is later) to the
    first pipeline that reached green without any manual rerun for that
    same commit. Aggregated at p90 across PRs in a window.
"""

from __future__ import annotations

from dataclasses import dataclass, field
from datetime import datetime

# Pipeline sources GitLab uses when a pipeline was triggered by a human-driven
# action rather than a normal git push. Any pipeline with one of these sources
# attached to a SHA invalidates "trustworthy green" for that SHA.
MANUAL_PIPELINE_SOURCES = frozenset({"web", "api", "trigger"})

# Pipeline sources we accept as the "natural" trigger for a push. A pipeline
# from one of these is the only kind that can constitute a trustworthy green.
NATURAL_PIPELINE_SOURCES = frozenset({"push", "merge_request_event"})


@dataclass(frozen=True)
class PipelineEvent:
    """A single GitLab pipeline run for a SHA on a PR.

    `has_manually_retried_job` is the v1 heuristic: True iff any job inside
    this pipeline reports `retried=True` from a manual replay. The current
    implementation does not distinguish auto-retries (configured in
    `.gitlab-ci.yml retry:`) from manual ones, so this may overcount; the
    plan is to refine after two weeks of data.
    """

    pipeline_id: int
    sha: str
    source: str
    status: str
    created_at: datetime
    finished_at: datetime | None
    has_manually_retried_job: bool = False


@dataclass(frozen=True)
class PRSnapshot:
    """Everything we need about a PR to compute the KPI."""

    pr_id: int
    branch: str
    branch_kind: str  # "dev" | "merge_queue"
    clock_start: datetime  # PR ready-for-review timestamp, or first push if later
    pipelines: tuple[PipelineEvent, ...] = field(default_factory=tuple)
    n_pushes: int = 0
    skip_ci: bool = False
    is_draft_only: bool = False
    team: str | None = None


@dataclass(frozen=True)
class PRMetric:
    """The output of the computation for a single PR."""

    pr_id: int
    duration_s: float | None
    reached_trustworthy_green: bool
    had_manual_retry: bool
    n_pushes: int
    n_pipelines: int
    branch_kind: str
    team: str | None


def is_eligible(snapshot: PRSnapshot) -> bool:
    """A PR is part of the KPI population unless it's a no-CI PR or never left draft."""
    if snapshot.skip_ci:
        return False
    if snapshot.is_draft_only:
        return False
    return True


def find_trustworthy_green(snapshot: PRSnapshot) -> PipelineEvent | None:
    """Return the earliest pipeline that constitutes a trustworthy green for the PR.

    A pipeline `P` qualifies iff all of:
      * `P.status == "success"`,
      * `P.source` is a natural source (push / merge_request_event),
      * no earlier pipeline for the *same SHA* has a manual source,
      * no earlier pipeline for the same SHA had a manually retried job,
      * `P` itself has no manually retried jobs.

    Earliness is measured by `finished_at` (the moment the dev would see green).
    """
    finished = [p for p in snapshot.pipelines if p.finished_at is not None]
    finished.sort(key=lambda p: p.finished_at)

    for candidate in finished:
        if candidate.status != "success":
            continue
        if candidate.source not in NATURAL_PIPELINE_SOURCES:
            continue
        if candidate.has_manually_retried_job:
            continue

        # Anything for the same SHA that finished before this candidate
        # can taint it. We look at finished_at on the prior pipeline because
        # that's when the dev would have observed the manual rerun.
        tainted = False
        for prior in finished:
            if prior is candidate:
                continue
            if prior.sha != candidate.sha:
                continue
            if prior.finished_at >= candidate.finished_at:
                continue
            if prior.source in MANUAL_PIPELINE_SOURCES or prior.has_manually_retried_job:
                tainted = True
                break

        if not tainted:
            return candidate

    return None


def had_any_manual_retry(snapshot: PRSnapshot) -> bool:
    """Informational: did anyone rerun anything on this PR, ever?"""
    for p in snapshot.pipelines:
        if p.source in MANUAL_PIPELINE_SOURCES:
            return True
        if p.has_manually_retried_job:
            return True
    return False


def compute_pr_metric(snapshot: PRSnapshot) -> PRMetric:
    """Reduce a snapshot to the KPI record."""
    green = find_trustworthy_green(snapshot)
    if green is None or green.finished_at is None:
        duration_s: float | None = None
        reached = False
    else:
        delta = green.finished_at - snapshot.clock_start
        duration_s = max(delta.total_seconds(), 0.0)
        reached = True

    return PRMetric(
        pr_id=snapshot.pr_id,
        duration_s=duration_s,
        reached_trustworthy_green=reached,
        had_manual_retry=had_any_manual_retry(snapshot),
        n_pushes=snapshot.n_pushes,
        n_pipelines=len(snapshot.pipelines),
        branch_kind=snapshot.branch_kind,
        team=snapshot.team,
    )


def compute_metrics(snapshots: list[PRSnapshot]) -> list[PRMetric]:
    """Compute KPI records for the eligible subset of snapshots."""
    return [compute_pr_metric(s) for s in snapshots if is_eligible(s)]
