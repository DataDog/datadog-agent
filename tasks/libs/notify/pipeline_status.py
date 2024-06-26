import os
import re
from collections import defaultdict
from datetime import datetime, timezone

from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.notify.utils import NOTIFICATION_DISCLAIMER
from tasks.libs.pipeline.notifications import (
    GITHUB_SLACK_MAP,
    base_message,
    email_to_slackid,
    find_job_owners,
    get_failed_tests,
    get_git_author,
    send_slack_message,
)
from tasks.libs.types.types import FailedJobs, SlackMessage, TeamMessage

UNKNOWN_OWNER_TEMPLATE = """The owner `{owner}` is not mapped to any slack channel.
Please check for typos in the JOBOWNERS file and/or add them to the Github <-> Slack map.
"""


def should_send_message_to_channel(git_ref: str, default_branch: str) -> bool:
    # Must match X.Y.Z, X.Y.x, W.X.Y-rc.Z
    # Must not match W.X.Y-rc.Z-some-feature
    release_ref_regex = re.compile(r"^[0-9]+\.[0-9]+\.(x|[0-9]+)$")
    release_ref_regex_rc = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]-rc.[0-9]+$")

    return git_ref == default_branch or release_ref_regex.match(git_ref) or release_ref_regex_rc.match(git_ref)


def generate_failure_messages(project_name: str, failed_jobs: FailedJobs) -> dict[str, SlackMessage]:
    all_teams = "@DataDog/agent-all"

    # Generate messages for each team
    messages_to_send = defaultdict(TeamMessage)
    messages_to_send[all_teams] = SlackMessage(jobs=failed_jobs)

    failed_job_owners = find_job_owners(failed_jobs)
    for owner, jobs in failed_job_owners.items():
        if owner == "@DataDog/multiple":
            for job in jobs.all_non_infra_failures():
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


def send_message_and_metrics(ctx, failed_jobs, messages_to_send, notification_type, print_to_stdout):
    # From the job failures, set whether the pipeline succeeded or failed and craft the
    # base message that will be sent.
    if failed_jobs.all_mandatory_failures():  # At least one mandatory job failed
        header_icon = ":host-red:"
        state = "failed"
        coda = NOTIFICATION_DISCLAIMER
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
    metrics = []
    timestamp = int(datetime.now(timezone.utc).timestamp())
    for owner, message in messages_to_send.items():
        channel = GITHUB_SLACK_MAP.get(owner.lower(), None)
        message.base_message = base
        if channel is None:
            channel = "#datadog-agent-pipelines"
            message.base_message += UNKNOWN_OWNER_TEMPLATE.format(owner=owner)
        message.coda = coda
        if print_to_stdout:
            print(f"Would send to {channel}:\n{str(message)}")
        else:
            all_teams = channel == "#datadog-agent-pipelines"
            default_branch = os.environ["CI_DEFAULT_BRANCH"]
            git_ref = os.environ["CI_COMMIT_REF_NAME"]
            send_dm = not should_send_message_to_channel(git_ref, default_branch) and all_teams

            if all_teams:
                recipient = channel
                send_slack_message(recipient, str(message))
                metrics.append(
                    create_count(
                        metric_name="datadog.ci.failed_job_notifications",
                        timestamp=timestamp,
                        tags=[
                            f"team:{owner}",
                            "repository:datadog-agent",
                            f"git_ref:{git_ref}",
                        ],
                        unit="notification",
                        value=1,
                    )
                )

            # DM author
            if send_dm:
                author_email = get_git_author(email=True)
                recipient = email_to_slackid(ctx, author_email)
                send_slack_message(recipient, str(message))

    if metrics:
        send_metrics(metrics)
