from __future__ import print_function


from invoke import task

from .deploy.trigger_agent_pipeline import trigger_agent_pipeline, wait_for_pipeline
from .deploy.gitlab import Gitlab


@task
def trigger(
    ctx,
    git_ref="master",
    release_version_6="nightly",
    release_version_7="nightly-a7",
    repo_branch="nightly",
    windows_update_latest=True,
):
    pipeline_id = trigger_agent_pipeline(
        git_ref, release_version_6, release_version_7, repo_branch, windows_update_latest
    )
    wait_for_pipeline("DataDog/datadog-agent", pipeline_id)


@task
def follow(ctx, id=None, git_ref=None, here=False):
    if id is not None:
        wait_for_pipeline("DataDog/datadog-agent", id)
    elif git_ref is not None:
        pipeline = Gitlab().last_pipeline_for_ref("DataDog/datadog-agent", git_ref)
        if pipeline is not None:
            wait_for_pipeline("DataDog/datadog-agent", pipeline['id'])
        else:
            print("No pipelines found for {ref}".format(ref=git_ref))
    elif here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
        pipeline = Gitlab().last_pipeline_for_ref("DataDog/datadog-agent", git_ref)
        if pipeline is not None:
            wait_for_pipeline("DataDog/datadog-agent", pipeline['id'])
        else:
            print("No pipelines found for {ref}".format(ref=git_ref))
