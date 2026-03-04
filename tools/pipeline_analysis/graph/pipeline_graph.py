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

import re
from collections import defaultdict
from pathlib import Path
from typing import Any

import networkx as nx

from pipeline_analysis.parsers.gitlab_ci import GitLabCIParser, Job

# ---------------------------------------------------------------------------
# Fold rules — strip known "dimension" tokens from job names to find a stem.
# Jobs sharing the same stem within a stage are collapsed into one node.
# ---------------------------------------------------------------------------

# Order matters in alternations: longer tokens before shorter ones.
_ARCH_RE = re.compile(r'[_-](arm64|x64|amd64|armhf|x86_64|x86_32)(?=[_-]|$)')
_FIPS_RE = re.compile(r'[_-](fips)(?=[_-]|$)')
_MANUAL_RE = re.compile(r'[_-](manual)$')
_OS_RE = re.compile(r'[_-](amazonlinux|centos|debian|ubuntu|suse|rhel|macos)(?=[_-]|$)')
_FORMAT_RE = re.compile(r'[_-](suse_rpm|deb|rpm|tar)(?=[_-]|$)')

# Stages where additional OS / format folding is applied
_OS_FOLD_STAGES = {'e2e_install_packages', 'install_script_testing'}
_FORMAT_FOLD_STAGES = {'packaging', 'deploy_packages', 'package_build'}

# Arch tokens — used to sort variants for display
_ARCH_TOKENS = {'arm64', 'x64', 'amd64', 'armhf', 'x86_64', 'x86_32'}


def _strip_dim(name: str, pattern: re.Pattern) -> tuple[str, str | None]:
    """Remove the first match of *pattern* from *name*.

    Returns ``(cleaned_name, dim_value)`` where *dim_value* is the captured
    group (the token without the leading separator), or ``None`` if no match.
    Double separators left after removal are collapsed to one.
    """
    m = pattern.search(name)
    if not m:
        return name, None
    dim_val = m.group(1)
    new_name = name[: m.start()] + name[m.end() :]
    new_name = re.sub(r"[-_]{2,}", lambda x: x.group(0)[0], new_name).strip("-_")
    return new_name, dim_val


def compute_fold_stem(job_name: str, stage: str) -> tuple[str, list[str]]:
    """Return ``(stem, dims)`` after stripping known dimension tokens.

    Dimensions stripped in order:

    1. ``_manual`` suffix
    2. ``[_-]fips`` and ``[_-]<arch>`` — iterated to handle either order
    3. OS token — only for ``e2e_install_packages`` / ``install_script_testing``
    4. Package-format token — only for ``packaging`` / ``deploy_packages`` /
       ``package_build``
    """
    dims: list[str] = []
    n = job_name

    # 1. manual (always a suffix)
    n, v = _strip_dim(n, _MANUAL_RE)
    if v:
        dims.append(v)

    # 2. fips + arch — iterate up to 3 rounds (they can appear in either order)
    for _ in range(3):
        n, v = _strip_dim(n, _FIPS_RE)
        if v:
            dims.append(v)
        n, v = _strip_dim(n, _ARCH_RE)
        if v:
            dims.append(v)

    # 3. OS (stage-specific)
    if stage in _OS_FOLD_STAGES:
        n, v = _strip_dim(n, _OS_RE)
        if v:
            dims.append(v)

    # 4. Package format (stage-specific)
    if stage in _FORMAT_FOLD_STAGES:
        n, v = _strip_dim(n, _FORMAT_RE)
        if v:
            dims.append(v)

    return n, dims


def _variant_label(dims: list[str]) -> str:
    """Return a compact variant string like ``arm64+fips`` or ``base``."""
    if not dims:
        return "base"
    # Sort: arch first, then fips, then the rest alphabetically
    arch = [d for d in dims if d in _ARCH_TOKENS]
    fips = [d for d in dims if d == "fips"]
    rest = sorted(d for d in dims if d not in _ARCH_TOKENS and d != "fips")
    return "+".join(arch + fips + rest)


def _fold_node_label(stem: str, variants: list[str], max_show: int = 8) -> str:
    """Format the display label for a folded node."""
    shown = sorted(set(variants))
    if len(shown) > max_show:
        extra = len(shown) - max_show
        shown = shown[:max_show] + [f"+{extra}"]
    return f"{stem}\n[{', '.join(shown)}]"


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

    def build_folded(self) -> nx.DiGraph:
        """Return a new DiGraph with similar jobs collapsed into single nodes.

        Jobs sharing the same stem (after stripping arch / fips / manual / OS /
        format dimension tokens) within the same stage are merged.  Each merged
        node carries a ``label`` attribute for display, e.g.::

            kmt_sysprobe_cleanup
            [arm64, arm64+manual, x64, x64+manual]

        Singleton nodes keep their original name as both node-id and label.
        Edges are the union of all member edges; intra-group edges become
        self-loops and are dropped.
        """
        if not self._built:
            self.build()

        # 1. Compute (stem, variant) per job
        job_stem: dict[str, str] = {}
        job_variant: dict[str, str] = {}
        for job in self._jobs:
            stem, dims = compute_fold_stem(job.name, job.stage)
            job_stem[job.name] = stem
            job_variant[job.name] = _variant_label(dims)

        # 2. Group by (stage, stem)
        groups: dict[tuple[str, str], list[str]] = defaultdict(list)
        for job in self._jobs:
            groups[(job.stage, job_stem[job.name])].append(job.name)

        # 3. Assign a node-id for each group
        #    Singletons: original name.  Groups: stem.
        node_of: dict[str, str] = {}  # job_name -> node_id in FG
        for (_stage, stem), members in groups.items():
            nid = members[0] if len(members) == 1 else stem
            for m in members:
                node_of[m] = nid

        # 4. Build folded graph
        FG = nx.DiGraph()
        for (stage, stem), members in groups.items():
            rep = self._job_map[members[0]]
            nid = node_of[members[0]]

            if len(members) == 1:
                label = members[0]
            else:
                variants = [job_variant[m] for m in members]
                label = _fold_node_label(stem, variants)

            # Merge S3 attributes
            produces: list[str] = []
            consumes: list[str] = []
            for m in members:
                produces.extend(self.G.nodes[m].get("produces_s3", []))
                consumes.extend(self.G.nodes[m].get("consumes_s3", []))

            FG.add_node(
                nid,
                stage=stage,
                platform=rep.platform,
                job_type=rep.job_type,
                label=label,
                member_count=len(members),
                members=members,
                produces_s3=list(set(produces)),
                consumes_s3=list(set(consumes)),
            )

        # 5. Merge edges (drop intra-group self-loops)
        for job_name in self.G.nodes:
            dst = node_of[job_name]
            for pred_name in self.G.predecessors(job_name):
                src = node_of[pred_name]
                if src != dst:
                    FG.add_edge(src, dst)

        return FG

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
