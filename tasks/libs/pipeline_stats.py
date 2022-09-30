from collections import Counter, defaultdict

from .pipeline_data import get_failed_jobs


def get_failed_jobs_stats(project_name, pipeline_id):
    """
    Returns a dictionary containing statistics on the reasons why these
    jobs failed.
    """

    # Prepare hash of stats for job failure reasons (to publish stats to the Datadog backend)
    # Format:
    # job_failure_stats: {
    #   "job_failure_type_1": {
    #     "job_failure_reason_1": 3,
    #     ...
    #   }
    # }
    job_failure_stats = defaultdict(Counter)

    failed_jobs = get_failed_jobs(project_name, pipeline_id)

    for job in failed_jobs:
        job_failure_stats[job["failure_type"]][job["failure_reason"]] += 1

    return job_failure_stats
