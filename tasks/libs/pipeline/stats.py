import os
import traceback
from collections import Counter
from datetime import datetime

from invoke import Exit

from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo
from tasks.libs.common.datadog_api import create_count, create_gauge
from tasks.libs.pipeline.data import get_failed_jobs
from tasks.libs.types.types import FailedJobType

REQUIRED_JOBS = [
    "go_mod_tidy_check",
    "tests_deb-x64-py3",
    "tests_deb-arm64-py3",
    "tests_rpm-x64-py3",
    "tests_rpm-arm64-py3",
    "slack_teams_channels_check",
    "lint_windows-x64",
    "lint_linux-x64",
    "lint_linux-arm64",
    "lint_flavor_iot_linux-x64",
    "lint_flavor_dogstatsd_linux-x64",
    "tests_ebpf_arm64",
    "tests_ebpf_x64",
    "do-not-merge",
    "agent_deb-x64-a7",
    "agent_rpm-x64-a7",
    "agent_suse-x64-a7",
    "deploy_deb_testing-a7_x64",
    "deploy_suse_rpm_testing_x64-a7",
    "new-e2e-agent-platform-install-script-debian-a7-x86_64",
    "new-e2e-agent-platform-install-script-suse-a7-x86_64",
    "new-e2e-agent-platform-install-script-ubuntu-a7-x86_64",
    "new-e2e-agent-platform-install-script-debian-iot-agent-a7-x86_64",
    "agent_heroku_deb-x64-a7",
    "iot_agent_deb-x64",
    "dogstatsd_deb-x64",
    "iot_agent_rpm-x64",
    "dogstatsd_rpm-x64",
    "security_go_generate_check",
    "lint_python",
]


def compute_failed_jobs_series(project_name: str):
    try:
        global_failure_reason, job_failure_stats = get_failed_jobs_stats(project_name, os.getenv("CI_PIPELINE_ID"))
    except Exception as e:
        print("Found exception when generating statistics:")
        print(e)
        traceback.print_exc(limit=-1)
        raise Exit(code=1)

    timestamp = int(datetime.now().timestamp())
    series = []

    for failure_tags, count in job_failure_stats.items():
        # This allows getting stats on the number of jobs that fail due to infrastructure
        # issues vs. other failures, and have a per-pipeline ratio of infrastructure failures.
        series.append(
            create_count(
                metric_name="datadog.ci.job_failures",
                timestamp=timestamp,
                value=count,
                tags=list(failure_tags)
                + [
                    "repository:datadog-agent",
                    f"git_ref:{os.getenv('CI_COMMIT_REF_NAME')}",
                ],
            )
        )

    # Consider the pipeline state as failed if at least one job failed
    pipeline_state = "failed" if job_failure_stats else "succeeded"

    pipeline_tags = [
        "repository:datadog-agent",
        f"git_ref:{os.getenv('CI_COMMIT_REF_NAME')}",
        f"status:{pipeline_state}",
    ]
    if global_failure_reason:  # Only set the reason if the pipeline fails
        pipeline_tags.append(f"reason:{global_failure_reason}")

    series.append(
        create_count(
            metric_name="datadog.ci.pipelines",
            timestamp=timestamp,
            value=1,
            tags=pipeline_tags,
        )
    )
    return series


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
        failure_type = job.failure_type
        failure_reason = job.failure_reason

        key = tuple(sorted(job.tag_list + [f"type:{failure_type.name}", f"reason:{failure_reason.name}"]))
        job_failure_stats[key] += 1

    return global_failure_reason, job_failure_stats


def compute_required_jobs_max_duration(project_name: str):
    timestamp = int(datetime.now().timestamp())
    series = []
    max_duration, status = get_max_duration(project_name=project_name)
    pipeline_tags = [
        "repository:datadog-agent",
        f"git_ref:{os.getenv('CI_COMMIT_REF_NAME')}",
        f"status:{status}",
    ]
    series.append(
        create_gauge(
            metric_name="datadog.ci.required_job.duration",
            timestamp=timestamp,
            value=max_duration,
            tags=pipeline_tags,
        )
    )

    return series


def get_max_duration(project_name: str):
    datadog_agent = get_gitlab_repo(repo=project_name)
    pipeline = datadog_agent.pipelines.get(os.getenv("CI_PIPELINE_ID"))
    start = datetime.fromisoformat(pipeline.created_at[:-1])
    max = start
    status = "success"
    jobs = pipeline.jobs.list(per_page=100, all=True)
    for job in jobs:
        if job.name in REQUIRED_JOBS and job.finished_at is not None:
            finished = datetime.fromisoformat(job.finished_at[:-1])
            if finished > max:
                max = finished
            if job.status != "success":
                status = job.status
    return (max - start).seconds, status
