import json

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.git import get_current_branch
from tasks.libs.testing.utof.pipeline import aggregate_results, fetch_pipeline_utof_results
from tasks.libs.testing.utof.pipeline_report import format_pipeline_report


@task
def pipeline_report(ctx, id=None, git_ref=None, here=False, project_name="DataDog/datadog-agent", json_output=None):
    """
    Aggregate UTOF (Unified Test Output Format) results across every
    UTOF-emitting job of a pipeline, and print a consolidated failure report.

    Use --here to report on the latest pipeline on your current branch.
    Use --git-ref to report on the latest pipeline on a given tag or branch.
    Use --id to report on a specific pipeline.
    Use --project-name to specify a repo other than DataDog/datadog-agent (default)
    Use --json-output <path> to also write a machine-readable JSON export.

    Examples:
    dda inv utof.pipeline-report --id 1234567
    dda inv utof.pipeline-report --git-ref my-branch
    dda inv utof.pipeline-report --here
    dda inv utof.pipeline-report --here --json-output pipeline_report.json
    """

    repo = get_gitlab_repo(project_name)

    args_given = 0
    if id is not None:
        args_given += 1
    if git_ref is not None:
        args_given += 1
    if here:
        args_given += 1
    if args_given != 1:
        raise Exit(
            "ERROR: Exactly one of --here, --git-ref or --id must be given.\nSee --help for an explanation of each.",
            code=1,
        )

    if id is not None:
        pipeline = repo.pipelines.get(id)
    else:
        ref = git_ref if git_ref is not None else get_current_branch(ctx)
        pipelines = repo.pipelines.list(ref=ref, per_page=1, order_by='updated_at')
        if not pipelines:
            raise Exit(f"No pipelines found for {ref}", code=1)
        pipeline = pipelines[0]

    jobs = fetch_pipeline_utof_results(repo, pipeline)
    agg = aggregate_results(str(pipeline.id), pipeline.web_url, jobs)

    print(format_pipeline_report(agg))

    if json_output:
        with open(json_output, "w") as f:
            json.dump(agg.to_dict(), f, indent=2)
        print(f"\nJSON export written to {json_output}")
