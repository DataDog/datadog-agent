import os
import re
from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from .libs.common.gitlab import Gitlab
from .libs.pipeline_notifications import (
    base_message,
    find_job_owners,
    get_failed_jobs,
    get_failed_tests,
    prepare_global_failure_message,
    prepare_team_failure_message,
    prepare_test_failure_section,
    send_slack_message,
)
from .libs.pipeline_tools import trigger_agent_pipeline, wait_for_pipeline

# Tasks to trigger pipelines


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

    pipeline_id = trigger_agent_pipeline(
        gitlab, project_name, git_ref, release_version_6, release_version_7, repo_branch, deploy=True
    )
    wait_for_pipeline(gitlab, project_name, pipeline_id)


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

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
    pipeline_id = trigger_agent_pipeline(
        gitlab, project_name, git_ref, release_version_6, release_version_7, "none", deploy=False
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
    "@DataDog/networks": "#networks",
    "@DataDog/agent-security": "#security-and-compliance-agent",
    "@DataDog/agent-apm": "#apm-agent",
    "@DataDog/infrastructure-integrations": "#infrastructure-integrations",
    "@DataDog/processes": "#processes",
    "@DataDog/agent-core": "#agent-core",
    "@DataDog/container-app": "#container-app",
    "@Datadog/metrics-aggregation": "#metrics-aggregation",
}


@task
def notify_failure(_, notification_type="merge"):
    """
    Send failure notifications for the current pipeline. CI-only task.
    """
    header = ""
    if notification_type == "merge":
        header = ":host-red: :merged: datadog-agent merge"
    elif notification_type == "deploy":
        header = ":host-red: :rocket: datadog-agent deploy"

    project_name = "DataDog/datadog-agent"
    failed_jobs = get_failed_jobs(project_name, os.getenv("CI_PIPELINE_ID"))

    # Take care of the global message that goes in the common channel
    message = prepare_global_failure_message(header, failed_jobs)
    send_slack_message("#agent-pipeline-notifications", message)

    # Take care of messages for each team
    messages_to_send = {}
    failed_job_owners = find_job_owners(failed_jobs)
    for owner, jobs in failed_job_owners.items():
        # Check if owner is defined
        if owner in GITHUB_SLACK_MAP.keys():
            message = prepare_team_failure_message(header, jobs)
            messages_to_send[GITHUB_SLACK_MAP[owner]] = message
        elif owner == "@DataDog/multiple":
            # Create map from owners to map from tests to jobs
            owners_to_failed_tests = defaultdict(lambda: defaultdict(list))
            for job in jobs:
                for test in get_failed_tests(project_name, job):
                    for owner in test.owners:
                        owners_to_failed_tests[owner][test].append(job)

            # Concat owners existing message with failed unit test message
            for owner, failed_tests in owners_to_failed_tests.items():
                slack_owner = GITHUB_SLACK_MAP[owner]
                if slack_owner not in messages_to_send:
                    messages_to_send[slack_owner] = base_message(header)

                messages_to_send[slack_owner] += prepare_test_failure_section(failed_tests)
        elif owner == "@DataDog/do-not-notify":
            # Jobs owned by @DataDog/do-not-notify do not send team messages
            pass
        else:
            message = """The owner `{owner}` is not mapped to any slack channel. Please check for typos
in the JOBOWNERS file and/or add them to the Github <-> Slack map.
Jobs they own:""".format(
                owner=owner
            )
            for job in jobs:
                message += "\n - <{url}|{name}> (stage: {stage}, after {retries} retries)".format(
                    url=job["url"], name=job["name"], stage=job["stage"], retries=len(job["retry_summary"]) - 1
                )

        for owner, message in messages_to_send.items():
            message += "\n(Test message, the real message would be sent to {})".format(owner)
            send_slack_message("#agent-pipeline-notifications", message)
