from collections import Counter

from .pipeline_data import get_failed_jobs
from .types import FailedJobType


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

    for job in failed_jobs:
        failure_type = job["failure_type"]
        failure_reason = job["failure_reason"]

        if failure_type == FailedJobType.JOB_FAILURE:
            global_failure_reason = FailedJobType.JOB_FAILURE.name
        elif failure_type == FailedJobType.INFRA_FAILURE and not global_failure_reason:
            global_failure_reason = FailedJobType.INFRA_FAILURE.name

        key = tuple(sorted(job["tag_list"] + [f"type:{failure_type.name}", f"reason:{failure_reason.name}"]))
        job_failure_stats[key] += 1

    return global_failure_reason, job_failure_stats
