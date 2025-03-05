import os
import re
import sys

from tasks.libs.ciproviders.gitlab_api import get_commit, get_pipeline
from tasks.libs.common.git import get_default_branch
from tasks.libs.common.utils import Color, color_message
from tasks.libs.notify.utils import DEPLOY_PIPELINES_CHANNEL, PIPELINES_CHANNEL, PROJECT_NAME, get_pipeline_type
from tasks.libs.pipeline.data import get_failed_jobs, get_jobs_skipped_on_pr
from tasks.libs.pipeline.notifications import (
    base_message,
    get_failed_tests,
)
from tasks.libs.types.types import SlackMessage


def should_send_message_to_author(git_ref: str, default_branch: str) -> bool:
    # Must match X.Y.Z, X.Y.x, W.X.Y-rc.Z
    # Must not match W.X.Y-rc.Z-some-feature
    release_ref_regex = re.compile(r"^[0-9]+\.[0-9]+\.(x|[0-9]+)$")
    release_ref_regex_rc = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]-rc.[0-9]+$")

    return not (git_ref == default_branch or release_ref_regex.match(git_ref) or release_ref_regex_rc.match(git_ref))


def send_message(pipeline_id, dry_run):
    pipeline = get_pipeline(PROJECT_NAME, pipeline_id)
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
    pipeline_type = get_pipeline_type(pipeline)
    if pipeline_type == "deploy" and pipeline.ref != get_default_branch():
        slack_channel = DEPLOY_PIPELINES_CHANNEL

    header = ""
    if pipeline_type == "merge":
        header = f"{header_icon} :merged: datadog-agent merge"
    elif pipeline_type == "deploy":
        header = f"{header_icon} :rocket: datadog-agent deploy"
    elif pipeline_type == "trigger":
        header = f"{header_icon} :arrow_forward: datadog-agent triggered"

    skipped_jobs, pr_pipeline_url = get_jobs_skipped_on_pr(pipeline, failed_jobs)
    message = SlackMessage(jobs=failed_jobs, skipped=skipped_jobs)
    message.base_message = base_message(PROJECT_NAME, pipeline, commit, header, state, pr_pipeline_url)

    for job in failed_jobs.all_non_infra_failures():
        for test in get_failed_tests(PROJECT_NAME, job):
            message.add_test_failure(test, job)

    if dry_run:
        print(f"Would send to {slack_channel}:\n{str(message)}")
        if should_send_message_to_author(pipeline.ref, get_default_branch()):
            print(f"Would send to {commit.author_email}:\n{str(message)}")
        return

    # Send message
    from slack_sdk import WebClient
    from slack_sdk.errors import SlackApiError

    client = WebClient(token=os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
    client.chat_postMessage(channel=slack_channel, text=str(message))
    if should_send_message_to_author(pipeline.ref, get_default_branch()):
        author_email = commit.author_email
        try:
            recipient = client.users_lookupByEmail(email=author_email)
            client.chat_postMessage(channel=recipient.data['user']['id'], text=str(message))
        except SlackApiError as e:
            print(
                f"[{color_message('ERROR', Color.RED)}] Failed to send message to {author_email}: {e.response['error']}",
                file=sys.stderr,
            )
