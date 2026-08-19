"""Fetch and aggregate UTOF documents across every job of a GitLab CI pipeline."""

from __future__ import annotations

import concurrent.futures
from dataclasses import asdict, dataclass, field
from functools import partial
from typing import TYPE_CHECKING

from tasks.libs.testing.utof.models import UTOFDocument, UTOFSummary, UTOFTestResult, _strip_none, walk_tests
from tasks.libs.testing.utof.report import _get_test_failure

if TYPE_CHECKING:
    from gitlab.v4.objects import Project, ProjectPipeline, ProjectPipelineJob

# The UTOF artifact filename each stage's jobs archive it under — see
# .gitlab/build/source_test/*.yml (TEST_OUTPUT_FILE) and
# .gitlab/test/e2e/e2e.yml (E2E_RESULT_JSON_UNIFIED).
STAGE_ARTIFACT_NAME = {
    "source_test": "test_output_unified.json",
    "e2e": "e2e_test_output_unified.json",
    "e2e_pre_test": "e2e_test_output_unified.json",
}

# Only these stages run jobs that call generate_unified_output(); probing
# every job in a pipeline would multiply API calls for no benefit.
RELEVANT_STAGES = frozenset(STAGE_ARTIFACT_NAME)

# Only failed jobs are worth reporting on here — passed/canceled/manual/skipped/
# still-running jobs are never probed, which also means their test counts never
# feed into the aggregate summary (only failed jobs' totals are reflected there).
JOB_STATUS_TO_PROBE = "failed"

# Number of jobs to fetch artifacts for concurrently. Each fetch is a
# blocking HTTP round trip to GitLab; a pipeline can have hundreds of
# relevant jobs, so doing this serially dominates the command's runtime.
DEFAULT_FETCH_CONCURRENCY = 16


@dataclass
class JobUTOFResult:
    job_name: str
    job_url: str
    job_status: str
    utof: UTOFDocument | None = None
    # Set when the job failed/errored and no UTOF artifact could be found,
    # so the job isn't silently missing from the pipeline overview.
    error: str | None = None


@dataclass
class PipelineUTOFAggregate:
    pipeline_id: str
    pipeline_url: str
    jobs: list[JobUTOFResult]
    failures: list[tuple[JobUTOFResult, UTOFTestResult]] = field(default_factory=list)
    flaky: list[tuple[JobUTOFResult, UTOFTestResult]] = field(default_factory=list)
    no_data_jobs: list[JobUTOFResult] = field(default_factory=list)
    summary: UTOFSummary = field(default_factory=UTOFSummary)

    def to_dict(self) -> dict:
        return _strip_none(
            {
                "pipeline_id": self.pipeline_id,
                "pipeline_url": self.pipeline_url,
                "jobs_checked": len(self.jobs),
                "summary": asdict(self.summary),
                "failures": [_test_dict(job, t) for job, t in self.failures],
                "flaky": [_test_dict(job, t) for job, t in self.flaky],
                "no_data_jobs": [
                    {
                        "job_name": j.job_name,
                        "job_url": j.job_url,
                        "job_status": j.job_status,
                        "error": j.error,
                    }
                    for j in self.no_data_jobs
                ],
            }
        )


def fetch_pipeline_utof_results(
    repo: Project, pipeline: ProjectPipeline, max_workers: int = DEFAULT_FETCH_CONCURRENCY
) -> list[JobUTOFResult]:
    """Fetch the UTOF document for every failed UTOF-emitting job in a pipeline.

    Only the latest attempt of each job is considered: GitLab's job list
    excludes older retries unless include_retried=True is passed.

    Artifact fetches are one blocking HTTP call each, so they're run
    concurrently across jobs — a pipeline can have hundreds of relevant
    jobs, and doing this serially dominates the command's runtime.
    """
    candidates = [
        job
        for job in pipeline.jobs.list(per_page=100, all=True)
        if job.stage in RELEVANT_STAGES and job.status == JOB_STATUS_TO_PROBE
    ]

    with concurrent.futures.ThreadPoolExecutor(max_workers=max_workers) as executor:
        utofs = list(executor.map(partial(_fetch_job_utof, repo), candidates))

    results: list[JobUTOFResult] = []
    for job, utof in zip(candidates, utofs, strict=False):
        if utof is not None:
            results.append(JobUTOFResult(job_name=job.name, job_url=job.web_url, job_status=job.status, utof=utof))
        else:
            results.append(
                JobUTOFResult(
                    job_name=job.name,
                    job_url=job.web_url,
                    job_status=job.status,
                    error=f"job {job.status}, no UTOF artifact found",
                )
            )

    return results


def _fetch_job_utof(repo: Project, job: ProjectPipelineJob) -> UTOFDocument | None:
    name = STAGE_ARTIFACT_NAME.get(job.stage)
    if name is None:
        return None

    project_job = repo.jobs.get(job.id, lazy=True)
    data = _artifact_or_none(project_job, name)
    if data is None:
        return None
    try:
        return UTOFDocument.from_json(data)
    except (ValueError, KeyError, TypeError) as e:
        print(f"Warning: failed to parse UTOF artifact {name} for job {job.name}: {e}")
        return None


def _artifact_or_none(project_job, name: str) -> bytes | None:
    try:
        data = project_job.artifact(name)
    except Exception:
        return None
    return data if isinstance(data, bytes) else None


def aggregate_results(pipeline_id: str, pipeline_url: str, jobs: list[JobUTOFResult]) -> PipelineUTOFAggregate:
    agg = PipelineUTOFAggregate(pipeline_id=pipeline_id, pipeline_url=pipeline_url, jobs=jobs)

    for job in jobs:
        if job.utof is None:
            agg.no_data_jobs.append(job)
            continue

        s = job.utof.summary
        agg.summary.total += s.total
        agg.summary.passed += s.passed
        agg.summary.failed += s.failed
        agg.summary.skipped += s.skipped
        agg.summary.flaky += s.flaky
        agg.summary.retried += s.retried

        for t in walk_tests(job.utof.tests):
            if t.status == "fail":
                agg.failures.append((job, t))
            elif t.status in ("flaky_fail", "flaky_pass"):
                agg.flaky.append((job, t))

    agg.summary.status = "fail" if agg.summary.failed else "pass"
    return agg


def _test_dict(job: JobUTOFResult, t: UTOFTestResult) -> dict:
    failure = _get_test_failure(t)
    return {
        "job_name": job.job_name,
        "job_url": job.job_url,
        "package": t.package,
        "test": t.full_name,
        "status": t.status,
        "duration_seconds": t.duration_seconds,
        "retry_count": t.retry_count,
        "flaky_source": t.flaky.source if t.flaky else None,
        "failure_type": failure.type if failure else None,
        "message": failure.message if failure else None,
    }
