"""
Graphviz DOT → SVG renderer for the CI pipeline graph.

Three render modes:
1. stages  — cluster-per-stage, inter-stage edges only (summarized)
2. jobs    — all job nodes, needs: edges, colored by platform
3. artifact — jobs + S3 artifact nodes, producer→artifact→consumer
"""

from __future__ import annotations

import re
from pathlib import Path

import networkx as nx

try:
    import graphviz

    HAS_GRAPHVIZ = True
except ImportError:
    HAS_GRAPHVIZ = False


# ---------------------------------------------------------------------------
# Color palette
# ---------------------------------------------------------------------------

_COLORS = {
    # Job types
    "build": "#cce5ff",  # light blue
    "test": "#ccffcc",  # light green
    "deploy": "#ffe5cc",  # light orange
    "trigger": "#e5ccff",  # purple
    "other": "#f0f0f0",  # grey
    # Platform overlays (border color)
    "windows": "#cccc00",  # dark yellow
    "mac": "#cc8800",  # dark orange
    "arm": "#cc00cc",  # purple
    "linux": "#333333",  # dark grey
}

_STAGE_TYPE_MAP = {
    "binary_build": "build",
    "package_deps_build": "build",
    "package_build": "build",
    "packaging": "build",
    "container_build": "build",
    "deps_build": "build",
    "deps_fetch": "build",
    "choco_build": "build",
    "source_test": "test",
    "lint": "test",
    "integration_test": "test",
    "functional_test": "test",
    "benchmarks": "test",
    "e2e": "test",
    "e2e_k8s": "test",
    "e2e_install_packages": "test",
    "kernel_matrix_testing_system_probe": "test",
    "kernel_matrix_testing_security_agent": "test",
    "install_script_testing": "test",
    "dynamic_test": "test",
    "scan": "test",
    "software_composition_analysis": "test",
    "container_scan": "test",
    "deploy_packages": "deploy",
    "trigger_distribution": "deploy",
    "dev_container_deploy": "deploy",
    "install_script_deploy": "deploy",
    "internal_image_deploy": "deploy",
    "e2e_deploy": "deploy",
    "internal_kubernetes_deploy": "deploy",
    "post_rc_build": "deploy",
}


def _job_color(attrs: dict) -> str:
    jtype = attrs.get("job_type", "other")
    return _COLORS.get(jtype, _COLORS["other"])


def _border_color(attrs: dict) -> str:
    platform = attrs.get("platform", "linux")
    return _COLORS.get(platform, _COLORS["linux"])


# ---------------------------------------------------------------------------
# Stage view
# ---------------------------------------------------------------------------


def render_stages(
    G: nx.DiGraph,
    stages: list[str],
    output_path: str | Path,
    fmt: str = "svg",
) -> Path:
    """
    Render a stage-level summary: one subgraph cluster per stage,
    showing job counts and inter-stage dependency edges.
    """
    if not HAS_GRAPHVIZ:
        raise ImportError("graphviz package is required: pip install graphviz")

    output_path = Path(output_path)
    dot = graphviz.Digraph(
        name="pipeline_stages",
        comment="Datadog Agent CI Pipeline — Stage View",
        format=fmt,
    )
    dot.attr(rankdir="LR", fontsize="10", fontname="Helvetica", splines="ortho")
    dot.attr("node", fontname="Helvetica", fontsize="9", shape="box", style="filled,rounded")
    dot.attr("edge", fontname="Helvetica", fontsize="8")

    # Group jobs by stage
    stage_jobs: dict[str, list[str]] = {s: [] for s in stages}
    ungrouped: list[str] = []
    for node, attrs in G.nodes(data=True):
        stage = attrs.get("stage", "")
        if stage in stage_jobs:
            stage_jobs[stage].append(node)
        else:
            ungrouped.append(node)

    # One cluster per stage
    for _, stage in enumerate(stages):
        jobs = stage_jobs[stage]
        if not jobs:
            continue
        stage_type = _STAGE_TYPE_MAP.get(stage, "other")
        cluster_color = _COLORS.get(stage_type, _COLORS["other"])

        with dot.subgraph(name=f"cluster_{stage}") as c:
            c.attr(
                label=f"{stage}\n({len(jobs)} jobs)",
                style="filled",
                fillcolor=cluster_color,
                color="#666666",
                fontsize="10",
            )
            for job_name in jobs:
                node_attrs = G.nodes[job_name]
                border = _border_color(node_attrs)
                c.node(
                    _node_id(job_name),
                    label=_node_label(job_name, node_attrs),
                    fillcolor=cluster_color,
                    color=border,
                )

    # Add inter-stage edges (deduplicated by stage pair)
    stage_edges: set[tuple[str, str]] = set()
    for src, dst in G.edges():
        src_stage = G.nodes[src].get("stage", "")
        dst_stage = G.nodes[dst].get("stage", "")
        if src_stage != dst_stage and src_stage in stage_jobs and dst_stage in stage_jobs:
            stage_edges.add((src_stage, dst_stage))

    # Use representative nodes as edge endpoints
    stage_rep: dict[str, str] = {}
    for stage, jobs in stage_jobs.items():
        if jobs:
            stage_rep[stage] = jobs[0]

    for src_stage, dst_stage in sorted(stage_edges):
        if src_stage in stage_rep and dst_stage in stage_rep:
            dot.edge(_node_id(stage_rep[src_stage]), _node_id(stage_rep[dst_stage]), style="dashed", color="#999999")

    output_file = str(output_path.with_suffix(""))
    dot.render(output_file, cleanup=True)
    return output_path


# ---------------------------------------------------------------------------
# Job view
# ---------------------------------------------------------------------------


def render_jobs(
    G: nx.DiGraph,
    stages: list[str],
    output_path: str | Path,
    fmt: str = "svg",
    max_nodes: int = 500,
) -> Path:
    """
    Render all jobs as nodes, grouped by stage cluster, with needs: edges.
    """
    if not HAS_GRAPHVIZ:
        raise ImportError("graphviz package is required: pip install graphviz")

    output_path = Path(output_path)
    dot = graphviz.Digraph(
        name="pipeline_jobs",
        comment="Datadog Agent CI Pipeline — Job View",
        format=fmt,
    )
    dot.attr(rankdir="LR", fontsize="9", fontname="Helvetica", splines="true")
    dot.attr("node", fontname="Helvetica", fontsize="8", shape="box", style="filled,rounded")
    dot.attr("edge", fontname="Helvetica", fontsize="7", arrowsize="0.5")

    # If graph is too large, warn
    if G.number_of_nodes() > max_nodes:
        print(f"Warning: {G.number_of_nodes()} nodes — large graph may render slowly")

    stage_jobs: dict[str, list[str]] = {s: [] for s in stages}
    for node, attrs in G.nodes(data=True):
        stage = attrs.get("stage", "")
        if stage in stage_jobs:
            stage_jobs[stage].append(node)
        else:
            stage_jobs.setdefault(stage, []).append(node)

    for stage in stages:
        jobs = stage_jobs.get(stage, [])
        if not jobs:
            continue
        stage_type = _STAGE_TYPE_MAP.get(stage, "other")
        cluster_color = _COLORS.get(stage_type, _COLORS["other"])

        with dot.subgraph(name=f"cluster_{stage}") as c:
            c.attr(label=stage, style="filled", fillcolor=cluster_color, color="#888888")
            for job_name in jobs:
                node_attrs = G.nodes[job_name]
                fill = _job_color(node_attrs)
                border = _border_color(node_attrs)
                tooltip = f"stage: {node_attrs.get('stage', '')}\nplatform: {node_attrs.get('platform', '')}"
                c.node(
                    _node_id(job_name),
                    label=_node_label(job_name, node_attrs),
                    fillcolor=fill,
                    color=border,
                    tooltip=tooltip,
                )

    for src, dst in G.edges():
        dot.edge(_node_id(src), _node_id(dst), arrowsize="0.4")

    output_file = str(output_path.with_suffix(""))
    dot.render(output_file, cleanup=True)
    return output_path


# ---------------------------------------------------------------------------
# Single-job subgraph view
# ---------------------------------------------------------------------------


def render_job_subgraph(
    subG: nx.DiGraph,
    job_name: str,
    stages: list[str],
    output_path: str | Path,
    fmt: str = "svg",
) -> Path:
    """Render the transitive dependency subgraph for a single job."""
    if not HAS_GRAPHVIZ:
        raise ImportError("graphviz package is required: pip install graphviz")

    output_path = Path(output_path)
    dot = graphviz.Digraph(
        name=f"job_{job_name}",
        comment=f"Dependencies of {job_name}",
        format=fmt,
    )
    dot.attr(rankdir="LR", fontsize="9", fontname="Helvetica")
    dot.attr("node", fontname="Helvetica", fontsize="8", shape="box", style="filled,rounded")

    for node, attrs in subG.nodes(data=True):
        fill = _job_color(attrs)
        border = _border_color(attrs)
        label = _node_label(node, attrs)
        if node == job_name:
            dot.node(_node_id(node), label=label, fillcolor="#ff9999", color=border, penwidth="2")
        else:
            dot.node(_node_id(node), label=label, fillcolor=fill, color=border)

    for src, dst in subG.edges():
        dot.edge(_node_id(src), _node_id(dst), arrowsize="0.5")

    output_file = str(output_path.with_suffix(""))
    dot.render(output_file, cleanup=True)
    return output_path


# ---------------------------------------------------------------------------
# Artifact view
# ---------------------------------------------------------------------------


def render_artifacts(
    G: nx.DiGraph,
    output_path: str | Path,
    fmt: str = "svg",
) -> Path:
    """
    Render jobs + S3 artifact nodes.
    job → s3_uri → consuming_job
    """
    if not HAS_GRAPHVIZ:
        raise ImportError("graphviz package is required: pip install graphviz")

    output_path = Path(output_path)
    dot = graphviz.Digraph(
        name="pipeline_artifacts",
        comment="Datadog Agent CI Pipeline — Artifact View",
        format=fmt,
    )
    dot.attr(rankdir="LR", fontsize="9", fontname="Helvetica")
    dot.attr("node", fontname="Helvetica", fontsize="8", shape="box", style="filled,rounded")

    # Build artifact adjacency
    artifact_producers: dict[str, list[str]] = {}  # uri → [job]
    artifact_consumers: dict[str, list[str]] = {}  # uri → [job]

    for node, attrs in G.nodes(data=True):
        for uri in attrs.get("produces_s3", []):
            artifact_producers.setdefault(uri, []).append(node)
        for uri in attrs.get("consumes_s3", []):
            artifact_consumers.setdefault(uri, []).append(node)

    # Add job nodes
    for node, attrs in G.nodes(data=True):
        fill = _job_color(attrs)
        border = _border_color(attrs)
        dot.node(_node_id(node), label=_short_name(node), fillcolor=fill, color=border)

    # Add artifact nodes + edges
    all_uris = set(artifact_producers) | set(artifact_consumers)
    for uri in sorted(all_uris):
        safe_id = re.sub(r"[^a-zA-Z0-9_]", "_", uri)[:60]
        label = _short_uri(uri)
        dot.node(safe_id, label=label, shape="cylinder", fillcolor="#ffffcc", color="#888800")
        for producer in artifact_producers.get(uri, []):
            dot.edge(_node_id(producer), safe_id, color="#0000aa")
        for consumer in artifact_consumers.get(uri, []):
            dot.edge(safe_id, _node_id(consumer), color="#aa0000")

    output_file = str(output_path.with_suffix(""))
    dot.render(output_file, cleanup=True)
    return output_path


# ---------------------------------------------------------------------------
# Helpers
# ---------------------------------------------------------------------------


def _node_label(node_name: str, attrs: dict, max_len: int = 35) -> str:
    """Return the display label for a node.

    Folded nodes carry a ``label`` attribute (e.g. ``stem\\n[arm64, x64]``).
    Plain nodes fall back to a shortened version of the node name.
    The stem portion is truncated to *max_len* chars; the variant line is kept.
    """
    raw = attrs.get("label") or node_name
    if "\n" in raw:
        stem, rest = raw.split("\n", 1)
        return f"{_short_name(stem, max_len)}\n{rest}"
    return _short_name(raw, max_len)


def _node_id(name: str) -> str:
    """Return a DOT-safe node identifier for a job name.

    DOT identifiers must not contain colons (port separator), hyphens at
    unexpected positions, or other special characters unless quoted.  The
    graphviz Python library does not reliably quote all such names, so we
    sanitize explicitly and keep the original name only in the label.
    """
    return re.sub(r"[^a-zA-Z0-9_]", "_", name)


def _short_name(name: str, max_len: int = 30) -> str:
    """Shorten job name for display."""
    if len(name) <= max_len:
        return name
    return name[: max_len - 2] + ".."


def _short_uri(uri: str, max_len: int = 40) -> str:
    """Shorten S3 URI for display."""
    # Show last two path components
    parts = uri.rstrip("/").split("/")
    short = "/".join(parts[-2:]) if len(parts) >= 2 else uri
    if len(short) > max_len:
        short = short[: max_len - 2] + ".."
    return short
