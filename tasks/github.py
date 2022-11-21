import os

from invoke import task

from .libs.github_actions_tools import download_artifacts, follow_workflow_run, trigger_macos_workflow
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

    follow_workflow_run(run_id)

    download_artifacts(run_id, destination)


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

    follow_workflow_run(run_id)

    download_artifacts(run_id, destination)
