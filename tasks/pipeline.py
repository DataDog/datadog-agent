import io
import os
import pprint
import re
import traceback
from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from tasks.utils import DEFAULT_BRANCH

from .libs.common.color import color_message
from .libs.common.gitlab import Gitlab
from .libs.pipeline_notifications import (
    base_message,
    find_job_owners,
    get_failed_jobs,
    get_failed_tests,
    send_slack_message,
)
from .libs.pipeline_tools import (
    cancel_pipelines_with_confirmation,
    get_running_pipelines_on_same_ref,
    trigger_agent_pipeline,
    wait_for_pipeline,
)
from .libs.types import SlackMessage, TeamMessage

# Tasks to trigger pipelines

ALLOWED_REPO_BRANCHES = {"stable", "beta", "nightly", "none"}


def check_deploy_pipeline(gitlab, project_name, git_ref, release_version_6, release_version_7, repo_branch):
    """
    Run checks to verify a deploy pipeline is valid:
    - it targets a valid repo branch
    - it has matching Agent 6 and Agent 7 tags (depending on release_version_* values)
    """

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


@task
def clean_running_pipelines(ctx, git_ref=DEFAULT_BRANCH, here=False, use_latest_sha=False, sha=None):
    """
    Fetch running pipelines on a target ref (+ optionally a git sha), and ask the user if they
    should be cancelled.
    """

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    print("Fetching running pipelines on {}".format(git_ref))

    if not sha and use_latest_sha:
        sha = ctx.run("git rev-parse {}".format(git_ref), hide=True).stdout.strip()
        print("Git sha not provided, using the one {} currently points to: {}".format(git_ref, sha))
    elif not sha:
        print("Git sha not provided, fetching all running pipelines on {}".format(git_ref))

    pipelines = get_running_pipelines_on_same_ref(gitlab, project_name, git_ref, sha)

    print(
        "Found {} running pipeline(s) matching the request.".format(len(pipelines)),
        "They are ordered from the newest one to the oldest one.\n",
        sep='\n',
    )
    cancel_pipelines_with_confirmation(gitlab, project_name, pipelines)


@task
def trigger(
    ctx, git_ref=DEFAULT_BRANCH, release_version_6="nightly", release_version_7="nightly-a7", repo_branch="nightly"
):
    """
    DEPRECATED: Trigger a deploy pipeline on the given git ref. Use pipeline.run with the --deploy option instead.

    The --release-version-6 and --release-version-7 options indicate which release.json entries are used.
    To not build Agent 6, set --release-version-6 "". To not build Agent 7, set --release-version-7 "".
    The --repo-branch option indicates which branch of the staging repository the packages will be deployed to.

    Example:
    inv pipeline.trigger --git-ref 7.22.0 --release-version-6 "6.22.0" --release-version-7 "7.22.0" --repo-branch "stable"
    """
    print(
        color_message(
            "WARNING: the pipeline.trigger invoke task is deprecated and will be removed in the future.\n"
            + "         Use pipeline.run with the --deploy option instead.",
            "orange",
        )
    )

    run(
        ctx,
        git_ref=git_ref,
        release_version_6=release_version_6,
        release_version_7=release_version_7,
        repo_branch=repo_branch,
        deploy=True,
        all_builds=True,
        kitchen_tests=True,
    )


@task
def run(
    ctx,
    git_ref=DEFAULT_BRANCH,
    here=False,
    release_version_6="nightly",
    release_version_7="nightly-a7",
    repo_branch="nightly",
    deploy=False,
    all_builds=True,
    kitchen_tests=True,
):
    """
    Run a pipeline on the given git ref, or on the current branch if --here is given.
    By default, this pipeline will run all builds & tests, including all kitchen tests, but is not a deploy pipeline.
    Use --deploy to make this pipeline a deploy pipeline, which will upload artifacts to the staging repositories.
    Use --no-all-builds to not run builds for all architectures (only a subset of jobs will run. No effect on pipelines on the default branch).
    Use --no-kitchen-tests to not run all kitchen tests on the pipeline.

    The --release-version-6 and --release-version-7 options indicate which release.json entries are used.
    To not build Agent 6, set --release-version-6 "". To not build Agent 7, set --release-version-7 "".
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

    Run a deploy pipeline on the 7.28.0 tag, uploading the artifacts to the stable branch of the staging repositories:
      inv pipeline.run --deploy --git-ref 7.28.0 --release-version-6 "6.28.0" --release-version-7 "7.28.0" --repo-branch "stable"
    """

    project_name = "DataDog/datadog-agent"
    gitlab = Gitlab()
    gitlab.test_project_found(project_name)

    if deploy:
        # Check the validity of the deploy pipeline
        check_deploy_pipeline(
            gitlab, project_name, git_ref, release_version_6, release_version_7, repo_branch,
        )
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

    if here:
        git_ref = ctx.run("git rev-parse --abbrev-ref HEAD", hide=True).stdout.strip()

    pipelines = get_running_pipelines_on_same_ref(gitlab, project_name, git_ref)

    if pipelines:
        print(
            "There are already {} pipeline(s) running on the target git ref.".format(len(pipelines)),
            "For each of them, you'll be asked whether you want to cancel them or not.",
            "If you don't need these pipelines, please cancel them to save CI resources.",
            "They are ordered from the newest one to the oldest one.\n",
            sep='\n',
        )
        cancel_pipelines_with_confirmation(gitlab, project_name, pipelines)

    pipeline_id = trigger_agent_pipeline(
        gitlab,
        project_name,
        git_ref,
        release_version_6,
        release_version_7,
        repo_branch,
        deploy=deploy,
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
    "@DataDog/container-integrations": "#container-integrations",
    "@DataDog/integrations-tools-and-libraries": "#intg-tools-libs",
    "@DataDog/agent-network": "#network-agent",
    "@DataDog/agent-security": "#security-and-compliance-agent",
    "@DataDog/agent-apm": "#apm-agent",
    "@DataDog/infrastructure-integrations": "#infrastructure-integrations",
    "@DataDog/processes": "#processes",
    "@DataDog/agent-core": "#agent-core",
    "@DataDog/container-app": "#container-app",
    "@Datadog/metrics-aggregation": "#metrics-aggregation",
    "@Datadog/serverless": "#serverless-agent",
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
            messages_to_send[owner].failed_jobs = jobs

    return messages_to_send


@task
def trigger_child_pipeline(_, git_ref, project_name, variables=""):
    """
    Trigger a child pipeline on a target repository and git ref.
    Used in CI jobs only (requires CI_JOB_TOKEN).

    Use --variables to specify the environment variables that should be passed to the child pipeline, as a comma-separated list.

    Examples:
    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "RELEASE_VERSION"

    inv pipeline.trigger-child-pipeline --git-ref "master" --project-name "DataDog/agent-release-management" --variables "VAR1,VAR2,VAR3"
    """

    if not os.environ.get('CI_JOB_TOKEN'):
        raise Exit("CI_JOB_TOKEN variable needed to create child pipelines.", 1)

    # The Gitlab lib requires `GITLAB_TOKEN` to be set, though
    # we won't use it here
    os.environ["GITLAB_TOKEN"] = os.environ['CI_JOB_TOKEN']

    gitlab = Gitlab()

    data = {"token": os.environ['CI_JOB_TOKEN'], "ref": git_ref, "variables": {}}

    # Fill the environment variables to pass to the child pipeline.
    for v in variables.split(','):
        data['variables'][v] = os.environ[v]

    print(
        "Creating child pipeline in repo {}, on git ref {} with params: {}".format(
            project_name, git_ref, data['variables']
        )
    )

    res = gitlab.trigger_pipeline(project_name, data)

    if 'id' not in res:
        raise Exit("Failed to create child pipeline: {}".format(res), 1)

    print("Created a child pipeline with id={}, url={}".format(res['id'], res['web_url']))

    # TODO: Add mode where we follow the pipeline, for jobs that need to depend on the child
    # pipeline.


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


def _init_pipeline_schedule_task():
    project_name = "DataDog/datadog-agent"
    try:
        project_access_token = os.environ['GITLAB_PROJECT_ACCESS_TOKEN']
    except KeyError:
        raise Exit(message="You must specify GITLAB_PROJECT_ACCESS_TOKEN environment variable", code=1)
    gitlab = Gitlab(api_token=project_access_token)
    gitlab.test_project_found(project_name)
    return project_name, gitlab


@task
def get_schedules(_):
    """
    Pretty-print all pipeline schedules on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    for ps in gitlab.all_pipeline_schedules(project_name):
        pprint.pprint(ps)


@task
def get_schedule(_, schedule_id):
    """
    Pretty-print a single pipeline schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.pipeline_schedule(project_name, schedule_id)
    pprint.pprint(result)


@task
def create_schedule(_, description, ref, cron, cron_timezone=None, active=False):
    """
    Create a new pipeline schedule on the repository.

    Note that unless you explicitly specify the --active flag, the schedule will be created as inactive.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule(project_name, description, ref, cron, cron_timezone, active)
    pprint.pprint(result)


@task
def edit_schedule(_, schedule_id, description=None, ref=None, cron=None, cron_timezone=None):
    """
    Edit an existing pipeline schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(project_name, schedule_id, description, ref, cron, cron_timezone)
    pprint.pprint(result)


@task
def activate_schedule(_, schedule_id):
    """
    Activate an existing pipeline schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(project_name, schedule_id, active=True)
    pprint.pprint(result)


@task
def deactivate_schedule(_, schedule_id):
    """
    Deactivate an existing pipeline schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule(project_name, schedule_id, active=False)
    pprint.pprint(result)


@task
def delete_schedule(_, schedule_id):
    """
    Delete an existing pipeline schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule(project_name, schedule_id)
    pprint.pprint(result)


@task
def create_schedule_variable(_, schedule_id, key, value):
    """
    Create a variable for an existing schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.create_pipeline_schedule_variable(project_name, schedule_id, key, value)
    pprint.pprint(result)


@task
def edit_schedule_variable(_, schedule_id, key, value):
    """
    Edit an existing variable for a schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.edit_pipeline_schedule_variable(project_name, schedule_id, key, value)
    pprint.pprint(result)


@task
def delete_schedule_variable(_, schedule_id, key):
    """
    Delete an existing variable for a schedule on the repository.
    """

    project_name, gitlab = _init_pipeline_schedule_task()
    result = gitlab.delete_pipeline_schedule_variable(project_name, schedule_id, key)
    pprint.pprint(result)
