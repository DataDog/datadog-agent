"""
Critical Path Analysis for GitLab CI Pipelines

Computes the critical path through a pipeline execution by combining:
1. Job execution data from GitLab API (timing, status)
2. Job dependencies from GitLab CI configuration (needs clauses, stage ordering)

Usage:
    from tasks.libs.pipeline.critical_path_analysis import analyze_pipeline_critical_path

    result = analyze_pipeline_critical_path(ctx, pipeline_id=94783455)
    print(result.summary())
"""

from __future__ import annotations

from dataclasses import dataclass
from datetime import datetime

from invoke import Context

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo, resolve_gitlab_ci_configuration


@dataclass
class JobExecution:
    """Represents a job execution with timing data"""

    name: str
    stage: str
    status: str
    start: datetime
    end: datetime
    duration: float  # seconds
    job_id: int


@dataclass
class CriticalPathResult:
    """Result of critical path analysis"""

    pipeline_id: int
    pipeline_url: str
    branch: str
    status: str
    pipeline_start: datetime
    pipeline_end: datetime
    pipeline_duration: float  # seconds

    critical_path: list[JobExecution]
    total_job_duration: float  # sum of job durations on critical path
    total_wait_time: float  # sum of gaps between jobs
    gaps: list[tuple[str, str, float]]  # (from_job, to_job, gap_seconds)

    total_jobs: int  # total jobs in pipeline

    @property
    def efficiency(self) -> float:
        """Ratio of job execution time to pipeline duration"""
        if self.pipeline_duration == 0:
            return 0
        return self.total_job_duration / self.pipeline_duration

    def summary(self) -> str:
        """Return a formatted summary string"""
        lines = [
            "=" * 100,
            f"CRITICAL PATH ANALYSIS - Pipeline {self.pipeline_id}",
            "=" * 100,
            "",
            f"Pipeline URL:      {self.pipeline_url}",
            f"Branch:            {self.branch}",
            f"Status:            {self.status}",
            "",
            f"Pipeline Start:    {self.pipeline_start.strftime('%Y-%m-%d %H:%M:%S UTC')}",
            f"Pipeline End:      {self.pipeline_end.strftime('%Y-%m-%d %H:%M:%S UTC')}",
            f"Pipeline Duration: {self.pipeline_duration:.0f}s ({self.pipeline_duration/60:.1f} min)",
            "",
            "-" * 100,
            "CRITICAL PATH (jobs in chronological order):",
            "-" * 100,
            "",
            f"{'#':<3} {'Job Name':<50} {'Stage':<22} {'Duration':>10} {'Start':>10} {'End':>10}",
            "-" * 100,
        ]

        for i, job in enumerate(self.critical_path, 1):
            name = job.name[:48] + ".." if len(job.name) > 50 else job.name
            stage = job.stage[:20] + ".." if len(job.stage) > 22 else job.stage
            start_time = job.start.strftime('%H:%M:%S')
            end_time = job.end.strftime('%H:%M:%S')
            lines.append(f"{i:<3} {name:<50} {stage:<22} {job.duration:>7.0f}s  {start_time:>10} {end_time:>10}")

        lines.extend(
            [
                "-" * 100,
                "",
                "=" * 100,
                "SUMMARY",
                "=" * 100,
                "",
                f"Total jobs in pipeline:            {self.total_jobs}",
                f"Jobs on critical path:             {len(self.critical_path)}",
                f"Sum of critical path durations:    {self.total_job_duration:.0f}s ({self.total_job_duration/60:.1f} min)",
                f"Wait/queue time between jobs:      {self.total_wait_time:.0f}s ({self.total_wait_time/60:.1f} min)",
                f"Pipeline wall-clock duration:      {self.pipeline_duration:.0f}s ({self.pipeline_duration/60:.1f} min)",
                f"Critical path efficiency:          {self.efficiency * 100:.1f}%",
            ]
        )

        # Show largest gap
        if self.gaps:
            max_gap = max(self.gaps, key=lambda x: x[2])
            if max_gap[2] > 60:
                lines.extend(
                    [
                        "",
                        f"Largest wait: {max_gap[2]:.0f}s ({max_gap[2]/60:.1f} min) between:",
                        f"  - {max_gap[0]}",
                        f"  - {max_gap[1]}",
                    ]
                )

        return "\n".join(lines)

    def to_dict(self) -> dict:
        """Convert to dictionary for JSON serialization"""
        return {
            "pipeline_id": self.pipeline_id,
            "pipeline_url": self.pipeline_url,
            "branch": self.branch,
            "status": self.status,
            "pipeline_start": self.pipeline_start.isoformat(),
            "pipeline_end": self.pipeline_end.isoformat(),
            "pipeline_duration_seconds": self.pipeline_duration,
            "total_jobs": self.total_jobs,
            "critical_path": [
                {
                    "name": j.name,
                    "stage": j.stage,
                    "status": j.status,
                    "start": j.start.isoformat(),
                    "end": j.end.isoformat(),
                    "duration_seconds": j.duration,
                    "job_id": j.job_id,
                }
                for j in self.critical_path
            ],
            "total_job_duration_seconds": self.total_job_duration,
            "total_wait_time_seconds": self.total_wait_time,
            "efficiency": self.efficiency,
            "gaps": [{"from": g[0], "to": g[1], "seconds": g[2]} for g in self.gaps],
        }


def fetch_pipeline_jobs(pipeline_id: int, repo_name: str = "DataDog/datadog-agent") -> tuple[list[JobExecution], dict]:
    """
    Fetch all jobs from a pipeline via GitLab API.

    Returns:
        Tuple of (list of JobExecution, pipeline metadata dict)
    """
    repo = get_gitlab_repo(repo_name)
    pipeline = repo.pipelines.get(pipeline_id)

    pipeline_meta = {
        "id": pipeline.id,
        "url": pipeline.web_url,
        "branch": pipeline.ref,
        "status": pipeline.status,
    }

    jobs_list = list(pipeline.jobs.list(per_page=100, all=True))

    jobs = []
    for job in jobs_list:
        if job.started_at and job.finished_at:
            start = datetime.fromisoformat(job.started_at.replace('Z', '+00:00'))
            end = datetime.fromisoformat(job.finished_at.replace('Z', '+00:00'))
            duration = (end - start).total_seconds()

            jobs.append(
                JobExecution(
                    name=job.name,
                    stage=job.stage,
                    status=job.status,
                    start=start,
                    end=end,
                    duration=duration,
                    job_id=job.id,
                )
            )

    return jobs, pipeline_meta


def build_dependency_map(
    job_names: list[str],
    job_by_name: dict[str, JobExecution],
    config: dict,
    stages: list[str],
) -> dict[str, list[str]]:
    """
    Build a map of job dependencies from GitLab CI config.

    Args:
        job_names: List of job names from the pipeline
        job_by_name: Map of job name to JobExecution
        config: Resolved GitLab CI configuration
        stages: Ordered list of stages

    Returns:
        Dict mapping job_name -> list of dependency job names
    """
    stage_order = {stage: i for i, stage in enumerate(stages)}

    def get_config_needs(job_name: str) -> list[str] | None:
        """Get needs from config for a job"""
        # Exact match
        if job_name in config and isinstance(config[job_name], dict):
            if 'needs' in config[job_name]:
                return parse_needs(config[job_name]['needs'])

        # Base name match (for matrix jobs)
        base = job_name.split(':')[0].strip()
        if base in config and isinstance(config[base], dict):
            if 'needs' in config[base]:
                return parse_needs(config[base]['needs'])

        return None

    def parse_needs(needs) -> list[str]:
        """Parse needs field into list of job names"""
        result = []
        if not needs:
            return result
        for n in needs:
            if isinstance(n, str):
                result.append(n)
            elif isinstance(n, dict) and 'job' in n:
                result.append(n['job'])
        return result

    def match_need_to_jobs(need_name: str) -> list[str]:
        """Find actual job names matching a need (handles matrix jobs)"""
        matches = [j for j in job_names if j == need_name or j.startswith(need_name + ':')]
        return matches if matches else [need_name]

    dependencies = {}

    for job_name in job_names:
        job = job_by_name[job_name]
        job_stage_idx = stage_order.get(job.stage, -1)

        needs = get_config_needs(job_name)

        if needs is not None:
            # Explicit needs
            deps = []
            for need in needs:
                for m in match_need_to_jobs(need):
                    if m in job_by_name:
                        deps.append(m)
            dependencies[job_name] = deps
        else:
            # Stage-based: depends on ALL jobs from ALL earlier stages
            # (GitLab waits for all previous stages to complete)
            deps = []
            if job_stage_idx < 0:
                # Stage not in config stages list (e.g., .post, notify)
                # Depends on all jobs from all known stages
                deps = [j for j in job_names if job_by_name[j].stage in stage_order and j != job_name]
            else:
                for prev_idx in range(job_stage_idx):
                    prev_stage = stages[prev_idx]
                    deps.extend([j for j in job_names if job_by_name[j].stage == prev_stage])
            dependencies[job_name] = deps

    return dependencies


def trace_critical_path(
    start_job_name: str,
    dependencies: dict[str, list[str]],
    job_by_name: dict[str, JobExecution],
) -> list[JobExecution]:
    """
    Trace critical path backwards from a starting job.

    For each job, finds the dependency that ended latest (the "blocker").
    """
    path = []
    current = start_job_name
    visited = set()

    while current and current not in visited and current in job_by_name:
        visited.add(current)
        path.append(job_by_name[current])

        deps = [d for d in dependencies.get(current, []) if d in job_by_name]
        if not deps:
            break

        # Find dependency that ended latest
        current = max(deps, key=lambda d: job_by_name[d].end)

    path.reverse()
    return path


def analyze_pipeline_critical_path(
    ctx: Context,
    pipeline_id: int,
    repo_name: str = "DataDog/datadog-agent",
    config_file: str = ".gitlab-ci.yml",
    cached_config: dict | None = None,
) -> CriticalPathResult:
    """
    Analyze the critical path of a pipeline execution.

    Args:
        ctx: Invoke context
        pipeline_id: GitLab pipeline ID
        repo_name: GitLab repository name
        config_file: Path to GitLab CI config file
        cached_config: Pre-resolved GitLab CI config (optional, speeds up batch analysis)

    Returns:
        CriticalPathResult with analysis data
    """
    # Fetch job data
    jobs, pipeline_meta = fetch_pipeline_jobs(pipeline_id, repo_name)

    if not jobs:
        raise ValueError(f"No jobs found for pipeline {pipeline_id}")

    # Load GitLab CI config (use cached if provided)
    if cached_config is not None:
        config = cached_config
    else:
        config = resolve_gitlab_ci_configuration(ctx, config_file, resolve_only_includes=False)
    stages = config.get('stages', [])

    # Build data structures
    job_by_name = {j.name: j for j in jobs}
    job_names = list(job_by_name.keys())

    pipeline_start = min(j.start for j in jobs)
    pipeline_end = max(j.end for j in jobs)
    pipeline_duration = (pipeline_end - pipeline_start).total_seconds()

    # Build dependency map
    dependencies = build_dependency_map(job_names, job_by_name, config, stages)

    # Find last jobs
    non_post_jobs = [j for j in jobs if j.stage != '.post']
    last_job = max(jobs, key=lambda x: x.end)

    if non_post_jobs:
        latest_non_post = max(non_post_jobs, key=lambda x: x.end)
        # Trace path to latest non-.post job, then add final job
        path_to_latest = trace_critical_path(latest_non_post.name, dependencies, job_by_name)

        if last_job.stage == '.post' and last_job.name != latest_non_post.name:
            critical_path = path_to_latest + [last_job]
        else:
            critical_path = path_to_latest
    else:
        critical_path = trace_critical_path(last_job.name, dependencies, job_by_name)

    # Calculate statistics
    total_job_duration = sum(j.duration for j in critical_path)

    gaps = []
    total_wait_time = 0
    for i in range(1, len(critical_path)):
        gap = (critical_path[i].start - critical_path[i - 1].end).total_seconds()
        if gap > 0:
            total_wait_time += gap
            gaps.append((critical_path[i - 1].name, critical_path[i].name, gap))

    return CriticalPathResult(
        pipeline_id=pipeline_id,
        pipeline_url=pipeline_meta.get("url", ""),
        branch=pipeline_meta.get("branch", ""),
        status=pipeline_meta.get("status", ""),
        pipeline_start=pipeline_start,
        pipeline_end=pipeline_end,
        pipeline_duration=pipeline_duration,
        critical_path=critical_path,
        total_job_duration=total_job_duration,
        total_wait_time=total_wait_time,
        gaps=gaps,
        total_jobs=len(jobs),
    )


def analyze_multiple_pipelines(
    ctx: Context,
    pipeline_ids: list[int],
    repo_name: str = "DataDog/datadog-agent",
    config_file: str = ".gitlab-ci.yml",
) -> list[CriticalPathResult]:
    """
    Analyze critical paths for multiple pipelines.

    Args:
        ctx: Invoke context
        pipeline_ids: List of pipeline IDs to analyze
        repo_name: GitLab repository name
        config_file: Path to GitLab CI config file

    Returns:
        List of CriticalPathResult objects
    """
    # Resolve config once and reuse for all pipelines
    print("Resolving GitLab CI configuration...")
    cached_config = resolve_gitlab_ci_configuration(ctx, config_file, resolve_only_includes=False)
    print("Configuration resolved. Starting pipeline analysis...")

    results = []

    for pipeline_id in pipeline_ids:
        try:
            result = analyze_pipeline_critical_path(
                ctx, pipeline_id, repo_name, config_file, cached_config=cached_config
            )
            results.append(result)
            print(
                f"✓ Analyzed pipeline {pipeline_id}: {result.pipeline_duration:.0f}s, "
                f"{len(result.critical_path)} jobs on critical path"
            )
        except Exception as e:
            print(f"✗ Failed to analyze pipeline {pipeline_id}: {e}")

    return results


def aggregate_critical_path_stats(results: list[CriticalPathResult]) -> dict:
    """
    Aggregate statistics across multiple pipeline analyses.

    Returns dict with:
    - Average/min/max pipeline duration
    - Average/min/max critical path job count
    - Most common jobs on critical path
    - Average efficiency
    """
    if not results:
        return {}

    durations = [r.pipeline_duration for r in results]
    cp_lengths = [len(r.critical_path) for r in results]
    efficiencies = [r.efficiency for r in results]

    # Count job frequency on critical paths
    job_frequency = {}
    for r in results:
        for job in r.critical_path:
            # Normalize matrix job names
            base_name = job.name.split(':')[0].strip()
            job_frequency[base_name] = job_frequency.get(base_name, 0) + 1

    # Sort by frequency
    top_jobs = sorted(job_frequency.items(), key=lambda x: -x[1])[:10]

    return {
        "pipeline_count": len(results),
        "duration": {
            "avg": sum(durations) / len(durations),
            "min": min(durations),
            "max": max(durations),
        },
        "critical_path_length": {
            "avg": sum(cp_lengths) / len(cp_lengths),
            "min": min(cp_lengths),
            "max": max(cp_lengths),
        },
        "efficiency": {
            "avg": sum(efficiencies) / len(efficiencies),
            "min": min(efficiencies),
            "max": max(efficiencies),
        },
        "top_critical_path_jobs": top_jobs,
    }
