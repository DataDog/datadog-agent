import io
import os
import pprint
import re
import traceback
from collections import defaultdict
from datetime import datetime

import yaml
from invoke import task
from invoke.exceptions import Exit

from .libs.common.color import color_message
from .libs.common.gitlab import Gitlab, get_gitlab_bot_token, get_gitlab_token
from .libs.datadog_api import create_count, send_metrics
from .libs.pipeline_data import get_failed_jobs
from .libs.pipeline_notifications import (
    GITHUB_SLACK_MAP,
    base_message,
    find_job_owners,
    get_failed_tests,
    send_slack_message,
)
from .libs.pipeline_stats import get_failed_jobs_stats
from .libs.pipeline_tools import (
    FilteredOutException,
    cancel_pipelines_with_confirmation,
    get_running_pipelines_on_same_ref,
    trigger_agent_pipeline,
    wait_for_pipeline,
)
from .libs.types import SlackMessage, TeamMessage
from .utils import (
    DEFAULT_BRANCH,
    get_all_allowed_repo_branches,
    is_allowed_repo_branch,
    nightly_entry_for,
    release_entry_for,
)

# Tasks to trigger pipelines


def check_deploy_pipeline(gitlab, git_ref, release_version_6, release_version_7, repo_branch):
    """
    Run checks to verify a deploy pipeline is valid:
    - it targets a valid repo branch
    - it has matching Agent 6 and Agent 7 tags (depending on release_version_* values)
    """

    # Check that the target repo branch is valid
    if not is_allowed_repo_branch(repo_branch):
        print(
            f"--repo-branch argument '{repo_branch}' is not in the list of allowed repository branches: {get_all_allowed_repo_branches()}"
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
        gitlab_tag = gitlab.find_tag(tag_name)

        if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
            print(f"Cannot find GitLab v6 tag {tag_name} while trying to build git ref {git_ref}")
            raise Exit(code=1)

        print(f"Successfully cross checked v6 tag {tag_name} and git ref {git_ref}")
    else:
        match = re.match(v6_pattern, git_ref)

        if release_version_7 and match:
            # release_version_7 is not empty and git_ref matches v6 pattern, construct v7 tag and check.
            tag_name = "7." + "".join(match.groups())
            gitlab_tag = gitlab.find_tag(tag_name)

            if ("name" not in gitlab_tag) or gitlab_tag["name"] != tag_name:
                print(f"Cannot find GitLab v7 tag {tag_name} while trying to build git ref {git_ref}")
                raise Exit(code=1)

            print(f"Successfully cross checked v7 tag {tag_name} and git ref {git_ref}")


@task
def clean_running_pipelines(ctx, git_ref=DEFAULT_BRANCH, here=False, use_latest_sha=False, sha=None):
    """
    Fetch running pipelines on a target ref (+ optionally a git sha), and ask the user if they
    should be cancelled.
    """

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
    gitlab.test_project_found()

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    print(f"Fetching running pipelines on {git_ref}")

    if not sha and use_latest_sha:
        sha = ctx.run(f"git rev-parse {git_ref}", hide=True).stdout.strip()
        print(f"Git sha not provided, using the one {git_ref} currently points to: {sha}")
    elif not sha:
        print(f"Git sha not provided, fetching all running pipelines on {git_ref}")

    pipelines = get_running_pipelines_on_same_ref(gitlab, git_ref, sha)

    print(
        f"Found {len(pipelines)} running pipeline(s) matching the request.",
        "They are ordered from the newest one to the oldest one.\n",
        sep='\n',
    )
    cancel_pipelines_with_confirmation(gitlab, pipelines)


def workflow_rules(gitlab_file=".gitlab-ci.yml"):
    """Get Gitlab workflow rules list in a YAML-formatted string."""
    with open(gitlab_file, 'r') as f:
        return yaml.dump(yaml.safe_load(f.read())["workflow"]["rules"])


@task
def trigger(
    _, git_ref=DEFAULT_BRANCH, release_version_6="nightly", release_version_7="nightly-a7", repo_branch="nightly"
):
    """
    OBSOLETE: Trigger a deploy pipeline on the given git ref. Use pipeline.run with the --deploy option instead.
    """

    use_release_entries = ""
    major_versions = []

    if release_version_6 != "nightly" and release_version_7 != "nightly-a7":
        use_release_entries = "--use-release-entries "

    if release_version_6 != "":
        major_versions.append("6")

    if release_version_7 != "":
        major_versions.append("7")

    raise Exit(
        f"""The pipeline.trigger task is obsolete. Use:
    pipeline.run --git-ref {git_ref} --deploy --major-versions "{','.join(major_versions)}" --repo-branch {repo_branch} {use_release_entries}
instead.""",
        1,
    )


@task
def run(
    ctx,
    git_ref=None,
    here=False,
    use_release_entries=False,
    major_versions='6,7',
    repo_branch="nightly",
    deploy=False,
    all_builds=True,
    kitchen_tests=True,
):
    """
    Run a pipeline on the given git ref (--git-ref <git ref>), or on the current branch if --here is given.
    By default, this pipeline will run all builds & tests, including all kitchen tests, but is not a deploy pipeline.
    Use --deploy to make this pipeline a deploy pipeline, which will upload artifacts to the staging repositories.
    Use --no-all-builds to not run builds for all architectures (only a subset of jobs will run. No effect on pipelines on the default branch).
    Use --no-kitchen-tests to not run all kitchen tests on the pipeline.

    By default, the nightly release.json entries (nightly and nightly-a7) are used.
    Use the --use-release-entries option to use the release-a6 and release-a7 release.json entries instead.

    By default, the pipeline builds both Agent 6 and Agent 7.
    Use the --major-versions option to specify a comma-separated string of the major Agent versions to build
    (eg. '6' to build Agent 6 only, '6,7' to build both Agent 6 and Agent 7).

    The --repo-branch option indicates which branch of the staging repository the packages will be deployed to (useful only on deploy pipelines).

    If other pipelines are already running on the git ref, the script will prompt the user to confirm if these previous
    pipelines should be cancelled.

    Examples
    Run a pipeline on my-branch:
      inv pipeline.run --git-ref my-branch

    Run a pipeline on the current branch:
      inv pipeline.run --here

    Run a pipeline without kitchen tests on the current branch:
      inv pipeline.run --here --no-kitchen-tests

    Run a deploy pipeline on the 7.32.0 tag, uploading the artifacts to the stable branch of the staging repositories:
      inv pipeline.run --deploy --use-release-entries --major-versions "6,7" --git-ref "7.32.0" --repo-branch "stable"
    """

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
    gitlab.test_project_found()

    if (not git_ref and not here) or (git_ref and here):
        raise Exit("ERROR: Exactly one of --here or --git-ref <git ref> must be specified.", code=1)

    if use_release_entries:
        release_version_6 = release_entry_for(6)
        release_version_7 = release_entry_for(7)
    else:
        release_version_6 = nightly_entry_for(6)
        release_version_7 = nightly_entry_for(7)

    major_versions = major_versions.split(',')
    if '6' not in major_versions:
        release_version_6 = ""
    if '7' not in major_versions:
        release_version_7 = ""

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    if deploy:
        # Check the validity of the deploy pipeline
        check_deploy_pipeline(gitlab, git_ref, release_version_6, release_version_7, repo_branch)
        # Force all builds and kitchen tests to be run
        if not all_builds:
            print(
                color_message(
                    "WARNING: ignoring --no-all-builds option, RUN_ALL_BUILDS is automatically set to true on deploy pipelines",
                    "orange",
                )
            )
            all_builds = True
        if not kitchen_tests:
            print(
                color_message(
                    "WARNING: ignoring --no-kitchen-tests option, RUN_KITCHEN_TESTS is automatically set to true on deploy pipelines",
                    "orange",
                )
            )
            kitchen_tests = True

    pipelines = get_running_pipelines_on_same_ref(gitlab, git_ref)

    if pipelines:
        print(
            f"There are already {len(pipelines)} pipeline(s) running on the target git ref.",
            "For each of them, you'll be asked whether you want to cancel them or not.",
            "If you don't need these pipelines, please cancel them to save CI resources.",
            "They are ordered from the newest one to the oldest one.\n",
            sep='\n',
        )
        cancel_pipelines_with_confirmation(gitlab, pipelines)

    try:
        pipeline_id = trigger_agent_pipeline(
            gitlab,
            git_ref,
            release_version_6,
            release_version_7,
            repo_branch,
            deploy=deploy,
            all_builds=all_builds,
            kitchen_tests=kitchen_tests,
        )
    except FilteredOutException:
        print(color_message(f"ERROR: pipeline does not match any workflow rule. Rules:\n{workflow_rules()}", "red"))
        return

    wait_for_pipeline(gitlab, pipeline_id)


@task
def follow(ctx, id=None, git_ref=None, here=False, project_name="DataDog/datadog-agent"):
    """
    Follow a pipeline's progress in the CLI.
    Use --here to follow the latest pipeline on your current branch.
    Use --git-ref to follow the latest pipeline on a given tag or branch.
    Use --id to follow a specific pipeline.
    Use --project-name to specify a repo other than DataDog/datadog-agent (default)

    Examples:
    inv pipeline.follow --git-ref my-branch
    inv pipeline.follow --here
    inv pipeline.follow --id 1234567
    """

    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
    gitlab.test_project_found()

    args_given = 0
    if id is not None:
        args_given += 1
    if git_ref is not None:
        args_given += 1
    if here:
        args_given += 1
    if args_given != 1:
        raise Exit(
            "ERROR: Exactly one of --here, --git-ref or --id must be given.\nSee --help for an explanation of each.",
            code=1,
        )

    if id is not None:
        wait_for_pipeline(gitlab, id)
    elif git_ref is not None:
        wait_for_pipeline_from_ref(gitlab, git_ref)
    elif here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()
        wait_for_pipeline_from_ref(gitlab, git_ref)


def wait_for_pipeline_from_ref(gitlab, ref):
    pipeline = gitlab.last_pipeline_for_ref(ref)
    if pipeline is not None:
        wait_for_pipeline(gitlab, pipeline['id'])
    else:
        print(f"No pipelines found for {ref}")
        raise Exit(code=1)


# Tasks to trigger pipeline notifications

UNKNOWN_OWNER_TEMPLATE = """The owner `{owner}` is not mapped to any slack channel.
Please check for typos in the JOBOWNERS file and/or add them to the Github <-> Slack map.
"""


def generate_failure_messages(project_name, failed_jobs):
    all_teams = "@DataDog/agent-all"

    # Generate messages for each team
    messages_to_send = defaultdict(TeamMessage)
    messages_to_send[all_teams] = SlackMessage(jobs=failed_jobs)

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
            messages_to_send[owner].failed_jobs = jobs

    return messages_to_send


@task
def trigger_child_pipeline(_, git_ref, project_name, variables="", follow=True):
    """
    Trigger a child pipeline on a target repository and git ref.
    Used in CI jobs only (requires CI_JOB_TOKEN).

    Use --variables to specify the environment variables that should be passed to the child pipeline, as a comma-separated list.

    Use --follow to make this task wait for the pipeline to finish, and return 1 if it fails. (requires GITLAB_TOKEN).

    Examples:
    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "RELEASE_VERSION"

    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "VAR1,VAR2,VAR3"
    """

    if not os.environ.get('CI_JOB_TOKEN'):
        raise Exit("CI_JOB_TOKEN variable needed to create child pipelines.", 1)

    if not os.environ.get('GITLAB_TOKEN'):
        if follow:
            raise Exit("GITLAB_TOKEN variable needed to follow child pipelines.", 1)
        else:
            # The Gitlab lib requires `GITLAB_TOKEN` to be
            # set, but trigger_pipeline doesn't use it
            os.environ["GITLAB_TOKEN"] = os.environ['CI_JOB_TOKEN']

    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())

    data = {"token": os.environ['CI_JOB_TOKEN'], "ref": git_ref, "variables": {}}

    # Fill the environment variables to pass to the child pipeline.
    for v in variables.split(','):
        data['variables'][v] = os.environ[v]

    print(f"Creating child pipeline in repo {project_name}, on git ref {git_ref} with params: {data['variables']}")

    res = gitlab.trigger_pipeline(data)

    if 'id' not in res:
        raise Exit(f"Failed to create child pipeline: {res}", code=1)

    pipeline_id = res['id']
    pipeline_url = res['web_url']
    print(f"Created a child pipeline with id={pipeline_id}, url={pipeline_url}")

    if follow:
        print("Waiting for child pipeline to finish...")

        wait_for_pipeline(gitlab, pipeline_id)

        # Check pipeline status
        pipeline = gitlab.pipeline(pipeline_id)
        pipestatus = pipeline["status"].lower().strip()

        if pipestatus != "success":
            raise Exit(f"Error: child pipeline status {pipestatus.title()}", code=1)

        print("Child pipeline finished successfully")


@task
def notify(_, notification_type="merge", print_to_stdout=False):
    """
    Send notifications for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    real slack messages.
    """
    project_name = "DataDog/datadog-agent"

    try:
        failed_jobs = get_failed_jobs(project_name, os.getenv("CI_PIPELINE_ID"))
        messages_to_send = generate_failure_messages(project_name, failed_jobs)
    except Exception as e:
        buffer = io.StringIO()
        print(base_message("datadog-agent", "is in an unknown state"), file=buffer)
        print("Found exception when generating notification:", file=buffer)
        traceback.print_exc(limit=-1, file=buffer)
        print("See the notify job log for the full exception traceback.", file=buffer)

        messages_to_send = {
            "@DataDog/agent-all": SlackMessage(base=buffer.getvalue()),
        }
        # Print traceback on job log
        print(e)
        traceback.print_exc()
        raise Exit(code=1)

    # From the job failures, set whether the pipeline succeeded or failed and craft the
    # base message that will be sent.
    if failed_jobs:  # At least one job failed
        header_icon = ":host-red:"
        state = "failed"
        coda = "If there is something wrong with the notification please contact #agent-platform"
    else:
        header_icon = ":host-green:"
        state = "succeeded"
        coda = ""

    header = ""
    if notification_type == "merge":
        header = f"{header_icon} :merged: datadog-agent merge"
    elif notification_type == "deploy":
        header = f"{header_icon} :rocket: datadog-agent deploy"
    base = base_message(header, state)

    # Send messages
    for owner, message in messages_to_send.items():
        channel = GITHUB_SLACK_MAP.get(owner, "#datadog-agent-pipelines")
        message.base_message = base
        if owner not in GITHUB_SLACK_MAP.keys():
            message.base_message += UNKNOWN_OWNER_TEMPLATE.format(owner=owner)
        message.coda = coda
        if print_to_stdout:
            print(f"Would send to {channel}:\n{str(message)}")
        else:
            send_slack_message(channel, str(message))  # TODO: use channel variable


@task
def send_stats(_, print_to_stdout=False):
    """
    Send statistics to Datadog for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    data points to Datadog.
    """
    project_name = "DataDog/datadog-agent"

    try:
        global_failure_reason, job_failure_stats = get_failed_jobs_stats(project_name, os.getenv("CI_PIPELINE_ID"))
    except Exception as e:
        print("Found exception when generating statistics:")
        print(e)
        traceback.print_exc(limit=-1)
        raise Exit(code=1)

    if not (print_to_stdout or os.environ.get("DD_API_KEY")):
        print("DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend")
        raise Exit(code=1)

    timestamp = int(datetime.now().timestamp())
    series = []

    for failure_tags, count in job_failure_stats.items():
        # This allows getting stats on the number of jobs that fail due to infrastructure
        # issues vs. other failures, and have a per-pipeline ratio of infrastructure failures.
        series.append(
            create_count(
                metric_name="datadog.ci.job_failures",
                timestamp=timestamp,
                value=count,
                tags=list(failure_tags)
                + [
                    "repository:datadog-agent",
                    f"git_ref:{os.getenv('CI_COMMIT_REF_NAME')}",
                ],
            )
        )

    if job_failure_stats:  # At least one job failed
        pipeline_state = "failed"
    else:
        pipeline_state = "succeeded"

    pipeline_tags = [
        "repository:datadog-agent",
        f"git_ref:{os.getenv('CI_COMMIT_REF_NAME')}",
        f"status:{pipeline_state}",
    ]
    if global_failure_reason:  # Only set the reason if the pipeline fails
        pipeline_tags.append(f"reason:{global_failure_reason}")

    series.append(
        create_count(
            metric_name="datadog.ci.pipelines",
            timestamp=timestamp,
            value=1,
            tags=pipeline_tags,
        )
    )

    if not print_to_stdout:
        response = send_metrics(series)
        if response["errors"]:
            print(f"Error(s) while sending pipeline metrics to the Datadog backend: {response['errors']}")
            raise Exit(code=1)
        print(f"Sent pipeline metrics: {series}")
    else:
        print(f"Would send: {series}")


def _init_pipeline_schedule_task():
    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_bot_token())
    gitlab.test_project_found()
    return gitlab


@task
def get_schedules(_):
    """
    Pretty-print all pipeline schedules on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    for ps in gitlab.all_pipeline_schedules():
        pprint.pprint(ps)


@task
def get_schedule(_, schedule_id):
    """
    Pretty-print a single pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.pipeline_schedule(schedule_id)
    pprint.pprint(result)


@task
def create_schedule(_, description, ref, cron, cron_timezone=None, active=False):
    """
    Create a new pipeline schedule on the repository.

    Note that unless you explicitly specify the --active flag, the schedule will be created as inactive.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule(description, ref, cron, cron_timezone, active)
    pprint.pprint(result)


@task
def edit_schedule(_, schedule_id, description=None, ref=None, cron=None, cron_timezone=None):
    """
    Edit an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, description, ref, cron, cron_timezone)
    pprint.pprint(result)


@task
def activate_schedule(_, schedule_id):
    """
    Activate an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, active=True)
    pprint.pprint(result)


@task
def deactivate_schedule(_, schedule_id):
    """
    Deactivate an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(schedule_id, active=False)
    pprint.pprint(result)


@task
def delete_schedule(_, schedule_id):
    """
    Delete an existing pipeline schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule(schedule_id)
    pprint.pprint(result)


@task
def create_schedule_variable(_, schedule_id, key, value):
    """
    Create a variable for an existing schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule_variable(schedule_id, key, value)
    pprint.pprint(result)


@task
def edit_schedule_variable(_, schedule_id, key, value):
    """
    Edit an existing variable for a schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule_variable(schedule_id, key, value)
    pprint.pprint(result)


@task
def delete_schedule_variable(_, schedule_id, key):
    """
    Delete an existing variable for a schedule on the repository.
    """

    gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule_variable(schedule_id, key)
    pprint.pprint(result)
