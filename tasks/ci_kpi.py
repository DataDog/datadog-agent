"""
Compute the "Time-to-Trustworthy-Green per PR" CI KPI and emit it to Datadog.

See `tasks/libs/pipeline/ci_kpi.py` for the KPI definition and the
`compute_pr_metric` algorithm. This module is the I/O layer: it walks PRs
on GitHub, joins them to pipelines on GitLab by commit SHA, builds
`PRSnapshot`s, and submits per-PR metrics to Datadog.

Usage:
    dda inv ci-kpi.compute --window=14 --dry-run
    dda inv ci-kpi.compute --window=14
"""

from __future__ import annotations

import sys
from datetime import datetime, timedelta, timezone

from invoke import task
from invoke.context import Context

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.datadog_api import create_count, create_gauge, send_metrics
from tasks.libs.pipeline.ci_kpi import (
    PipelineEvent,
    PRMetric,
    PRSnapshot,
    compute_metrics,
)

SKIP_CI_LABELS = {"qa/skip-qa", "skip-ci", "no-ci"}
GITLAB_PROJECT = "DataDog/datadog-agent"


def _to_utc(dt: datetime | None) -> datetime | None:
    if dt is None:
        return None
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def _require_utc(dt: datetime) -> datetime:
    """Like `_to_utc` but the input is guaranteed non-None — keeps type checkers happy."""
    if dt.tzinfo is None:
        return dt.replace(tzinfo=timezone.utc)
    return dt.astimezone(timezone.utc)


def _classify_branch(branch: str) -> str:
    if branch.startswith("gh-readonly-queue/") or "/merge-queue/" in branch:
        return "merge_queue"
    return "dev"


def _build_snapshot(pr, gitlab_repo) -> PRSnapshot | None:
    """Pull GitHub PR + GitLab pipelines and build a PRSnapshot.

    Returns None if the PR has no commits we can inspect (e.g. transient
    GH state); the caller should skip such PRs without failing the run.
    """
    commits = list(pr.get_commits())
    if not commits:
        return None

    shas = [c.sha for c in commits]

    # Earliest commit timestamp gives us "first push" — the clock can't
    # start before code existed on the PR.
    first_commit_ts = min(_require_utc(c.commit.author.date) for c in commits)
    pr_opened = _require_utc(pr.created_at)
    clock_start = max(first_commit_ts, pr_opened)

    # Collect pipelines for every SHA the PR carried (force-pushes included).
    pipelines: list[PipelineEvent] = []
    for sha in shas:
        try:
            for p in gitlab_repo.pipelines.list(sha=sha, get_all=True):
                pipelines.append(
                    PipelineEvent(
                        pipeline_id=int(p.id),
                        sha=sha,
                        source=str(p.source),
                        status=str(p.status),
                        created_at=_to_utc(_parse_gitlab_ts(p.created_at)),
                        finished_at=_to_utc(_parse_gitlab_ts(p.finished_at)),
                    )
                )
        except Exception as e:
            # One bad SHA shouldn't fail the whole window. Surface and skip.
            print(f"[ci-kpi] skipping sha {sha[:8]} on PR #{pr.number}: {e}", file=sys.stderr)

    labels = {label.name for label in pr.get_labels()}
    skip_ci = bool(labels & SKIP_CI_LABELS)
    is_draft_only = pr.draft and pr.state == "closed" and not pr.merged

    return PRSnapshot(
        pr_id=pr.number,
        branch=pr.head.ref,
        branch_kind=_classify_branch(pr.head.ref),
        clock_start=clock_start,
        pipelines=tuple(pipelines),
        n_pushes=len(shas),
        skip_ci=skip_ci,
        is_draft_only=is_draft_only,
    )


def _parse_gitlab_ts(value) -> datetime | None:
    if value is None:
        return None
    if isinstance(value, datetime):
        return value
    return datetime.fromisoformat(str(value).replace("Z", "+00:00"))


def _list_recent_prs(gh: GithubAPI, since: datetime):
    """Yield PRs touched (created/updated/closed) since the cutoff.

    GitHub's API returns PRs sorted by `updated` desc, so we can stop
    iterating once we go past the cutoff.
    """
    for pr in gh.repo.get_pulls(state="all", sort="updated", direction="desc"):
        if _to_utc(pr.updated_at) < since:
            break
        yield pr


def _series_for(metric: PRMetric, ts: int):
    common_tags = [
        f"pr_id:{metric.pr_id}",
        f"branch_kind:{metric.branch_kind}",
        f"had_manual_retry:{str(metric.had_manual_retry).lower()}",
        "repository:datadog-agent",
    ]
    if metric.team:
        common_tags.append(f"team:{metric.team}")

    series = []
    if metric.duration_s is not None:
        series.append(
            create_gauge(
                metric_name="datadog.ci.time_to_trustworthy_green",
                timestamp=ts,
                value=metric.duration_s,
                tags=common_tags,
                unit="second",
            )
        )
    series.append(
        create_gauge(
            metric_name="datadog.ci.pr_pushes",
            timestamp=ts,
            value=metric.n_pushes,
            tags=common_tags,
        )
    )
    if not metric.reached_trustworthy_green:
        series.append(
            create_count(
                metric_name="datadog.ci.pr_never_green_count",
                timestamp=ts,
                value=1,
                tags=common_tags,
            )
        )
    return series


@task
def compute(ctx: Context, window: int = 14, dry_run: bool = False):
    """Compute time-to-trustworthy-green for PRs touched in the last `window` days.

    With `--dry-run`, prints the per-PR records without submitting metrics.
    """
    if window <= 0:
        print("--window must be positive", file=sys.stderr)
        sys.exit(2)

    cutoff = datetime.now(timezone.utc) - timedelta(days=window)
    gh = GithubAPI(repository=GITHUB_REPO_NAME)
    gitlab_repo = get_gitlab_repo(GITLAB_PROJECT)

    snapshots: list[PRSnapshot] = []
    for pr in _list_recent_prs(gh, cutoff):
        try:
            snap = _build_snapshot(pr, gitlab_repo)
        except Exception as e:
            print(f"[ci-kpi] skipping PR #{pr.number}: {e}", file=sys.stderr)
            continue
        if snap is not None:
            snapshots.append(snap)

    metrics = compute_metrics(snapshots)
    ts = int(datetime.now(timezone.utc).timestamp())

    if dry_run:
        for m in metrics:
            duration = f"{m.duration_s:.0f}s" if m.duration_s is not None else "never-green"
            print(
                f"PR #{m.pr_id}\tduration={duration}\tpushes={m.n_pushes}\t"
                f"pipelines={m.n_pipelines}\tmanual_retry={m.had_manual_retry}\t"
                f"branch_kind={m.branch_kind}"
            )
        print(f"\n{len(metrics)} PRs evaluated, dry-run (no metrics submitted).")
        return

    series = []
    for m in metrics:
        series.extend(_series_for(m, ts))

    if not series:
        print("[ci-kpi] no metrics to submit")
        return

    send_metrics(series)
    print(f"[ci-kpi] submitted {len(series)} series for {len(metrics)} PRs")
