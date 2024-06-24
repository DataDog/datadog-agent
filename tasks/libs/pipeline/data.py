import re
from collections import defaultdict

from gitlab.v4.objects import ProjectJob

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.types.types import FailedJobReason, FailedJobs, FailedJobType


def get_failed_jobs(project_name: str, pipeline_id: str) -> FailedJobs:
    """
    Retrieves the list of failed jobs for a given pipeline id in a given project.
    """
    repo = get_gitlab_repo(project_name)
    pipeline = repo.pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(per_page=100, all=True)

    # Get instances of failed jobs grouped by name
    failed_jobs = defaultdict(list)
    for job in jobs:
        if job.status == "failed":
            failed_jobs[job.name].append(job)

    # There, we now have the following map:
    # job name -> list of jobs with that name, including at least one failed job
    processed_failed_jobs = FailedJobs()
    for job_name, jobs in failed_jobs.items():
        # We sort each list per creation date
        jobs.sort(key=lambda x: x.created_at)
        # We truncate the job name to increase readability
        job_name = truncate_job_name(job_name)
        job = jobs[-1]
        # Check the final job in the list: it contains the current status of the job
        # This excludes jobs that were retried and succeeded
        trace = str(repo.jobs.get(job.id, lazy=True).trace(), 'utf-8')
        failure_type, failure_reason = get_job_failure_context(job, trace)
        final_status = ProjectJob(
            repo.manager,
            attrs={
                "name": job_name,
                "id": job.id,
                "stage": job.stage,
                "status": job.status,
                "tag_list": job.tag_list,
                "allow_failure": job.allow_failure,
                "web_url": job.web_url,
                "retry_summary": [ijob.status for ijob in jobs],
                "failure_type": failure_type,
                "failure_reason": failure_reason,
            },
        )

        # Also exclude jobs allowed to fail
        if final_status.status == "failed" and should_report_job(job_name, final_status.allow_failure):
            processed_failed_jobs.add_failed_job(final_status)

    return processed_failed_jobs


infra_failure_logs = [
    # Gitlab errors while pulling image on legacy runners
    (re.compile(r'no basic auth credentials \(.*\)'), FailedJobReason.RUNNER),
    (re.compile(r'net/http: TLS handshake timeout \(.*\)'), FailedJobReason.RUNNER),
    # docker / docker-arm runner init failures
    (re.compile(r'Docker runner job start script failed'), FailedJobReason.RUNNER),
    (
        re.compile(
            r'A disposable runner accepted this job, while it shouldn\'t have\. Runners are meant to run just one job and be terminated\.'
        ),
        FailedJobReason.RUNNER,
    ),
    (
        re.compile(r'WARNING: Failed to pull image with policy "always":.*\(.*\)'),
        FailedJobReason.RUNNER,
    ),
    # k8s Gitlab runner init failures
    (
        re.compile(
            r'Job failed \(system failure\): prepare environment: waiting for pod running: timed out waiting for pod to start'
        ),
        FailedJobReason.RUNNER,
    ),
    (
        re.compile(
            r'Job failed \(system failure\): prepare environment: setting up build pod: Internal error occurred: failed calling webhook'
        ),
        FailedJobReason.RUNNER,
    ),
    # kitchen tests Azure VM allocation failures
    (
        re.compile(
            r'Allocation failed\. We do not have sufficient capacity for the requested VM size in this region\.'
        ),
        FailedJobReason.KITCHEN_AZURE,
    ),
    # Gitlab 5xx errors
    (
        re.compile(r'fatal: unable to access \'.*\': The requested URL returned error: 5..'),
        FailedJobReason.GITLAB,
    ),
    # kitchen tests general infrastructure issues
    (
        re.compile(r'ERROR: The kitchen tests failed due to infrastructure failures\.'),
        FailedJobReason.KITCHEN,
    ),
    # End to end tests EC2 Spot instances allocation failures
    (
        re.compile(r'Failed to allocate end to end test EC2 Spot instance after [0-9]+ attempts'),
        FailedJobReason.EC2_SPOT,
    ),
    (
        re.compile(r'Connection to [0-9]+\.[0-9]+\.[0-9]+\.[0-9]+ closed by remote host\.'),
        FailedJobReason.EC2_SPOT,
    ),
    # End to end tests internal infrastructure failures
    (
        re.compile(r'E2E INTERNAL ERROR'),
        FailedJobReason.E2E_INFRA_FAILURE,
    ),
]


def get_infra_failure_info(job_log: str):
    for regex, type in infra_failure_logs:
        if regex.search(job_log):
            return type


def get_job_failure_context(job: ProjectJob, job_log: str):
    """
    Parses job logs (provided as a string), and returns the type of failure (infra or job) as well
    as the precise reason why the job failed.
    """

    infra_failure_reasons = FailedJobReason.get_infra_failure_mapping().keys()

    if job.failure_reason in infra_failure_reasons:
        return FailedJobType.INFRA_FAILURE, FailedJobReason.from_gitlab_job_failure_reason(job.failure_reason)

    type = get_infra_failure_info(job_log)
    if type:
        return FailedJobType.INFRA_FAILURE, type

    return FailedJobType.JOB_FAILURE, FailedJobReason.FAILED_JOB_SCRIPT


# These jobs are allowed to have their full name in the data.
# They are often matrix/parallel jobs where the dimension values are important.
jobs_allowed_to_have_full_names = [re.compile(r'kmt_run_.+_tests_.*')]


def truncate_job_name(job_name, max_char_per_job=48):
    if any(pattern.fullmatch(job_name) for pattern in jobs_allowed_to_have_full_names):
        return job_name

    # Job header should be before the colon, if there is no colon this won't change job_name
    truncated_job_name = job_name.split(":")[0]
    # We also want to avoid it being too long
    truncated_job_name = truncated_job_name[:max_char_per_job]
    return truncated_job_name


# Those jobs have `allow_failure: true` but still need to be included
# in failure reports
jobs_allowed_to_fail_but_need_report = [re.compile(r'kitchen_test_security_agent.*')]


def should_report_job(job_name, allow_failure):
    return not allow_failure or any(pattern.fullmatch(job_name) for pattern in jobs_allowed_to_fail_but_need_report)
