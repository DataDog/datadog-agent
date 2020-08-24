from __future__ import print_function

from invoke import task

from .deploy.github_actions_tools import (
    download_artifacts,
    follow_workflow_run,
    get_macos_workflow_run_id_for_ref,
    trigger_macos_workflow,
)


@task
def trigger_macos_build(
    ctx,
    git_ref="master",
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
    buildimages_version="master",
    destination=".",
):
    run_id = trigger_macos_workflow(git_ref, release_version, major_version, python_runtimes, buildimages_version)

    follow_workflow_run(run_id)

    download_artifacts(run_id, destination)


@task
def follow_macos_build(ctx, run_id=None, git_ref=None):
    if run_id is not None:
        pass
    elif git_ref is not None:
        run_id = get_macos_workflow_run_id_for_ref(git_ref)

    follow_workflow_run(run_id)


@task
def download_macos_artifacts(ctx, run_id=None, git_ref=None, destination="."):
    if run_id is not None:
        pass
    elif git_ref is not None:
        run_id = get_macos_workflow_run_id_for_ref(git_ref)

    download_artifacts(run_id, destination)
