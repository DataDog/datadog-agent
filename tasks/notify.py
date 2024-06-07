from __future__ import annotations

import io
import json
import os
import re
import tempfile
import traceback
from collections import defaultdict
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone
from urllib.parse import quote

from invoke import task
from invoke.context import Context
from invoke.exceptions import Exit, UnexpectedExit

from tasks.libs.ciproviders.gitlab_api import BASE_URL
from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.pipeline.data import get_failed_jobs
from tasks.libs.pipeline.notifications import (
    GITHUB_SLACK_MAP,
    base_message,
    check_for_missing_owners_slack_and_jira,
    email_to_slackid,
    find_job_owners,
    get_failed_tests,
    get_git_author,
    get_pr_from_commit,
    send_slack_message,
)
from tasks.libs.pipeline.stats import compute_failed_jobs_series, compute_required_jobs_max_duration
from tasks.libs.types.types import FailedJobs, SlackMessage, TeamMessage
from tasks.owners import make_partition

UNKNOWN_OWNER_TEMPLATE = """The owner `{owner}` is not mapped to any slack channel.
Please check for typos in the JOBOWNERS file and/or add them to the Github <-> Slack map.
"""
PROJECT_NAME = "DataDog/datadog-agent"
PROJECT_TITLE = PROJECT_NAME.removeprefix("DataDog/")
AWS_S3_CP_CMD = "aws s3 cp --only-show-errors --region us-east-1 --sse AES256"
S3_CI_BUCKET_URL = "s3://dd-ci-artefacts-build-stable/datadog-agent/failed_jobs"
CONSECUTIVE_THRESHOLD = 3
CUMULATIVE_THRESHOLD = 5
CUMULATIVE_LENGTH = 10
CI_VISIBILITY_JOB_URL = 'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20%40ci.job.name%3A{}&agg_m=count'
NOTIFICATION_DISCLAIMER = "If there is something wrong with the notification please contact #agent-developer-experience"


def get_ci_visibility_job_url(name: str, prefix=True) -> str:
    # Escape (https://docs.datadoghq.com/logs/explorer/search_syntax/#escape-special-characters-and-spaces)
    name = re.sub(r"([-+=&|><!(){}[\]^\"“”~*?:\\ ])", r"\\\1", name)

    if prefix:
        name += '*'

    # URL Quote
    name = quote(name)

    return CI_VISIBILITY_JOB_URL.format(name)


@dataclass
class ExecutionsJobInfo:
    job_id: int
    failing: bool = True
    commit: str | None = None

    def url(self):
        return f'{BASE_URL}/DataDog/datadog-agent/-/jobs/{self.job_id}'

    def to_dict(self):
        return {"id": self.job_id, "failing": self.failing, "commit": self.commit}

    @staticmethod
    def ci_visibility_url(name):
        return get_ci_visibility_job_url(name)

    @staticmethod
    def from_dict(data):
        return ExecutionsJobInfo(data["id"], data["failing"], data["commit"])


@dataclass
class ExecutionsJobSummary:
    consecutive_failures: int
    jobs_info: list[ExecutionsJobInfo]

    def to_dict(self):
        return {
            "consecutive_failures": self.consecutive_failures,
            "jobs_info": [info.to_dict() for info in self.jobs_info],
        }

    @staticmethod
    def from_dict(data):
        return ExecutionsJobSummary(
            data["consecutive_failures"],
            [ExecutionsJobInfo.from_dict(failure) for failure in data["jobs_info"]],
        )


class PipelineRuns:
    def __init__(self):
        self.jobs: dict[str, ExecutionsJobSummary] = {}
        self.pipeline_id = 0

    def add_execution(self, name: str, execution: ExecutionsJobSummary):
        self.jobs[name] = execution

    def to_dict(self):
        return {"pipeline_id": self.pipeline_id, "jobs": {name: job.to_dict() for name, job in self.jobs.items()}}

    @staticmethod
    def from_dict(data):
        job_executions = PipelineRuns()
        job_executions.jobs = {name: ExecutionsJobSummary.from_dict(job) for name, job in data["jobs"].items()}
        job_executions.pipeline_id = data.get("pipeline_id", 0)

        return job_executions

    def __repr__(self) -> str:
        return f"Executions({self.to_dict()})"


@dataclass
class CumulativeJobAlert:
    """
    Test that both fails and passes multiple times in few executions
    """

    failures: dict[str, list[ExecutionsJobInfo]]

    def message(self) -> str:
        if len(self.failures) == 0:
            return ''

        job_list = ', '.join(f'<{ExecutionsJobInfo.ci_visibility_url(name)}|{name}>' for name in self.failures)
        message = f'Job(s) {job_list} failed {CUMULATIVE_THRESHOLD} times in last {CUMULATIVE_LENGTH} executions.\n'

        return message


@dataclass
class ConsecutiveJobAlert:
    """
    Test that fails multiple times in a row
    """

    failures: dict[str, list[ExecutionsJobInfo]]

    def message(self, ctx: Context) -> str:
        if len(self.failures) == 0:
            return ''

        # Find initial PR
        initial_pr_sha = next(iter(self.failures.values()))[0].commit
        initial_pr_title = ctx.run(f'git show -s --format=%s {initial_pr_sha}', hide=True).stdout.strip()
        initial_pr_info = get_pr_from_commit(initial_pr_title, PROJECT_TITLE)
        if initial_pr_info:
            pr_id, pr_url = initial_pr_info
            initial_pr = f'<{pr_url}|#{pr_id}>'
        else:
            # Cannot find PR, display the commit sha
            initial_pr = initial_pr_sha[:8]

        return self.format_message(initial_pr)

    def format_message(self, initial_pr: str) -> str:
        job_list = ', '.join(self.failures)
        details = '\n'.join(
            [
                f'- <{ExecutionsJobInfo.ci_visibility_url(name)}|{name}>: '
                + ', '.join(f"<{fail.url()}|{fail.job_id}>" for fail in failures)
                for name, failures in self.failures.items()
            ]
        )
        message = f'Job(s) {job_list} failed {CONSECUTIVE_THRESHOLD} times in a row.\nFirst occurence after merge of {initial_pr}\n{details}\n'

        return message


@task
def check_teams(_):
    if check_for_missing_owners_slack_and_jira():
        print(
            "Error: Some teams in CODEOWNERS don't have their slack notification channel or jira specified!\n"
            "Please specify one in the GITHUB_SLACK_MAP or GITHUB_JIRA_MAP maps in tasks/libs/pipeline/github_slack_map.yaml"
            " or tasks/libs/pipeline/github_jira_map.yaml"
        )
        raise Exit(code=1)
    else:
        print("All CODEOWNERS teams have their slack notification channel and jira project specified !!")


@task
def send_message(ctx, notification_type="merge", print_to_stdout=False):
    """
    Send notifications for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    real slack messages.
    """
    default_branch = os.getenv("CI_DEFAULT_BRANCH")
    git_ref = os.getenv("CI_COMMIT_REF_NAME")

    try:
        failed_jobs = get_failed_jobs(PROJECT_NAME, os.getenv("CI_PIPELINE_ID"))
        messages_to_send = generate_failure_messages(PROJECT_NAME, failed_jobs)
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
            send_dm = not _should_send_message_to_channel(git_ref, default_branch) and all_teams

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


def _should_send_message_to_channel(git_ref: str, default_branch: str) -> bool:
    # Must match X.Y.Z, X.Y.x, W.X.Y-rc.Z
    # Must not match W.X.Y-rc.Z-some-feature
    release_ref_regex = re.compile(r"^[0-9]+\.[0-9]+\.(x|[0-9]+)$")
    release_ref_regex_rc = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]-rc.[0-9]+$")

    return git_ref == default_branch or release_ref_regex.match(git_ref) or release_ref_regex_rc.match(git_ref)


@task
def send_stats(_, print_to_stdout=False):
    """
    Send statistics to Datadog for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    data points to Datadog.
    """
    if not (print_to_stdout or os.environ.get("DD_API_KEY")):
        print("DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend")
        raise Exit(code=1)

    series = compute_failed_jobs_series(PROJECT_NAME)
    series.extend(compute_required_jobs_max_duration(PROJECT_NAME))

    if not print_to_stdout:
        send_metrics(series)
        print(f"Sent pipeline metrics: {series}")
    else:
        print(f"Would send: {series}")


# Tasks to trigger pipeline notifications


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


@task
def check_consistent_failures(ctx, job_failures_file="job_executions.v2.json"):
    # Retrieve the stored document in aws s3. It has the following format:
    # {
    #     "pipeline_id": 123,
    #     "jobs": {
    #         "job1": {"consecutive_failures": 2, "jobs_info": [{"id": null, "failing": false, "commit": "abcdef42"}, {"id": 314618, "failing": true, "commit": "abcdef42"}, {"id": 618314, "failing": true, "commit": "abcdef42"}]},
    #         "job2": {"consecutive_failures": 0, "cumulative_failures": [{"id": 314618, "failing": true, "commit": "abcdef42"}, {"id": null, "failing": false, "commit": "abcdef42"}]},
    #         "job3": {"consecutive_failures": 1, "cumulative_failures": [{"id": 314618, "failing": true, "commit": "abcdef42"}]},
    #     }
    # }
    # NOTE: this format is described by the Executions class
    # The pipeline_id is used to by-pass the check if the pipeline chronological order is not respected
    # The jobs dictionary contains the consecutive and cumulative failures for each job
    # The consecutive failures are reset to 0 when the job is not failing, and are raising an alert when reaching the CONSECUTIVE_THRESHOLD (3)
    # The cumulative failures list contains 1 for failures, 0 for succes. They contain only then CUMULATIVE_LENGTH(10) last executions and raise alert when 50% failure rate is reached

    job_executions = retrieve_job_executions(ctx, job_failures_file)

    # By-pass if the pipeline chronological order is not respected
    if job_executions.pipeline_id > int(os.getenv("CI_PIPELINE_ID")):
        return
    job_executions.pipeline_id = int(os.getenv("CI_PIPELINE_ID"))

    alert_jobs, job_executions = update_statistics(job_executions)

    send_notification(ctx, alert_jobs)

    # Upload document
    with open(job_failures_file, "w") as f:
        json.dump(job_executions.to_dict(), f)
    ctx.run(
        f"{AWS_S3_CP_CMD} {job_failures_file} {S3_CI_BUCKET_URL}/{job_failures_file} ",
        hide="stdout",
    )


def retrieve_job_executions(ctx, job_failures_file):
    """
    Retrieve the stored document in aws s3, or create it
    """
    try:
        ctx.run(
            f"{AWS_S3_CP_CMD}  {S3_CI_BUCKET_URL}/{job_failures_file} {job_failures_file}",
            hide=True,
        )
        with open(job_failures_file) as f:
            job_executions = json.load(f)
        job_executions = PipelineRuns.from_dict(job_executions)
    except UnexpectedExit as e:
        if "404" in e.result.stderr:
            job_executions = create_initial_job_executions(job_failures_file)
        else:
            raise e
    return job_executions


def create_initial_job_executions(job_failures_file):
    job_executions = PipelineRuns()
    with open(job_failures_file, "w") as f:
        json.dump(job_executions.to_dict(), f)
    return job_executions


def update_statistics(job_executions: PipelineRuns):
    consecutive_alerts = {}
    cumulative_alerts = {}

    # Update statistics and collect consecutive failed jobs
    failed_jobs = get_failed_jobs(PROJECT_NAME, os.getenv("CI_PIPELINE_ID"))
    commit_sha = os.getenv("CI_COMMIT_SHA")
    failed_dict = {job.name: ExecutionsJobInfo(job.id, True, commit_sha) for job in failed_jobs.all_failures()}

    # Insert newly failing jobs
    new_failed_jobs = {name for name in failed_dict if name not in job_executions.jobs}
    for job_name in new_failed_jobs:
        job_executions.add_execution(job_name, ExecutionsJobSummary(0, []))

    # Reset information for no-more failing jobs
    solved_jobs = {name for name in job_executions.jobs if name not in failed_dict}
    for job in solved_jobs:
        job_executions.jobs[job].consecutive_failures = 0
        # Append the job without its id
        job_executions.jobs[job].jobs_info.append(ExecutionsJobInfo(None, False))
        # Truncate the cumulative failures list
        if len(job_executions.jobs[job].jobs_info) > CUMULATIVE_LENGTH:
            job_executions.jobs[job].jobs_info.pop(0)

    # Update information for still failing jobs and save them if they hit the threshold
    consecutive_failed_jobs = {name: job for name, job in failed_dict.items() if name in job_executions.jobs}
    for job_name, job in consecutive_failed_jobs.items():
        job_executions.jobs[job_name].consecutive_failures += 1
        job_executions.jobs[job_name].jobs_info.append(job)
        # Truncate the cumulative failures list
        if len(job_executions.jobs[job_name].jobs_info) > CUMULATIVE_LENGTH:
            job_executions.jobs[job_name].jobs_info.pop(0)
        # Save the failed job if it hits the threshold
        if job_executions.jobs[job_name].consecutive_failures == CONSECUTIVE_THRESHOLD:
            consecutive_alerts[job_name] = [job for job in job_executions.jobs[job_name].jobs_info if job.failing]
        if sum(1 for job in job_executions.jobs[job_name].jobs_info if job.failing) == CUMULATIVE_THRESHOLD:
            cumulative_alerts[job_name] = [job for job in job_executions.jobs[job_name].jobs_info if job.failing]

    return {
        'consecutive': consecutive_alerts,
        'cumulative': cumulative_alerts,
    }, job_executions


def send_notification(ctx: Context, alert_jobs, jobowners=".gitlab/JOBOWNERS"):
    def send_alert(channel, consecutive: ConsecutiveJobAlert, cumulative: CumulativeJobAlert):
        message = consecutive.message(ctx) + cumulative.message()
        message = message.strip()

        if message:
            send_slack_message(channel, message)

    all_alerts = set(alert_jobs["consecutive"]) | set(alert_jobs["cumulative"])
    partition = make_partition(all_alerts, jobowners, get_channels=True)

    for channel in partition:
        consecutive = ConsecutiveJobAlert(
            {name: jobs for (name, jobs) in alert_jobs["consecutive"].items() if name in partition[channel]}
        )
        cumulative = CumulativeJobAlert(
            {name: jobs for (name, jobs) in alert_jobs["cumulative"].items() if name in partition[channel]}
        )
        send_alert(channel, consecutive, cumulative)

    # Send all alerts to #agent-platform-ops
    consecutive = ConsecutiveJobAlert(alert_jobs["consecutive"])
    cumulative = CumulativeJobAlert(alert_jobs["cumulative"])
    send_alert('#agent-platform-ops', consecutive, cumulative)


@task
def send_failure_summary_notification(
    _, jobs: dict[str, any] | None = None, allowed_to_fail: bool = False, list_max_len=10, jobowners=".gitlab/JOBOWNERS"
):
    from slack_sdk import WebClient

    def send_summary(channel, stats):
        """
        Send the summary to channel with these job stats
        """
        # Create message
        not_allowed_query = '-' if not allowed_to_fail else ''
        period = 'Daily' if not allowed_to_fail else 'Weekly'
        duration = '24 hours' if not allowed_to_fail else 'week'
        delta = timedelta(days=1) if not allowed_to_fail else timedelta(weeks=1)
        you_own = ' you own' if channel != '#agent-platform-ops' else ''
        flaky_tests = (
            ''
            if allowed_to_fail
            else ' In case of tests, you can <https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3405611398/Flaky+tests+in+go+introducing+flake.Mark|mark them as flaky>.'
        )
        expected_to_fail = 'They are allowed to fail' if allowed_to_fail else 'They are not expected to fail'

        message = []
        for name, fail in stats:
            link = CI_VISIBILITY_JOB_URL.format(quote(name))
            message.append(f"- <{link}|{name}>: *{fail} failures*")

        timestamp_start = int((datetime.now() - delta).timestamp() * 1000)
        timestamp_end = int(datetime.now().timestamp() * 1000)

        header = f'{period} Job Failure Report'
        description = f'These jobs{you_own} had the most failures in the last {duration}:'

        footer = (
            f'{expected_to_fail}. Click <https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20env%3Aprod%20%40git.repository.id%3A%22gitlab.ddbuild.io%2FDataDog%2Fdatadog-agent%22%20%40ci.pipeline.name%3A%22DataDog%2Fdatadog-agent%22%20%40ci.provider.instance%3Agitlab-ci%20%40git.branch%3Amain%20%40ci.status%3Aerror%20%40gitlab.pipeline_source%3A%28push%20OR%20schedule%29%20{not_allowed_query}%40ci.allowed_to_fail%3Atrue&agg_m=count&agg_m_source=base&agg_q=%40ci.job.name&start={timestamp_start}&end={timestamp_end}&agg_q_source=base&agg_t=count&fromUser=false&index=cipipeline&sort_m=count&sort_m_source=base&sort_t=count&top_n=25&top_o=top&viz=toplist&x_missing=true&paused=false|here> for more details.{flaky_tests}\n'
            + NOTIFICATION_DISCLAIMER
        )

        body = '\n'.join(message)
        # Rarely the body may be bigger than 3K characters, split into two messages in this case
        if len(body) >= 3000:
            body = ['\n'.join(message[: len(message) // 2]), '\n'.join(message[len(message) // 2 :])]
        else:
            body = [body]

        blocks = [
            {'type': 'header', 'text': {'type': 'plain_text', 'text': header}},
            {'type': 'section', 'text': {'type': 'mrkdwn', 'text': description}},
            *[{'type': 'section', 'text': {'type': 'mrkdwn', 'text': text}} for text in body],
            {'type': 'context', 'elements': [{'type': 'mrkdwn', 'text': ':information_source: ' + footer}]},
        ]

        # Send message
        client = WebClient(os.environ["SLACK_API_TOKEN"])
        client.chat_postMessage(channel=channel, blocks=blocks)

    # Get args passed by the environment variable
    if jobs is None:
        args = os.environ["ARGS"]
        args = json.loads(args)
        jobs = args['jobs']
        allowed_to_fail = args['allowedToFail']

    # List of (job_name, failure_count) ordered by failure_count
    stats = sorted(
        ((name, data['failures']) for name, data in jobs.items() if data['failures'] > 0),
        key=lambda x: x[1],
        reverse=True,
    )

    # Don't send message if no failure
    if len(stats) == 0:
        return

    # Partition by channels as some teams share the same slack channel (avoid duplicate messages)
    partition = make_partition([name for name, _ in stats], jobowners, get_channels=True)

    # team_stats[team] = [(job_name, failure_count), ...]
    team_stats = {}
    for channel in partition:
        team_stats[channel] = [(name, stat) for (name, stat) in stats if name in partition[channel]]
        team_stats[channel] = team_stats[channel][:list_max_len]

    for channel, stat in team_stats.items():
        send_summary(channel, stat)

    # Send full message to #agent-platform-ops
    send_summary('#agent-platform-ops', stats[:list_max_len])

    print('Messages sent')


@task
def unit_tests(ctx, pipeline_id, pipeline_url, branch_name):
    from tasks.libs.ciproviders.github_api import GithubAPI

    pipeline_id_regex = re.compile(r"pipeline ([0-9]*)")

    jobs_with_no_tests_run = process_unit_tests_tarballs(ctx)
    gh = GithubAPI("DataDog/datadog-agent")
    prs = gh.get_pr_for_branch(branch_name)

    if prs.totalCount == 0:
        # If the branch is not linked to any PR we stop here
        return
    pr = prs[0]

    comment = gh.find_comment(pr.number, "[Fast Unit Tests Report]")
    if comment is None and len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        gh.publish_comment(pr.number, msg)
        return

    if comment is None:
        # If no tests are executed and no previous comment exists, we stop here
        return

    previous_comment_pipeline_id = pipeline_id_regex.findall(comment.body)
    # An older pipeline should not edit a message corresponding to a newer pipeline
    if previous_comment_pipeline_id and previous_comment_pipeline_id[0] > pipeline_id:
        return

    if len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        comment.edit(msg)
    else:
        comment.delete()


def create_msg(pipeline_id, pipeline_url, job_list):
    msg = f"""
[Fast Unit Tests Report]

On pipeline [{pipeline_id}]({pipeline_url}) ([CI Visibility](https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Apipeline%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40ci.pipeline.id%3A{pipeline_id}&fromUser=false&index=cipipeline)). The following jobs did not run any unit tests:

<details>
<summary>Jobs:</summary>

"""
    for job in job_list:
        msg += f"  - {job}\n"
    msg += "</details>\n"
    msg += "\n"
    msg += "If you modified Go files and expected unit tests to run in these jobs, please double check the job logs. If you think tests should have been executed reach out to #agent-developer-experience"
    return msg


def process_unit_tests_tarballs(ctx):
    tarballs = ctx.run("ls junit-tests_*.tgz", hide=True).stdout.split()
    jobs_with_no_tests_run = []
    for tarball in tarballs:
        with tempfile.TemporaryDirectory() as unpack_dir:
            ctx.run(f"tar -xzf {tarball} -C {unpack_dir}")

            # We check if the folder contains at least one junit.xml file. Otherwise we consider no tests were executed
            if not any(f.endswith(".xml") for f in os.listdir(unpack_dir)):
                jobs_with_no_tests_run.append(
                    tarball.replace("junit-", "").replace(".tgz", "").replace("-repacked", "")
                )  # We remove -repacked to have a correct job name macos

    return jobs_with_no_tests_run
