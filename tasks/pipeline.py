from __future__ import print_function

import re

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

    #
    # Create gitlab instance and make sure we have access.
    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    #
    # If git_ref matches v7 pattern and release_version_6 is not empty, make sure Gitlab has v6 tag.
    # If git_ref matches v6 pattern and release_version_7 is not empty, make sure Gitlab has v7 tag.
    # v7 version pattern should be able to match 7.12.24-rc2 and 7.12.34
    #
    v7_pattern = r'^7\.(\d+\.\d+)(-.+|)$'
    v6_pattern = r'^6\.(\d+\.\d+)(-.+|)$'

    match = re.match(v7_pattern, git_ref)

    if release_version_6 and match:
        # release_version_6 is not empty and git_ref matches v7 pattern, construct v6 tag and check.
        tag_name = "6." + "".join(match.groups())
        gitlab_tag = gitlab.find_tag(project_name, tag_name)

        if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
            print("Cannot find GitLab v6 tag {} while trying to build git ref {}".format(tag_name, git_ref))
            raise Exit(code=1)

        print("Successfully cross checked v6 tag {} and git ref {}".format(tag_name, git_ref))
    else:
        match = re.match(v6_pattern, git_ref)

        if release_version_7 and match:
            # release_version_7 is not empty and git_ref matches v6 pattern, construct v7 tag and check.
            tag_name = "7." + "".join(match.groups())
            gitlab_tag = gitlab.find_tag(project_name, tag_name)

            if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
                print("Cannot find GitLab v7 tag {} while trying to build git ref {}".format(tag_name, git_ref))
                raise Exit(code=1)

            print("Successfully cross checked v7 tag {} and git ref {}".format(tag_name, git_ref))

    pipeline_id = trigger_agent_pipeline(git_ref, release_version_6, release_version_7, repo_branch, deploy=True)
    wait_for_pipeline(project_name, pipeline_id)


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
