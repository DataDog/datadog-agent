"""
GitLab CI Execution Graph Generator

Parses GitLab CI configuration and generates a DAG (Directed Acyclic Graph)
representing job execution order based on:
- Stage ordering: jobs in a stage start after all jobs from the previous stage finish
- needs clause: if defined, a job starts as soon as its specified dependencies finish
  (overriding stage-based dependencies)

Output format:
{
    "nodes": [{"name": "job_name", "stage": "stage_name"}, ...],
    "edges": [{"from": "dependency_job", "to": "dependent_job"}, ...]
}
"""

from __future__ import annotations

import json
from dataclasses import dataclass, field
from typing import Any

from tasks.libs.ciproviders.gitlab_api import (
    CONFIG_SPECIAL_OBJECTS,
    ReferenceTag,
    expand_matrix_jobs,
    is_leaf_job,
    resolve_gitlab_ci_configuration,
)


@dataclass
class JobInfo:
    """Information about a CI job"""

    name: str
    stage: str
    needs: list[str] = field(default_factory=list)
    has_needs: bool = False  # True if job explicitly defines needs clause


@dataclass
class ExecutionGraph:
    """Represents the execution graph of a GitLab CI pipeline"""

    nodes: list[dict[str, str]] = field(default_factory=list)
    edges: list[dict[str, str]] = field(default_factory=list)

    def add_node(self, name: str, stage: str):
        self.nodes.append({"name": name, "stage": stage})

    def add_edge(self, from_job: str, to_job: str):
        self.edges.append({"from": from_job, "to": to_job})

    def to_dict(self) -> dict:
        return {"nodes": self.nodes, "edges": self.edges}

    def to_json(self, indent: int = 2) -> str:
        return json.dumps(self.to_dict(), indent=indent)


def parse_needs_field(needs: Any) -> list[str]:
    """
    Parse the 'needs' field which can have multiple formats.

    Formats:
    - needs: ["job1", "job2"]
    - needs: [{job: "job1"}, {job: "job2", artifacts: true}]
    - needs: [{pipeline: "$UPSTREAM_PIPELINE_ID"}]  # ignored for our purposes
    - needs: [!reference [.some_template]]  # reference tags

    Args:
        needs: The needs field value from GitLab CI config

    Returns:
        List of job names this job depends on
    """
    if not needs:
        return []

    dependencies = []

    for item in needs:
        if isinstance(item, str):
            # Simple string format: "job_name"
            dependencies.append(item)
        elif isinstance(item, dict):
            # Dict format with 'job' key
            if "job" in item:
                dependencies.append(item["job"])
            # Skip pipeline dependencies as they're external
        elif isinstance(item, ReferenceTag):
            # Reference tags are resolved by GitLab, skip them
            # They point to template definitions
            pass

    return dependencies


def extract_jobs_from_config(config: dict) -> dict[str, JobInfo]:
    """
    Extract all jobs from the GitLab CI configuration.

    Args:
        config: The resolved GitLab CI configuration

    Returns:
        Dict mapping job name to JobInfo
    """
    jobs = {}

    for key, value in config.items():
        # Skip non-job entries
        if key in CONFIG_SPECIAL_OBJECTS:
            continue
        if not isinstance(value, dict):
            continue
        if not is_leaf_job(key, value):
            continue

        stage = value.get("stage", "test")  # Default stage is "test" in GitLab

        has_needs = "needs" in value
        needs = []
        if has_needs:
            needs = parse_needs_field(value["needs"])

        jobs[key] = JobInfo(name=key, stage=stage, needs=needs, has_needs=has_needs)

    return jobs


def get_stages_order(config: dict) -> list[str]:
    """
    Get the ordered list of stages from the configuration.

    Args:
        config: The resolved GitLab CI configuration

    Returns:
        List of stage names in execution order
    """
    if "stages" in config:
        return list(config["stages"])

    # Default GitLab stages if not specified
    return ["build", "test", "deploy"]


def build_execution_graph(config: dict, expand_matrix: bool = True) -> ExecutionGraph:
    """
    Build the execution graph from GitLab CI configuration.

    Rules:
    1. A job with no 'needs' clause depends on ALL jobs from the previous stage
    2. A job with 'needs' clause only depends on the specified jobs (regardless of stage)
    3. Empty 'needs: []' means the job has no dependencies and can start immediately

    Args:
        config: The resolved GitLab CI configuration
        expand_matrix: Whether to expand matrix jobs into individual jobs

    Returns:
        ExecutionGraph with nodes and edges
    """
    # Optionally expand matrix jobs
    if expand_matrix:
        config = expand_matrix_jobs(config.copy())

    jobs = extract_jobs_from_config(config)
    stages = get_stages_order(config)

    # Create stage -> jobs mapping
    stage_jobs: dict[str, list[str]] = {stage: [] for stage in stages}
    for job_name, job_info in jobs.items():
        if job_info.stage in stage_jobs:
            stage_jobs[job_info.stage].append(job_name)
        else:
            # Job references a stage not in the stages list, add it
            stage_jobs[job_info.stage] = [job_name]
            stages.append(job_info.stage)

    graph = ExecutionGraph()

    # Add all nodes
    for job_name, job_info in jobs.items():
        graph.add_node(job_name, job_info.stage)

    # Build edges
    for job_name, job_info in jobs.items():
        if job_info.has_needs:
            # Job has explicit needs - use those as dependencies
            for dep_name in job_info.needs:
                # Handle matrix job dependencies
                matching_jobs = find_matching_jobs(dep_name, jobs)
                for matched_job in matching_jobs:
                    graph.add_edge(matched_job, job_name)
        else:
            # No needs clause - depend on all jobs from previous stage
            job_stage_index = stages.index(job_info.stage) if job_info.stage in stages else -1
            if job_stage_index > 0:
                prev_stage = stages[job_stage_index - 1]
                for prev_job_name in stage_jobs.get(prev_stage, []):
                    graph.add_edge(prev_job_name, job_name)

    return graph


def find_matching_jobs(dep_name: str, jobs: dict[str, JobInfo]) -> list[str]:
    """
    Find jobs matching a dependency name.

    Handles cases where:
    - Exact match exists
    - Matrix job reference (base name matches expanded jobs)

    Args:
        dep_name: The dependency name from needs clause
        jobs: Dict of all jobs

    Returns:
        List of matching job names
    """
    # Exact match
    if dep_name in jobs:
        return [dep_name]

    # Check for matrix job pattern: "base_job" matching "base_job: [variant1, variant2]"
    matching = []
    for job_name in jobs:
        # Check if job_name is a matrix expansion of dep_name
        if job_name.startswith(dep_name + ":"):
            matching.append(job_name)

    if matching:
        return matching

    # No match found - return original (might be an external dependency or template)
    return [dep_name]


def generate_execution_graph(
    ctx,
    input_file: str = ".gitlab-ci.yml",
    resolve_only_includes: bool = False,
    expand_matrix: bool = True,
    output_file: str | None = None,
) -> ExecutionGraph:
    """
    Generate the execution graph from a GitLab CI configuration file.

    Args:
        ctx: Invoke context
        input_file: Path to the GitLab CI configuration file
        resolve_only_includes: If True, only resolve includes (faster, offline).
                              If False (default), fully resolve via GitLab API
                              (handles extends, references - requires API access).
        expand_matrix: Whether to expand matrix jobs
        output_file: If provided, write JSON output to this file

    Returns:
        ExecutionGraph object

    Note:
        Using resolve_only_includes=True is faster but won't resolve:
        - extends: Templates/anchors won't be merged
        - !reference: Reference tags won't be expanded
        This means jobs may have incorrect stages/needs if inherited from templates.
        For accurate results, use resolve_only_includes=False (requires GitLab API access).
    """
    config = resolve_gitlab_ci_configuration(ctx, input_file, resolve_only_includes=resolve_only_includes)

    graph = build_execution_graph(config, expand_matrix=expand_matrix)

    if output_file:
        with open(output_file, "w") as f:
            f.write(graph.to_json())

    return graph


# Standalone execution support
if __name__ == "__main__":
    import argparse
    import sys

    from invoke import Context

    parser = argparse.ArgumentParser(description="Generate GitLab CI execution graph")
    parser.add_argument(
        "-i",
        "--input",
        default=".gitlab-ci.yml",
        help="Input GitLab CI configuration file (default: .gitlab-ci.yml)",
    )
    parser.add_argument(
        "-o",
        "--output",
        help="Output JSON file (default: stdout)",
    )
    parser.add_argument(
        "--no-expand-matrix",
        action="store_true",
        help="Do not expand matrix jobs",
    )
    parser.add_argument(
        "--offline",
        action="store_true",
        help="Only resolve includes (faster but may miss extends/references)",
    )

    args = parser.parse_args()

    ctx = Context()

    try:
        graph = generate_execution_graph(
            ctx,
            input_file=args.input,
            resolve_only_includes=args.offline,
            expand_matrix=not args.no_expand_matrix,
            output_file=args.output,
        )

        if not args.output:
            print(graph.to_json())

    except Exception as e:
        print(f"Error: {e}", file=sys.stderr)
        sys.exit(1)
