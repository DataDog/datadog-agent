from collections import Counter

from tasks.libs.pipeline.data import get_failed_jobs
from tasks.libs.types.types import FailedJobType


def get_failed_jobs_stats(project_name, pipeline_id):
    """
    Returns a dictionary containing statistics on the reasons why these
    jobs failed and the global reason the pipeline failed.
    """

    # Prepare hash of stats for job failure reasons (to publish stats to the Datadog backend)
    # Format:
    # job_failure_stats: {
    #   ("type:failure_type", "reason:failure_reason", "runner:runner_type"): 3,
    #   ...
    # }
    job_failure_stats = Counter()

    failed_jobs = get_failed_jobs(project_name, pipeline_id)

    # This stores the reason why a pipeline ultimately failed.
    # The goal is to have a statistic of the number of pipelines that fail
    # only due to infrastructure failures.
    global_failure_reason = None

    if failed_jobs.mandatory_job_failures:
        global_failure_reason = FailedJobType.JOB_FAILURE.name
    elif failed_jobs.mandatory_infra_job_failures:
        global_failure_reason = FailedJobType.INFRA_FAILURE.name

    for job in failed_jobs.all_mandatory_failures():
        failure_type = job["failure_type"]
        failure_reason = job["failure_reason"]

        key = tuple(sorted(job["tag_list"] + [f"type:{failure_type.name}", f"reason:{failure_reason.name}"]))
        job_failure_stats[key] += 1

    return global_failure_reason, job_failure_stats
