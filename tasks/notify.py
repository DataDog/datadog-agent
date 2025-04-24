import json
import os
import re
import tempfile
import traceback
from datetime import datetime

from invoke import task
from invoke.exceptions import Exit, UnexpectedExit

from tasks.libs.datadog_api import create_count, send_metrics
from tasks.libs.pipeline_data import get_failed_jobs
from tasks.libs.pipeline_notifications import check_for_missing_owners_slack_and_jira, send_slack_message
from tasks.libs.pipeline_stats import get_failed_jobs_stats

PROJECT_NAME = "DataDog/datadog-agent"
AWS_S3_CP_CMD = "aws s3 cp --only-show-errors --region us-east-1 --sse AES256"
S3_CI_BUCKET_URL = "s3://dd-ci-artefacts-build-stable/datadog-agent/failed_jobs"
CONSECUTIVE_THRESHOLD = 3
CUMULATIVE_THRESHOLD = 5
CUMULATIVE_LENGTH = 10


@task
def check_teams(_):
    if check_for_missing_owners_slack_and_jira():
        print(
            "Error: Some teams in CODEOWNERS don't have their slack notification channel or jira specified!\n"
            "Please specify one in the GITHUB_SLACK_MAP or GITHUB_JIRA_MAP maps in tasks/libs/github_slack_map.yaml"
            " or tasks/libs/github_jira_map.yaml"
        )
        raise Exit(code=1)
    else:
        print("All CODEOWNERS teams have their slack notification channel and jira project specified !!")


@task
def send_stats(_, print_to_stdout=False):
    """
    Send statistics to Datadog for the current pipeline. CI-only task.
    Use the --print-to-stdout option to test this locally, without sending
    data points to Datadog.
    """
    try:
        global_failure_reason, job_failure_stats = get_failed_jobs_stats(PROJECT_NAME, os.getenv("CI_PIPELINE_ID"))
    except Exception as e:
        print("Found exception when generating statistics:")
        print(e)
        traceback.print_exc(limit=-1)
        raise Exit(code=1)

    if not (print_to_stdout or os.environ.get("DD_API_KEY")):
        print("DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend")
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

    if job_failure_stats:  # At least one job failed
        pipeline_state = "failed"
    else:
        pipeline_state = "succeeded"

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

    if not print_to_stdout:
        response = send_metrics(series)
        if response["errors"]:
            print(f"Error(s) while sending pipeline metrics to the Datadog backend: {response['errors']}")
            raise Exit(code=1)
        print(f"Sent pipeline metrics: {series}")
    else:
        print(f"Would send: {series}")


@task
def check_consistent_failures(ctx, job_failures_file="job_executions.json"):
    # Retrieve the stored document in aws s3. It has the following format:
    # {
    #     "pipeline_id": 123,
    #     "jobs": {
    #         "job1": {"consecutive_failures": 2, "cumulative_failures": [0, 0, 0, 1, 0, 1, 1, 0, 1, 1]},
    #         "job2": {"consecutive_failures": 0, "cumulative_failures": [1, 0, 0, 0, 0, 0, 0, 0, 0, 0]},
    #         "job3": {"consecutive_failures": 1, "cumulative_failures": [1]},
    #     }
    # }
    # The pipeline_id is used to by-pass the check if the pipeline chronological order is not respected
    # The jobs dictionary contains the consecutive and cumulative failures for each job
    # The consecutive failures are reset to 0 when the job is not failing, and are raising an alert when reaching the CONSECUTIVE_THRESHOLD (3)
    # The cumulative failures list contains 1 for failures, 0 for succes. They contain only then CUMULATIVE_LENGTH(10) last executions and raise alert when 50% failure rate is reached
    job_executions = retrieve_job_executions(ctx, job_failures_file)

    # By-pass if the pipeline chronological order is not respected
    if job_executions.get("pipeline_id", 0) > int(os.getenv("CI_PIPELINE_ID")):
        return
    job_executions["pipeline_id"] = int(os.getenv("CI_PIPELINE_ID"))

    alert_jobs, job_executions = update_statistics(job_executions)

    send_notification(alert_jobs)

    # Upload document
    with open(job_failures_file, "w") as f:
        json.dump(job_executions, f)
    ctx.run(f"{AWS_S3_CP_CMD} {job_failures_file} {S3_CI_BUCKET_URL}/{job_failures_file} ", hide="stdout")


def retrieve_job_executions(ctx, job_failures_file):
    """
    Retrieve the stored document in aws s3, or create it
    """
    try:
        ctx.run(f"{AWS_S3_CP_CMD}  {S3_CI_BUCKET_URL}/{job_failures_file} {job_failures_file}", hide=True)
        with open(job_failures_file) as f:
            job_executions = json.load(f)
    except UnexpectedExit as e:
        if "404" in e.result.stderr:
            job_executions = create_initial_job_executions(job_failures_file)
        else:
            raise e
    return job_executions


def create_initial_job_executions(job_failures_file):
    job_executions = {"pipeline_id": 0, "jobs": {}}
    with open(job_failures_file, "w") as f:
        json.dump(job_executions, f)
    return job_executions


def update_statistics(job_executions):
    # Update statistics and collect consecutive failed jobs
    alert_jobs = {"consecutive": [], "cumulative": []}
    failed_jobs = get_failed_jobs(PROJECT_NAME, os.getenv("CI_PIPELINE_ID"))
    failed_set = {job["name"] for job in failed_jobs.all_failures()}
    current_set = set(job_executions["jobs"].keys())
    # Insert data for newly failing jobs
    new_failed_jobs = failed_set - current_set
    for job in new_failed_jobs:
        job_executions["jobs"][job] = {"consecutive_failures": 1, "cumulative_failures": [1]}
    # Reset information for no-more failing jobs
    solved_jobs = current_set - failed_set
    for job in solved_jobs:
        job_executions["jobs"][job]["consecutive_failures"] = 0
        job_executions["jobs"][job]["cumulative_failures"].append(0)
        # Truncate the cumulative failures list
        if len(job_executions["jobs"][job]["cumulative_failures"]) > CUMULATIVE_LENGTH:
            job_executions["jobs"][job]["cumulative_failures"].pop(0)
    # Update information for still failing jobs and save them if they hit the threshold
    consecutive_failed_jobs = failed_set & current_set
    for job in consecutive_failed_jobs:
        job_executions["jobs"][job]["consecutive_failures"] += 1
        job_executions["jobs"][job]["cumulative_failures"].append(1)
        # Truncate the cumulative failures list
        if len(job_executions["jobs"][job]["cumulative_failures"]) > CUMULATIVE_LENGTH:
            job_executions["jobs"][job]["cumulative_failures"].pop(0)
        # Save the failed job if it hits the threshold
        if job_executions["jobs"][job]["consecutive_failures"] == CONSECUTIVE_THRESHOLD:
            alert_jobs["consecutive"].append(job)
        if sum(job_executions["jobs"][job]["cumulative_failures"]) == CUMULATIVE_THRESHOLD:
            alert_jobs["cumulative"].append(job)
    return alert_jobs, job_executions


def send_notification(alert_jobs):
    message = ""
    if len(alert_jobs["consecutive"]) > 0:
        jobs = ", ".join(f"`{j}`" for j in alert_jobs["consecutive"])
        message += f"Job(s) {jobs} failed {CONSECUTIVE_THRESHOLD} times in a row.\n"
    if len(alert_jobs["cumulative"]) > 0:
        jobs = ", ".join(f"`{j}`" for j in alert_jobs["cumulative"])
        message += f"Job(s) {jobs} failed {CUMULATIVE_THRESHOLD} times in last {CUMULATIVE_LENGTH} executions.\n"
    if message:
        send_slack_message("#agent-agent6-ops", message)


@task
def unit_tests(ctx, pipeline_id, pipeline_url, branch_name):
    from tasks.libs.common.github_api import GithubAPI

    pipeline_id_regex = re.compile(r"pipeline ([0-9]*)")

    jobs_with_no_tests_run = process_unit_tests_tarballs(ctx)
    gh = GithubAPI("DataDog/datadog-agent")
    prs = gh.get_pr_for_branch(branch_name)

    if prs.totalCount == 0:
        # If the branch is not linked to any PR we stop here
        return
    pr = prs[0]

    comment = gh.find_comment(pr.number, "[Fast Unit Tests Report]")
    if comment is None and len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        gh.publish_comment(pr.number, msg)
        return

    if comment is None:
        # If no tests are executed and no previous comment exists, we stop here
        return

    previous_comment_pipeline_id = pipeline_id_regex.findall(comment.body)
    # An older pipeline should not edit a message corresponding to a newer pipeline
    if previous_comment_pipeline_id and previous_comment_pipeline_id[0] > pipeline_id:
        return

    if len(jobs_with_no_tests_run) > 0:
        msg = create_msg(pipeline_id, pipeline_url, jobs_with_no_tests_run)
        comment.edit(msg)
    else:
        comment.delete()


def create_msg(pipeline_id, pipeline_url, job_list):
    msg = f'''
[Fast Unit Tests Report]

On pipeline [{pipeline_id}]({pipeline_url}) ([CI Visibility](https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Apipeline%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40ci.pipeline.id%3A{pipeline_id}&fromUser=false&index=cipipeline)). The following jobs did not run any unit tests:

<details>
<summary>Jobs:</summary>

'''
    for job in job_list:
        msg += f"  - {job}\n"
    msg += "</details>\n"
    msg += "\n"
    msg += "If you modified Go files and expected unit tests to run in these jobs, please double check the job logs. If you think tests should have been executed reach out to <#C06PBHLD4DQ>"
    return msg


def process_unit_tests_tarballs(ctx):
    tarballs = ctx.run("ls junit-tests_*.tgz", hide=True).stdout.split()
    jobs_with_no_tests_run = []
    for tarball in tarballs:
        with tempfile.TemporaryDirectory() as unpack_dir:
            ctx.run(f"tar -xzf {tarball} -C {unpack_dir}")

            # We check if the folder contains at least one junit.xml file. Otherwise we consider no tests were executed
            if not any(f.endswith(".xml") for f in os.listdir(unpack_dir)):
                jobs_with_no_tests_run.append(
                    tarball.replace("junit-", "").replace(".tgz", "").replace("-repacked", "")
                )  # We remove -repacked to have a correct job name macos

    return jobs_with_no_tests_run
