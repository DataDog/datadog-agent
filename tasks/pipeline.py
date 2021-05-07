import io
import os
import re
import traceback
from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from .libs.common.gitlab import Gitlab
from .libs.pipeline_notifications import (
    base_message,
    find_job_owners,
    get_failed_jobs,
    get_failed_tests,
    send_slack_message,
)
from .libs.pipeline_tools import trigger_agent_pipeline, wait_for_pipeline
from .libs.types import SlackMessage, TeamMessage

# Tasks to trigger pipelines

ALLOWED_REPO_BRANCHES = {"stable", "beta", "nightly", "none"}


@task
def trigger(_, git_ref="master", release_version_6="nightly", release_version_7="nightly-a7", repo_branch="nightly"):
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

    # Check that the target repo branch is valid
    if repo_branch not in ALLOWED_REPO_BRANCHES:
        print(
            "--repo-branch argument '{}' is not in the list of allowed repository branches: {}".format(
                repo_branch, ALLOWED_REPO_BRANCHES
            )
        )
        raise Exit(code=1)

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

    # Always run all builds and kitchen tests on a deploy pipeline
    pipeline_id = trigger_agent_pipeline(
        gitlab,
        project_name,
        git_ref,
        release_version_6,
        release_version_7,
        repo_branch,
        deploy=True,
        all_builds=True,
        kitchen_tests=True,
    )
    wait_for_pipeline(gitlab, project_name, pipeline_id)


@task
def run(
    ctx,
    git_ref="master",
    here=False,
    release_version_6="nightly",
    release_version_7="nightly-a7",
    all_builds=True,
    kitchen_tests=True,
):
    """
    Trigger a pipeline on the given git ref, or on the current branch if --here is given.
    By default, this pipeline will run all builds & tests, including all kitchen tests.
    Use --no-all-builds to not run builds for all architectures (only a subset of jobs will run. No effect on master pipelines).
    Use --no-kitchen-tests to not run all kitchen tests on the pipeline.
    The packages built won't be deployed to the staging repository. Use invoke pipeline.trigger if you want to
    deploy them.
    The --release-version-6 and --release-version-7 options indicate which release.json entries are used.
    To not build Agent 6, set --release-version-6 "". To not build Agent 7, set --release-version-7 "".

    Examples:
    inv pipeline.run --git-ref my-branch
    inv pipeline.run --here
    inv pipeline.run --here --no-kitchen-tests
    """

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    pipeline_id = trigger_agent_pipeline(
        gitlab,
        project_name,
        git_ref,
        release_version_6,
        release_version_7,
        "none",
        all_builds=all_builds,
        kitchen_tests=kitchen_tests,
    )
    wait_for_pipeline(gitlab, project_name, pipeline_id)


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

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    if id is not None:
        wait_for_pipeline(gitlab, project_name, id)
    elif git_ref is not None:
        wait_for_pipeline_from_ref(gitlab, project_name, git_ref)
    elif here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
        wait_for_pipeline_from_ref(gitlab, project_name, git_ref)


def wait_for_pipeline_from_ref(gitlab, project_name, ref):
    pipeline = Gitlab().last_pipeline_for_ref(project_name, ref)
    if pipeline is not None:
        wait_for_pipeline(gitlab, project_name, pipeline['id'])
    else:
        print("No pipelines found for {ref}".format(ref=ref))
        raise Exit(code=1)


# Tasks to trigger pipeline notifications

GITHUB_SLACK_MAP = {
    "@DataDog/agent-platform": "#agent-platform",
    "@DataDog/container-integrations": "#container-integration",
    "@DataDog/integrations-tools-and-libraries": "#intg-tools-libs",
    "@DataDog/agent-network": "#network-agent",
    "@DataDog/agent-security": "#security-and-compliance-agent",
    "@DataDog/agent-apm": "#apm-agent",
    "@DataDog/infrastructure-integrations": "#infrastructure-integrations",
    "@DataDog/processes": "#processes",
    "@DataDog/agent-core": "#agent-core",
    "@DataDog/container-app": "#container-app",
    "@Datadog/metrics-aggregation": "#metrics-aggregation",
    "@DataDog/agent-all": "#datadog-agent-pipelines",
}

UNKNOWN_OWNER_TEMPLATE = """The owner `{owner}` is not mapped to any slack channel.
Please check for typos in the JOBOWNERS file and/or add them to the Github <-> Slack map.
"""


def generate_failure_messages(base):
    project_name = "DataDog/datadog-agent"
    all_teams = "@DataDog/agent-all"
    failed_jobs = get_failed_jobs(project_name, os.getenv("CI_PIPELINE_ID"))
    # Generate messages for each team
    messages_to_send = defaultdict(lambda: TeamMessage(base))
    messages_to_send[all_teams] = SlackMessage(base, jobs=failed_jobs)

    failed_job_owners = find_job_owners(failed_jobs)
    for owner, jobs in failed_job_owners.items():
        if owner == "@DataDog/multiple":
            for job in jobs:
                for test in get_failed_tests(project_name, job):
                    messages_to_send[all_teams].add_test_failure(test, job)
                    for owner in test.owners:
                        messages_to_send[owner].add_test_failure(test, job)
        elif owner == "@DataDog/do-not-notify":
            # Jobs owned by @DataDog/do-not-notify do not send team messages
            pass
        elif owner == all_teams:
            # Jobs owned by @DataDog/agent-all will already be in the global
            # message, do not overwrite the failed jobs list
            pass
        else:
            pass
            # TODO: enable also jobs
            # messages_to_send[owner].failed_jobs = jobs

    return messages_to_send


@task
def notify_failure(_, notification_type="merge", print_to_stdout=False):
    """
    Send failure notifications for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    real slack messages.
    """

    header = ""
    if notification_type == "merge":
        header = ":host-red: :merged: datadog-agent merge"
    elif notification_type == "deploy":
        header = ":host-red: :rocket: datadog-agent deploy"
    base = base_message(header)

    try:
        messages_to_send = generate_failure_messages(base)
    except Exception as e:
        buffer = io.StringIO()
        print(base, file=buffer)
        print("Found exception when generating notification:", file=buffer)
        traceback.print_exc(limit=-1, file=buffer)
        print("See the job log for the full exception traceback.", file=buffer)
        messages_to_send = {
            "@DataDog/agent-all": SlackMessage(buffer.getvalue()),
        }
        # Print traceback on job log
        print(e)
        traceback.print_exc()

    # Send messages
    for owner, message in messages_to_send.items():
        channel = GITHUB_SLACK_MAP.get(owner, "#datadog-agent-pipelines")
        if owner not in GITHUB_SLACK_MAP.keys():
            message.base_message += UNKNOWN_OWNER_TEMPLATE.format(owner=owner)
        message.coda = "If there is something wrong with the notification please contact #agent-platform"
        if print_to_stdout:
            print("Would send to {channel}:\n{message}".format(channel=channel, message=str(message)))
        else:
            send_slack_message(channel, str(message))  # TODO: use channel variable
