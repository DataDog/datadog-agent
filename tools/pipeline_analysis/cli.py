"""
CLI entry point for pipeline_analysis.

Usage:
    python -m pipeline_analysis <command> [options]

Commands:
    graph   -- render pipeline graph to SVG/PNG/PDF
    jobs    -- list jobs (optionally filtered by stage)
    inputs  -- show what a job depends on
    stats   -- print graph statistics
"""

from __future__ import annotations

from pathlib import Path

import click

from pipeline_analysis.graph.pipeline_graph import PipelineGraph
from pipeline_analysis.viz.dot_viz import render_artifacts, render_job_subgraph, render_jobs, render_stages


def _get_repo_root(repo_root: str | None) -> Path:
    import os

    if repo_root:
        return Path(repo_root)
    # bazel run sets BUILD_WORKSPACE_DIRECTORY to the workspace root
    if ws := os.environ.get("BUILD_WORKSPACE_DIRECTORY"):
        return Path(ws)
    # Walk up from this file to find .gitlab-ci.yml
    here = Path(__file__).parent
    for parent in [here, here.parent, here.parent.parent]:
        if (parent / ".gitlab-ci.yml").exists():
            return parent
    raise click.ClickException("Could not find .gitlab-ci.yml. Pass --repo-root explicitly.")


@click.group()
@click.option(
    "--repo-root",
    default=None,
    envvar="REPO_ROOT",
    help="Path to the repository root (auto-detected if not set).",
)
@click.pass_context
def cli(ctx: click.Context, repo_root: str | None) -> None:
    """Pipeline analysis tools for the Datadog Agent GitLab CI pipeline."""
    ctx.ensure_object(dict)
    ctx.obj["repo_root"] = _get_repo_root(repo_root)


# ---------------------------------------------------------------------------
# graph command
# ---------------------------------------------------------------------------


@cli.command()
@click.option(
    "--mode",
    type=click.Choice(["stages", "jobs", "job", "artifacts"]),
    default="stages",
    show_default=True,
    help="Rendering mode.",
)
@click.option(
    "--job",
    default=None,
    help="Job name (required for --mode job).",
)
@click.option(
    "--output",
    default="pipeline.svg",
    show_default=True,
    help="Output file path.",
)
@click.option(
    "--format",
    "fmt",
    type=click.Choice(["svg", "png", "pdf"]),
    default="svg",
    show_default=True,
    help="Output format.",
)
@click.option(
    "--fold/--no-fold",
    default=True,
    show_default=True,
    help="Collapse similar jobs (arch/fips/OS/format variants) into single nodes.",
)
@click.pass_context
def graph(ctx: click.Context, mode: str, job: str | None, output: str, fmt: str, fold: bool) -> None:
    """Render the pipeline graph to a file."""
    repo_root = ctx.obj["repo_root"]
    click.echo(f"Loading pipeline from {repo_root} ...")

    pg = PipelineGraph(repo_root)
    pg.build()

    if fold and mode in ("stages", "jobs"):
        G = pg.build_folded()
        click.echo(
            f"Loaded {pg.G.number_of_nodes()} jobs → folded to {G.number_of_nodes()} nodes across {len(pg.stages)} stages."
        )
    else:
        G = pg.G
        click.echo(f"Loaded {G.number_of_nodes()} jobs across {len(pg.stages)} stages.")

    output_path = Path(output)

    if mode == "stages":
        out = render_stages(G, pg.stages, output_path, fmt=fmt)
        click.echo(f"Rendered stage view → {out}")

    elif mode == "jobs":
        out = render_jobs(G, pg.stages, output_path, fmt=fmt)
        click.echo(f"Rendered job view → {out}")

    elif mode == "job":
        if not job:
            raise click.UsageError("--job is required when --mode job")
        if job not in G:
            raise click.UsageError(f"Job '{job}' not found in pipeline.")
        subG = pg.subgraph_for_job(job)
        out = render_job_subgraph(subG, job, pg.stages, output_path, fmt=fmt)
        click.echo(f"Rendered subgraph for '{job}' ({subG.number_of_nodes()} nodes) → {out}")

    elif mode == "artifacts":
        out = render_artifacts(G, output_path, fmt=fmt)
        click.echo(f"Rendered artifact view → {out}")


# ---------------------------------------------------------------------------
# jobs command
# ---------------------------------------------------------------------------


@cli.command("jobs")
@click.option("--stage", default=None, help="Filter to this stage.")
@click.option("--platform", default=None, help="Filter to platform (linux/windows/mac/arm).")
@click.option("--type", "job_type", default=None, help="Filter to job type (build/test/deploy/trigger).")
@click.pass_context
def list_jobs(ctx: click.Context, stage: str | None, platform: str | None, job_type: str | None) -> None:
    """List jobs in the pipeline."""
    repo_root = ctx.obj["repo_root"]
    pg = PipelineGraph(repo_root)
    pg.build()

    jobs = pg.jobs
    if stage:
        jobs = [j for j in jobs if j.stage == stage]
    if platform:
        jobs = [j for j in jobs if j.platform == platform]
    if job_type:
        jobs = [j for j in jobs if j.job_type == job_type]

    # Group by stage
    from collections import defaultdict

    by_stage: dict[str, list] = defaultdict(list)
    for j in jobs:
        by_stage[j.stage].append(j)

    total = 0
    for s in pg.stages:
        if s not in by_stage:
            continue
        click.echo(f"\n[{s}]")
        for j in sorted(by_stage[s], key=lambda x: x.name):
            tags = f"  [{j.platform}]" if j.platform != "linux" else ""
            click.echo(f"  {j.name}{tags}")
            total += 1

    click.echo(f"\n{total} job(s) shown.")


# ---------------------------------------------------------------------------
# inputs command
# ---------------------------------------------------------------------------


@cli.command()
@click.argument("job_name")
@click.option("--transitive", is_flag=True, default=False, help="Show all transitive ancestors.")
@click.pass_context
def inputs(ctx: click.Context, job_name: str, transitive: bool) -> None:
    """Show what JOB_NAME depends on."""
    repo_root = ctx.obj["repo_root"]
    pg = PipelineGraph(repo_root)
    pg.build()

    if job_name not in pg.G:
        raise click.UsageError(f"Job '{job_name}' not found.")

    if transitive:
        deps = pg.transitive_needs(job_name)
        click.echo(f"Transitive inputs for '{job_name}' ({len(deps)} jobs):")
    else:
        deps = pg.direct_needs(job_name)
        click.echo(f"Direct inputs for '{job_name}' ({len(deps)} jobs):")

    for dep in deps:
        j = pg.job(dep)
        stage = j.stage if j else "?"
        click.echo(f"  {dep}  (stage: {stage})")


# ---------------------------------------------------------------------------
# stats command
# ---------------------------------------------------------------------------


@cli.command()
@click.pass_context
def stats(ctx: click.Context) -> None:
    """Print summary statistics about the pipeline graph."""
    repo_root = ctx.obj["repo_root"]
    click.echo(f"Loading pipeline from {repo_root} ...")
    pg = PipelineGraph(repo_root)
    pg.build()
    s = pg.stats()

    click.echo("\nPipeline Statistics")
    click.echo("===================")
    click.echo(f"Total jobs:   {s['total_jobs']}")
    click.echo(f"Total edges:  {s['total_edges']}")
    click.echo(f"Stages:       {s['stages']}")
    click.echo(f"Is DAG:       {s['is_dag']}")

    click.echo("\nJobs per platform:")
    for platform, count in sorted(s["platform_counts"].items()):
        click.echo(f"  {platform:<12} {count}")

    click.echo("\nJobs per type:")
    for jtype, count in sorted(s["type_counts"].items()):
        click.echo(f"  {jtype:<12} {count}")

    click.echo("\nJobs per stage (non-empty):")
    stage_counts = s["stage_counts"]
    for stage in pg.stages:
        count = stage_counts.get(stage, 0)
        if count:
            click.echo(f"  {stage:<45} {count:>4}")


# ---------------------------------------------------------------------------
# Entry point
# ---------------------------------------------------------------------------

if __name__ == "__main__":
    cli()
