from __future__ import annotations

import json
from datetime import timedelta, datetime

from gitlab.v4.objects import ProjectPipelineJob, Project

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.pipeline.data import get_infra_failure_info


class SummaryData:
    """
    Represents a summary of one pipeline.
    Each summary has his own file based on its timestamp
    """

    @staticmethod
    def read(repo: Project, id: int) -> SummaryData:
        data = read_file(SummaryData.filename(id))
        jobs = [ProjectPipelineJob(repo.manager, attrs=json.loads(job)) for job in data.split('\n')]

        return SummaryData(id=id, jobs=jobs)

    def __init__(self, id: int = None, jobs: list[ProjectPipelineJob] = None):
        self.id = id or datetime.now().timestamp()
        self.jobs = jobs or []

    def write(self):
        write_file(SummaryData.filename(self.id), str(self))

    @staticmethod
    def filename(id) -> str:
        return f"{id}.json"

    def __str__(self) -> str:
        return '\n'.join([job.to_json(separators=(',', ':')) for job in self.jobs])


# TODO : s3
def write_file(name: str, data: str):
    with open('/tmp/summary/' + name, 'w') as f:
        f.write(data)


def read_file(name: str) -> str:
    with open('/tmp/summary/' + name) as f:
        return f.read()


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


def fetch_jobs(pipeline_id: int) -> SummaryData:
    """
    Returns all the jobs for a given pipeline
    """
    id = datetime.now().timestamp()
    repo = get_gitlab_repo()

    jobs: list[ProjectPipelineJob] = []
    pipeline = repo.pipelines.get(pipeline_id, lazy=True)
    for job in pipeline.jobs.list(per_page=100, all=True):
        if is_valid_job(repo, job):
            jobs.append(job)

    return SummaryData(id=id, jobs=jobs)


# TODO : Make stats
def fetch_summaries(period: timedelta) -> list[SummaryData]:
    """
    Returns all summaries for a given period
    """
    pass


def upload_summary(pipeline_id: int):
    """
    Creates and uploads a summary for a given pipeline
    """
    pass


def clean_summaries(period: timedelta):
    """
    Will remove summaries older than this period
    """
    pass


def test():
    summary = fetch_jobs(36500940)
    id = datetime(2024, 1, 1).timestamp()
    summary.id = id
    summary.write()

    print()
    summary = SummaryData.read(get_gitlab_repo(), id)
    print(summary)
    print(len(summary.jobs))


