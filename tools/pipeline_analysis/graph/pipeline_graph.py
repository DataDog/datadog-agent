"""
NetworkX DAG builder for GitLab CI pipeline jobs.

Nodes: one per job.
Edges: job A → job B means B needs A (B depends on A).

Node attributes:
  - stage: str
  - platform: str (linux/windows/mac/arm/unknown)
  - job_type: str (build/test/deploy/trigger/other)
  - produces_s3: list[str]
  - consumes_s3: list[str]
  - tags: list[str]
  - image: str | None
"""

from __future__ import annotations

from pathlib import Path
from typing import Any

import networkx as nx

from pipeline_analysis.parsers.gitlab_ci import GitLabCIParser, Job


class PipelineGraph:
    """Build and query a NetworkX DiGraph of CI pipeline jobs."""

    def __init__(self, repo_root: str | Path):
        self.repo_root = Path(repo_root)
        self._parser = GitLabCIParser(self.repo_root)
        self._jobs: list[Job] = []
        self._job_map: dict[str, Job] = {}
        self.G: nx.DiGraph = nx.DiGraph()
        self._built = False

    # ------------------------------------------------------------------
    # Build
    # ------------------------------------------------------------------

    def build(self) -> nx.DiGraph:
        """Parse the pipeline and build the graph. Returns the DiGraph."""
        if self._built:
            return self.G

        self._jobs = self._parser.parse()
        self._job_map = {j.name: j for j in self._jobs}

        # Add all nodes first
        for job in self._jobs:
            self.G.add_node(
                job.name,
                stage=job.stage,
                platform=job.platform,
                job_type=job.job_type,
                produces_s3=job.s3_produces,
                consumes_s3=job.s3_consumes,
                tags=job.tags,
                image=job.image,
            )

        # Add edges from needs:
        for job in self._jobs:
            for dep_name in job.needs:
                if dep_name in self._job_map:
                    # Edge: dep_name → job.name  (dep must run before job)
                    self.G.add_edge(dep_name, job.name)
                # If dep not found, it may be from a cross-pipeline reference; skip.

        self._built = True
        return self.G

    @property
    def stages(self) -> list[str]:
        return self._parser.stages

    @property
    def variables(self) -> dict[str, str]:
        return self._parser.variables

    @property
    def jobs(self) -> list[Job]:
        return self._jobs

    def job(self, name: str) -> Job | None:
        return self._job_map.get(name)

    # ------------------------------------------------------------------
    # Query helpers
    # ------------------------------------------------------------------

    def jobs_in_stage(self, stage: str) -> list[Job]:
        return [j for j in self._jobs if j.stage == stage]

    def direct_needs(self, job_name: str) -> list[str]:
        """Jobs that `job_name` directly depends on."""
        return list(self.G.predecessors(job_name))

    def transitive_needs(self, job_name: str) -> list[str]:
        """All transitive ancestors of `job_name` (topological order)."""
        ancestors = nx.ancestors(self.G, job_name)
        subgraph = self.G.subgraph(ancestors | {job_name})
        order = list(nx.topological_sort(subgraph))
        # Return everything except the job itself
        return [n for n in order if n != job_name]

    def subgraph_for_job(self, job_name: str) -> nx.DiGraph:
        """Return subgraph containing job and all its transitive ancestors."""
        ancestors = nx.ancestors(self.G, job_name)
        return self.G.subgraph(ancestors | {job_name}).copy()

    def stats(self) -> dict[str, Any]:
        """Return summary statistics about the graph."""
        stage_counts: dict[str, int] = {}
        platform_counts: dict[str, int] = {}
        type_counts: dict[str, int] = {}

        for _, attrs in self.G.nodes(data=True):
            stage = attrs.get("stage", "unknown")
            platform = attrs.get("platform", "unknown")
            jtype = attrs.get("job_type", "other")
            stage_counts[stage] = stage_counts.get(stage, 0) + 1
            platform_counts[platform] = platform_counts.get(platform, 0) + 1
            type_counts[jtype] = type_counts.get(jtype, 0) + 1

        return {
            "total_jobs": self.G.number_of_nodes(),
            "total_edges": self.G.number_of_edges(),
            "stages": len(self.stages),
            "stage_counts": stage_counts,
            "platform_counts": platform_counts,
            "type_counts": type_counts,
            "is_dag": nx.is_directed_acyclic_graph(self.G),
        }
