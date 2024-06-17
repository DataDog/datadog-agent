from __future__ import annotations

import json
import os
import sys
import traceback
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timedelta, timezone

from gitlab.v4.objects import Project, ProjectPipeline, ProjectPipelineJob
from invoke import Context
from invoke.exceptions import Exit

from tasks.github_tasks import ALL_TEAMS, GITHUB_SLACK_MAP
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.pipeline.data import get_infra_failure_info
from tasks.owners import make_partition

"""
A summary contains a list of jobs from gitlab pipelines.
At the end of each pipeline, a summary is created and uploaded to a file in an s3 bucket (upload_summary).
Every week day, a summary is created for all the pipelines of the last 24 hours (send_summary_messages) out of the summaries on the s3 bucket.
Once a week, a failure summary with allow to fail jobs is sent to the teams (send_summary_messages).
"""

FAILURE_SUMMARY_S3_BUCKET_URL = "s3://dd-ci-persistent-artefacts-build-stable/datadog-agent/failure_summary"
FAILURE_SUMMARY_S3_BUCKET = FAILURE_SUMMARY_S3_BUCKET_URL.split('/')[2]
FAILURE_SUMMARY_S3_PREFIX = '/'.join(FAILURE_SUMMARY_S3_BUCKET_URL.split('/')[3:])
FAILURE_SUMMARY_TMP_FILE = "/tmp/failure_summary.json"


class SummaryData:
    """
    Represents a summary of one pipeline.
    Each summary has his own file based on its timestamp
    """

    @staticmethod
    def list_summaries(ctx: Context, before: int | None = None, after: int | None = None) -> list[int]:
        """
        Returns all the file ids of the summaries
        """
        ids = [SummaryData.get_id(filename) for filename in sorted(list_files(ctx))]

        if before:
            ids = [id for id in ids if id < before]

        if after:
            ids = [id for id in ids if id >= after]

        return ids

    @staticmethod
    def merge(summaries: list[SummaryData]) -> SummaryData:
        summary = SummaryData(ctx=summaries[0].ctx, jobs=[job for summary in summaries for job in summary.jobs])

        # It makes no sense to have an id for a merged summary
        summary.id = None

        return summary

    @staticmethod
    def read(ctx: Context, repo: Project, id: int) -> SummaryData:
        data = read_file(ctx, SummaryData.filename(id))
        data = json.loads(data)
        pipeline = ProjectPipeline(repo.manager, attrs=data['pipeline'])
        jobs = [ProjectPipelineJob(repo.manager, attrs=job) for job in data['jobs']]

        return SummaryData(ctx=ctx, id=id, jobs=jobs, pipeline=pipeline)

    @staticmethod
    def filename(id) -> str:
        return f"{id}.json"

    @staticmethod
    def get_id(filename) -> int:
        return int(filename.split('.')[0])

    def __init__(
        self, ctx: Context, id: int = None, jobs: list[ProjectPipelineJob] = None, pipeline: ProjectPipeline = None
    ):
        self.ctx = ctx
        self.id = id or int(datetime.now(timezone.utc).timestamp())
        self.jobs = jobs or []
        self.pipeline = pipeline

    def write(self):
        write_file(self.ctx, SummaryData.filename(self.id), str(self))

    def as_dict(self) -> dict:
        return {
            'pipeline': None if self.pipeline is None else self.pipeline.asdict(),
            'id': self.id,
            'jobs': [job.asdict() for job in self.jobs],
        }

    def __str__(self) -> str:
        return json.dumps(self.as_dict(), separators=(',', ':'))


@dataclass
class SummaryStats:
    """
    Aggregates and filter jobs to make statistics and produce messages
    """

    data: SummaryData
    allow_failure: bool

    def __post_init__(self):
        # Make summary stats
        total_counter = Counter()
        failure_counter = Counter()
        for job in self.data.jobs:
            # Ignore this job
            if job.allow_failure != self.allow_failure:
                continue

            total_counter.update([job.name])
            if job.status == 'failed':
                failure_counter.update([job.name])

        self.stats = [
            {'name': name, 'failures': failure_counter[name], 'runs': total_counter[name]}
            for name in total_counter.keys()
            if failure_counter[name] > 0
        ]
        # Sort by failures
        self.stats = sorted(self.stats, key=lambda x: x['failures'], reverse=True)

    def make_stats(self, max_length: int = 8, jobowners: str = '.gitlab/JOBOWNERS') -> dict[str, list[dict]]:
        """
        Process stats given self.stats
        Returns dict[team name, list[job stats]]
        """
        # Partition by channels as some teams share the same slack channel (avoid duplicate messages)
        partition = make_partition([s['name'] for s in self.stats], jobowners, get_channels=True)

        # team_stats[channel] = [(job_name, failure_count, total_runs), ...]
        team_stats = {}
        for channel in partition:
            team_stats[channel] = [s for s in self.stats if s['name'] in partition[channel]]
            team_stats[channel] = team_stats[channel][:max_length]

        if self.stats:
            team_stats[GITHUB_SLACK_MAP[ALL_TEAMS]] = self.stats[:max_length]

        return team_stats


def write_file(ctx: Context, name: str, data: str):
    from tasks.notify import AWS_S3_CP_CMD

    with open(FAILURE_SUMMARY_TMP_FILE, 'w') as f:
        f.write(data)

    ctx.run(f"{AWS_S3_CP_CMD} {FAILURE_SUMMARY_TMP_FILE} {FAILURE_SUMMARY_S3_BUCKET_URL}/{name}", hide="stdout")


def read_file(ctx: Context, name: str) -> str:
    from tasks.notify import AWS_S3_CP_CMD

    ctx.run(f"{AWS_S3_CP_CMD} {FAILURE_SUMMARY_S3_BUCKET_URL}/{name} {FAILURE_SUMMARY_TMP_FILE}", hide="stdout")

    with open(FAILURE_SUMMARY_TMP_FILE) as f:
        data = f.read()

    return data


def list_files(ctx: Context) -> list[str]:
    from tasks.notify import AWS_S3_LS_CMD

    listing = ctx.run(
        AWS_S3_LS_CMD.format(bucket=FAILURE_SUMMARY_S3_BUCKET, prefix=FAILURE_SUMMARY_S3_PREFIX), hide="stdout"
    ).stdout
    listing = json.loads(listing)
    listing = [item['Key'].removeprefix(FAILURE_SUMMARY_S3_PREFIX + '/') for item in listing['Contents']]
    listing = [item for item in listing if item.endswith('.json')]

    return listing


def is_valid_job(repo: Project, job: ProjectPipelineJob) -> bool:
    """
    Returns whether the job is finished (failed / success) and if it is not an infrastructure failure
    """
    # Not finished
    if job.status not in ['failed', 'success']:
        return False

    # Ignore infra failures
    if job.status == 'failed':
        trace = str(repo.jobs.get(job.id, lazy=True).trace(), 'utf-8')
        failure_type = get_infra_failure_info(trace)
        if failure_type is not None:
            return False

    return True


def fetch_jobs(ctx: Context, pipeline_id: int) -> SummaryData:
    """
    Returns all the jobs for a given pipeline
    """
    id = int(datetime.now(timezone.utc).timestamp())
    repo = get_gitlab_repo()

    jobs: list[ProjectPipelineJob] = []
    pipeline = repo.pipelines.get(pipeline_id, lazy=True)
    for job in pipeline.jobs.list(per_page=100, all=True):
        if is_valid_job(repo, job):
            jobs.append(job)

    return SummaryData(ctx=ctx, id=id, jobs=jobs, pipeline=pipeline)


def fetch_summaries(ctx: Context, period: timedelta) -> SummaryData:
    """
    Returns all summaries for a given period
    """
    ids = SummaryData.list_summaries(ctx, after=int((datetime.now(timezone.utc) - period).timestamp()))
    repo = get_gitlab_repo()
    summaries = [SummaryData.read(ctx, repo, id) for id in ids]
    # TODO
    print('Found', len(summaries), 'summaries')
    summary = SummaryData.merge(summaries)

    return summary


def upload_summary(ctx: Context, pipeline_id: int) -> SummaryData:
    """
    Creates and uploads a summary for a given pipeline
    """
    summary = fetch_jobs(ctx, pipeline_id)
    summary.write()

    return summary


def send_summary_slack_notification(channel: str, stats: list[dict], allow_failure: bool = False):
    """
    Send the summary to channel with these job stats
    - stats: Item of the dict returned by SummaryStats.make_stats
    """
    # Avoid circular dependency
    from slack_sdk import WebClient

    from tasks.notify import get_ci_visibility_job_url

    # Do not send notification
    if not stats:
        return

    # Create message
    not_allowed_query = '-' if not allow_failure else ''
    period = 'Daily' if not allow_failure else 'Weekly'
    duration = '24 hours' if not allow_failure else 'week'
    delta = timedelta(days=1) if not allow_failure else timedelta(weeks=1)
    you_own = ' you own' if channel != GITHUB_SLACK_MAP[ALL_TEAMS] else ''
    flaky_tests = (
        ''
        if allow_failure
        else ' In case of tests, you can <https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/3405611398/Flaky+tests+in+go+introducing+flake.Mark|mark them as flaky>.'
    )
    expected_to_fail = 'They are allowed to fail' if allow_failure else 'They are not expected to fail'

    message = []
    for stat in stats:
        name = stat['name']
        fail = stat['failures']
        link = get_ci_visibility_job_url(name, prefix=False, extra_flags=['status:error', '-@error.domain:provider'])
        message.append(f"- <{link}|{name}>: *{fail} failures*")

    timestamp_start = int((datetime.now() - delta).timestamp() * 1000)
    timestamp_end = int(datetime.now().timestamp() * 1000)

    header = f'{period} Job Failure Report'
    description = f'These jobs{you_own} had the most failures in the last {duration}:'

    details = 'Click <https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20env%3Aprod%20%40git.repository.id%3A%22gitlab.ddbuild.io%2FDataDog%2Fdatadog-agent%22%20%40ci.pipeline.name%3A%22DataDog%2Fdatadog-agent%22%20%40ci.provider.instance%3Agitlab-ci%20%40git.branch%3Amain%20%40ci.status%3Aerror%20%40gitlab.pipeline_source%3A%28push%20OR%20schedule%29%20{not_allowed_query}%40ci.allowed_to_fail%3Atrue&agg_m=count&agg_m_source=base&agg_q=%40ci.job.name&start={timestamp_start}&end={timestamp_end}&agg_q_source=base&agg_t=count&fromUser=false&index=cipipeline&sort_m=count&sort_m_source=base&sort_t=count&top_n=25&top_o=top&viz=toplist&x_missing=true&paused=false|here> for more details.{flaky_tests}\n{NOTIFICATION_DISCLAIMER}'
    footer = f'{expected_to_fail}. ' + details
    alt_message = f'{header} available. {details}'

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
    # TODO
    # client.chat_postMessage(channel=channel, blocks=blocks)
    client.chat_postMessage(channel='#celian-tests', blocks=blocks, text=alt_message)


def send_summary_messages(
    ctx: Context, allow_failure: bool, max_length: int, period: timedelta, jobowners: str = '.gitlab/JOBOWNERS'
):
    """
    Fetches the summaries for the period and sends messages to all teams having these jobs
    """
    summary = fetch_summaries(ctx, period)
    stats = SummaryStats(summary, allow_failure)

    # TODO : Remove test
    stats.stats.append({'name': 'lint_macos', 'failures': 1, 'runs': 2})

    team_stats = stats.make_stats(max_length, jobowners=jobowners)
    if not team_stats:
        print('No failures found')
        return

    error = False
    for channel, stat in team_stats.items():
        try:
            send_summary_slack_notification(channel=channel, stats=stat, allow_failure=allow_failure)
            print('Notification sent to', channel)
        except Exception:
            print(f"Error sending message to {channel}", file=sys.stderr)
            traceback.print_exc()
            error = True

    print('All notifications sent')

    if error:
        raise Exit('Error sending messages')
