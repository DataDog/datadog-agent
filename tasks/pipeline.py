from __future__ import print_function


from invoke import task

from .deploy.trigger_agent_pipeline import trigger_agent_pipeline, wait_for_pipeline


@task
def trigger_pipeline(
    ctx,
    ref="master",
    release_version_6="nightly",
    release_version_7="nightly-a7",
    repo_branch="nightly",
    windows_update_latest=True,
):
    pipeline_id = trigger_agent_pipeline(ref, release_version_6, release_version_7, repo_branch, windows_update_latest)
    wait_for_pipeline(pipeline_id)
