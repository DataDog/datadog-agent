import os

import gitlab
from invoke.tasks import task

from . import doc


@task(
    help={
        'pipeline-id': doc.pipeline_id,
        'job-name': doc.job_name,
    }
)
def retry_job(_, pipeline_id, job_name):
    """
    Retry gitlab pipeline job
    """
    agent, _, job = _get_job(pipeline_id, job_name)

    print(f'Retrying job {job_name} ({job.id})...')
    new_job = agent.jobs.get(job.id).retry()
    print(
        f'Job {job_name} retried, see status at https://gitlab.ddbuild.io/DataDog/datadog-agent/-/jobs/{new_job["id"]}'
    )


def _get_job(pipeline_id, job_name):
    """
    Get gitlab job of DataDog/datadog-agent from pipeline and name
    Returns (repository, pipeline, job)
    """
    gitlab_token = os.environ['GITLAB_TOKEN']

    gl = gitlab.Gitlab('https://gitlab.ddbuild.io', private_token=gitlab_token)
    agent_repo = gl.projects.get('DataDog/datadog-agent')
    pipeline = agent_repo.pipelines.get(pipeline_id)

    jobs = pipeline.jobs.list(all=True, per_page=100)

    # Latest job first by default
    job = [j for j in jobs if j.name == job_name]
    assert len(job) >= 1, f'Cannot find job {job_name}'
    job = job[0]

    return agent_repo, pipeline, job
