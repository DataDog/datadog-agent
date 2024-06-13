from __future__ import annotations

import json
import os
from collections import Counter
from dataclasses import dataclass
from datetime import datetime, timedelta

from gitlab.v4.objects import Project, ProjectPipeline, ProjectPipelineJob
from invoke import Context

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.pipeline.data import get_infra_failure_info

"""
A summary contains a list of jobs from gitlab pipelines.
"""


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
        ids = [SummaryData.get_id(filename) for filename in list_files(ctx)]

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
        self.id = id or int(datetime.now().timestamp())
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

    def make_message(self, stats: list[dict]) -> str | None:
        """
        Creates a message from the stats that are already processed
        """
        if not stats:
            return

        # TODO : Format etc...
        return '\n'.join(f'- {s["name"]}: {s["failures"]}/{s["runs"]}' for s in stats)

    def make_stats(self, max_length: int = 8, team: str | None = None) -> list[dict]:
        """
        Process stats given self.stats
        """

        # TODO : Filter by team

        # Sort by failures
        stats = sorted(self.stats, key=lambda x: x['failures'], reverse=True)
        stats = stats[:max_length]

        return stats


# TODO : s3
def write_file(ctx: Context, name: str, data: str):
    # TODO
    print('Writing file', name)

    with open('/tmp/summary/' + name, 'w') as f:
        f.write(data)


def read_file(ctx: Context, name: str) -> str:
    # TODO
    print('Reading file', name)

    with open('/tmp/summary/' + name) as f:
        return f.read()


def remove_files(ctx: Context, names: list[str]):
    # TODO
    print('Removing files', names)

    os.system(f'rm -f /tmp/summary/{{{",".join(names)}}}')


def list_files(ctx: Context) -> list[str]:
    return os.listdir('/tmp/summary')


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
    id = int(datetime.now().timestamp())
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
    ids = SummaryData.list_summaries(ctx, after=int((datetime.now() - period).timestamp()))
    summaries = [SummaryData.read(ctx, get_gitlab_repo(), id) for id in ids]
    summary = SummaryData.merge(summaries)

    return summary


def upload_summary(ctx: Context, pipeline_id: int):
    """
    Creates and uploads a summary for a given pipeline
    """
    summary = fetch_jobs(ctx, pipeline_id)
    summary.write()


def clean_summaries(ctx: Context, period: timedelta):
    """
    Will remove summaries older than this period
    """
    ids = SummaryData.list_summaries(ctx, before=int((datetime.now() - period).timestamp()))
    remove_files(ctx, [SummaryData.filename(id) for id in ids])


def send_summary_messages(ctx: Context, allow_failure: bool, max_length: int, period: timedelta):
    """
    Fetches the summaries for the period and sends messages to all teams having these jobs
    """
    summary = fetch_summaries(ctx, period)
    stats = SummaryStats(summary, allow_failure)

    # TODO : Send
    # TODO : Dispatch to teams (rm team=None)
    team = None
    team_stats = stats.make_stats(max_length, team=team)
    msg = stats.make_message(team_stats)

    print()
    print('* TO:', team)
    print(msg)


# TODO : rm
def test(ctx: Context):
    s = fetch_summaries(ctx, timedelta(days=999))
    stats = SummaryStats(s, allow_failure=True)
    print(stats.make_message(stats.make_stats(16)))

    return

    # repo = get_gitlab_repo()
    # pipeline = repo.pipelines.get(36500940)
    # print(json.dumps({'pipeline': pipeline.asdict()}, indent=2))
    # return

    upload_summary(ctx, 36500940)
    upload_summary(ctx, 36560009)

    # summary = fetch_jobs(ctx, 36500940)
    # id = int(datetime(2024, 1, 1).timestamp())
    # summary.id = id
    # summary.write()

    # summary2 = fetch_jobs(ctx, 36560009)
    # id2 = int(datetime(2024, 2, 1).timestamp())
    # summary2.id = id2
    # summary2.write()

    print()
    print(SummaryData.list_summaries(ctx))
    print(SummaryData.list_summaries(ctx, after=datetime(2024, 1, 15).timestamp()))
    print(SummaryData.list_summaries(ctx, before=datetime(2024, 1, 15).timestamp()))

    # print()
    # summary = SummaryData.read(get_gitlab_repo(), id)
    # print(summary)
    # print(len(summary.jobs))
