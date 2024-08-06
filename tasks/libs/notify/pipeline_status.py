import os
import re

from tasks.libs.ciproviders.gitlab_api import get_commit, get_pipeline
from tasks.libs.common.constants import DEFAULT_BRANCH
from tasks.libs.notify.utils import DEPLOY_PIPELINES_CHANNEL, PIPELINES_CHANNEL, PROJECT_NAME
from tasks.libs.pipeline.data import get_failed_jobs
from tasks.libs.pipeline.notifications import (
    base_message,
    email_to_slackid,
    get_failed_tests,
    send_slack_message,
)
from tasks.libs.types.types import SlackMessage


def should_send_message_to_author(git_ref: str, default_branch: str) -> bool:
    # Must match X.Y.Z, X.Y.x, W.X.Y-rc.Z
    # Must not match W.X.Y-rc.Z-some-feature
    release_ref_regex = re.compile(r"^[0-9]+\.[0-9]+\.(x|[0-9]+)$")
    release_ref_regex_rc = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]-rc.[0-9]+$")

    return not (git_ref == default_branch or release_ref_regex.match(git_ref) or release_ref_regex_rc.match(git_ref))


def send_message(ctx, notification_type, dry_run):
    pipeline = get_pipeline(PROJECT_NAME, os.environ["CI_PIPELINE_ID"])
    commit = get_commit(PROJECT_NAME, pipeline.sha)
    failed_jobs = get_failed_jobs(pipeline)

    # From the job failures, set whether the pipeline succeeded or failed and craft the
    # base message that will be sent.
    if failed_jobs.all_mandatory_failures():  # At least one mandatory job failed
        header_icon = ":host-red:"
        state = "failed"
    else:
        header_icon = ":host-green:"
        state = "succeeded"

    # For deploy pipelines not on the main branch, send notifications in a
    # dedicated channel.
    slack_channel = PIPELINES_CHANNEL
    if notification_type == "deploy" and pipeline.ref != DEFAULT_BRANCH:
        slack_channel = DEPLOY_PIPELINES_CHANNEL

    header = ""
    if notification_type == "merge":
        header = f"{header_icon} :merged: datadog-agent merge"
    elif notification_type == "deploy":
        header = f"{header_icon} :rocket: datadog-agent deploy"
    elif notification_type == "trigger":
        header = f"{header_icon} :arrow_forward: datadog-agent triggered"

    message = SlackMessage(jobs=failed_jobs)
    message.base_message = base_message(PROJECT_NAME, pipeline, commit, header, state)

    for job in failed_jobs.all_non_infra_failures():
        for test in get_failed_tests(PROJECT_NAME, job):
            message.add_test_failure(test, job)

    # Send messages
    if dry_run:
        print(f"Would send to {slack_channel}:\n{str(message)}")
    else:
        send_slack_message(slack_channel, str(message))

    if should_send_message_to_author(pipeline.ref, DEFAULT_BRANCH):
        author_email = commit.author_email
        if dry_run:
            print(f"Would send to {author_email}:\n{str(message)}")
        else:
            recipient = email_to_slackid(ctx, author_email)
            send_slack_message(recipient, str(message))
