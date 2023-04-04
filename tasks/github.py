import os

from invoke import task
from invoke.exceptions import Exit

from .libs.github_actions_tools import download_artifacts, follow_workflow_run, trigger_macos_workflow, print_workflow_conclusion
from .utils import DEFAULT_BRANCH, load_release_versions


@task
def trigger_macos_build(
    ctx,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
    destination=".",
):

    env = load_release_versions(ctx, release_version)
    github_action_ref = env["MACOS_BUILD_VERSION"]

    run_id = trigger_macos_workflow(
        workflow="macos.yaml",
        github_action_ref=github_action_ref,
        datadog_agent_ref=datadog_agent_ref,
        release_version=release_version,
        major_version=major_version,
        python_runtimes=python_runtimes,
        # Send pipeline id and bucket branch so that the package version
        # can be constructed properly for nightlies.
        gitlab_pipeline_id=os.environ.get("CI_PIPELINE_ID", None),
        bucket_branch=os.environ.get("BUCKET_BRANCH", None),
    )

    workflow_conclusion = follow_workflow_run(run_id)

    print_workflow_conclusion(workflow_conclusion)

    download_artifacts(run_id, destination)

    if workflow_conclusion != "success":
        raise Exit(code=1)


@task
def trigger_macos_test(
    ctx,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    python_runtimes="3",
    destination=".",
):

    env = load_release_versions(ctx, release_version)
    github_action_ref = env["MACOS_BUILD_VERSION"]

    run_id = trigger_macos_workflow(
        workflow="test.yaml",
        github_action_ref=github_action_ref,
        datadog_agent_ref=datadog_agent_ref,
        python_runtimes=python_runtimes,
    )

    workflow_conclusion = follow_workflow_run(run_id)

    print_workflow_conclusion(workflow_conclusion)

    download_artifacts(run_id, destination)

    if workflow_conclusion != "success":
        raise Exit(code=1)
