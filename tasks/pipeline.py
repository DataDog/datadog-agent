from __future__ import print_function

from invoke import task
from invoke.exceptions import Exit

from .deploy.gitlab import Gitlab
from .deploy.pipeline_tools import trigger_agent_pipeline, wait_for_pipeline


@task
def trigger(ctx, git_ref="master", release_version_6="nightly", release_version_7="nightly-a7", repo_branch="nightly"):
    """
    Trigger a deploy pipeline on the given git ref.
    The --release-version-6 and --release-version-7 options indicate which release.json entries are used.
    To not build Agent 6, set --release-version-6 "". To not build Agent 7, set --release-version-7 "".
    The --repo-branch option indicates which branch of the staging repository the packages will be deployed to.

    Example:
    inv pipeline.trigger --git-ref 7.22.0 --release-version-6 "6.22.0" --release-version-7 "7.22.0" --repo-branch "stable"
    """
    pipeline_id = trigger_agent_pipeline(git_ref, release_version_6, release_version_7, repo_branch, deploy=True)
    wait_for_pipeline("DataDog/datadog-agent", pipeline_id)


@task
def run_all_tests(ctx, git_ref="master", here=False, release_version_6="nightly", release_version_7="nightly-a7"):
    """
    Trigger a pipeline on the given git ref, or on the current branch if --here is given.
    This pipeline will run all tests, including kitchen tests.
    The packages built won't be deployed to the staging repository. Use invoke pipeline.trigger if you want to
    deploy them.
    The --release-version-6 and --release-version-7 options indicate which release.json entries are used.
    To not build Agent 6, set --release-version-6 "". To not build Agent 7, set --release-version-7 "".

    Examples:
    inv pipeline.run-all-tests --git-ref my-branch
    inv pipeline.run-all-tests --here
    """
    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    pipeline_id = trigger_agent_pipeline(git_ref, release_version_6, release_version_7, "none", deploy=False)
    wait_for_pipeline("DataDog/datadog-agent", pipeline_id)


@task
def follow(ctx, id=None, git_ref=None, here=False):
    """
    Follow a pipeline's progress in the CLI.
    Use --here to follow the latest pipeline on your current branch.
    Use --git-ref to follow the latest pipeline on a given tag or branch.
    Use --id to follow a specific pipeline.

    Examples:
    inv pipeline.follow --git-ref my-branch
    inv pipeline.follow --here
    inv pipeline.follow --id 1234567
    """
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
