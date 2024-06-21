from __future__ import annotations

import json
import os
from dataclasses import dataclass
from datetime import datetime, timezone

from invoke.context import Context
from invoke.exceptions import UnexpectedExit

from tasks.libs.ciproviders.gitlab_api import BASE_URL
from tasks.libs.common.datadog_api import create_count, send_metrics
from tasks.libs.notify.utils import AWS_S3_CP_CMD, PROJECT_NAME, PROJECT_TITLE, get_ci_visibility_job_url
from tasks.libs.pipeline.data import get_failed_jobs
from tasks.libs.pipeline.notifications import (
    get_pr_from_commit,
    send_slack_message,
)
from tasks.owners import channel_owners, make_partition

"""
Alerts are notifications sent to each team channel when a job fails multiple times in a row or
multiple times in a few executions.
"""

ALERTS_S3_CI_BUCKET_URL = "s3://dd-ci-artefacts-build-stable/datadog-agent/failed_jobs"

CONSECUTIVE_THRESHOLD = 3
CUMULATIVE_THRESHOLD = 5
CUMULATIVE_LENGTH = 10


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
        return get_ci_visibility_job_url(name, extra_flags=['status:error', '-@error.domain:provider'])

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


def retrieve_job_executions(ctx, job_failures_file):
    """
    Retrieve the stored document in aws s3, or create it
    """
    try:
        ctx.run(
            f"{AWS_S3_CP_CMD} {ALERTS_S3_CI_BUCKET_URL}/{job_failures_file} {job_failures_file}",
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
        nonlocal metrics

        message = consecutive.message(ctx) + cumulative.message()
        message = message.strip()

        if message:
            send_slack_message(channel, message)

            # Create metrics for consecutive and cumulative alerts
            metrics += [
                create_count(
                    metric_name=f"datadog.ci.failed_job_alerts.{alert_type}",
                    timestamp=timestamp,
                    tags=[f"team:{team}", "repository:datadog-agent"],
                    unit="notification",
                    value=len(failures),
                )
                for alert_type, failures in (("consecutive", consecutive.failures), ("cumulative", cumulative.failures))
                for team in channel_owners(channel)
                if len(failures) > 0
            ]

    metrics = []
    timestamp = int(datetime.now(timezone.utc).timestamp())
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

    if metrics:
        send_metrics(metrics)
        print('Metrics sent')


def upload_job_executions(ctx, job_executions: PipelineRuns, job_failures_file: str):
    with open(job_failures_file, "w") as f:
        json.dump(job_executions.to_dict(), f)
    ctx.run(
        f"{AWS_S3_CP_CMD} {job_failures_file} {ALERTS_S3_CI_BUCKET_URL}/{job_failures_file}",
        hide="stdout",
    )
