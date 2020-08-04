from __future__ import print_function

from invoke import task
from invoke.exceptions import Exit

from .deploy.gitlab import Gitlab
from .deploy.pipeline_tools import trigger_agent_pipeline, wait_for_pipeline


@task
def trigger(ctx, git_ref="master", release_version_6="nightly", release_version_7="nightly-a7", repo_branch="nightly"):
    pipeline_id = trigger_agent_pipeline(git_ref, release_version_6, release_version_7, repo_branch)
    wait_for_pipeline("DataDog/datadog-agent", pipeline_id)


@task
def follow(ctx, id=None, git_ref=None, here=False):
    if id is not None:
        wait_for_pipeline("DataDog/datadog-agent", id)
    elif git_ref is not None:
        wait_for_pipeline_from_ref(git_ref)
    elif here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
        wait_for_pipeline_from_ref(git_ref)


def wait_for_pipeline_from_ref(ref):
    pipeline = Gitlab().last_pipeline_for_ref("DataDog/datadog-agent", ref)
    if pipeline is not None:
        wait_for_pipeline("DataDog/datadog-agent", pipeline['id'])
    else:
        print("No pipelines found for {ref}".format(ref=ref))
        raise Exit(code=1)
