import os
import re
import sys
import tempfile
import uuid
import zipfile
from datetime import datetime
from time import sleep

from invoke.exceptions import Exit

from ..utils import DEFAULT_BRANCH
from .common.color import color_message
from .common.github_api import GithubAPI


def trigger_macos_workflow(
    workflow_name="macos.yaml",
    github_action_ref="master",
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version=None,
    major_version=None,
    python_runtimes="3",
    gitlab_pipeline_id=None,
    bucket_branch=None,
    version_cache_file_content=None,
):
    """
    Trigger a workflow to build a MacOS Agent.
    """
    inputs = {}

    if datadog_agent_ref is not None:
        inputs["datadog_agent_ref"] = datadog_agent_ref

    if release_version is not None:
        inputs["release_version"] = release_version

    if major_version is not None:
        inputs["agent_major_version"] = major_version

    if python_runtimes is not None:
        inputs["python_runtimes"] = python_runtimes

    if gitlab_pipeline_id is not None:
        inputs["gitlab_pipeline_id"] = gitlab_pipeline_id

    if bucket_branch is not None:
        inputs["bucket_branch"] = bucket_branch

    if version_cache_file_content:
        inputs["version_cache"] = version_cache_file_content

    # The workflow trigger endpoint doesn't return anything. You need to fetch the workflow run id
    # by yourself.
    workflow_id = str(uuid.uuid1())
    inputs["id"] = workflow_id

    print(
        "Creating workflow on datadog-agent-macos-build on commit {} with args:\n{}".format(  # noqa: FS002
            github_action_ref, "\n".join([f"  - {k}: {inputs[k]}" for k in inputs])
        )
    )
    # Hack: get current time to only fetch workflows that started after now.
    now = datetime.utcnow()

    gh = GithubAPI('DataDog/datadog-agent-macos-build')
    gh.trigger_workflow(workflow_name, github_action_ref, inputs)

    # Thus the following hack: Send an id as input when creating a workflow on Github. The worklow will use the id and put it in the name of one of its jobs.
    # We then fetch workflows and check if it contains the id in its job name.

    MAX_RETRIES = 10  # Retry up to 10 times
    for i in range(MAX_RETRIES):
        print(f"Fetching triggered workflow (try {i + 1}/{MAX_RETRIES})")
        recent_runs = gh.workflow_run_for_ref_after_date(workflow_name, github_action_ref, now)
        for recent_run in recent_runs:
            jobs = recent_run.jobs()
            if jobs.totalCount >= 2:
                for job in jobs:
                    if any([step.name == workflow_id for step in job.steps]):
                        return recent_run
            else:
                print("waiting for jobs to popup...")
                sleep(3)
        sleep(5)

    # Something went wrong :(
    print("Couldn't fetch workflow run that was triggered.")
    raise Exit(code=1)


def follow_workflow_run(run):
    """
    Follow the workflow run until completion and return its conclusion.
    """
    # Imported from here since only x86_64 images ship this module
    from github import GithubException

    print(color_message("Workflow run link: " + color_message(run.html_url, "green"), "blue"))

    minutes = 0
    failures = 0
    MAX_FAILURES = 5
    while True:
        # Do not fail outright for temporary failures
        try:
            github = GithubAPI('DataDog/datadog-agent-macos-build')
            run = github.workflow_run(run.id)
        except GithubException:
            failures += 1
            print(f"Workflow run not found, retrying in 15 seconds (failure {failures}/{MAX_FAILURES})")
            if failures == MAX_FAILURES:
                raise Exit(code=1)
            sleep(15)
            continue

        status = run.status
        conclusion = run.conclusion
        run_url = run.html_url

        if status == "completed":
            return conclusion, run_url
        else:
            print(f"Workflow still running... ({minutes}m)")
            # For some unknown reason, in Gitlab these lines do not get flushed, leading to not being
            # able to see where's the job at in the logs. The following line forces the flush.
            sys.stdout.flush()

        minutes += 1
        sleep(60)


def print_workflow_conclusion(conclusion, workflow_uri):
    """
    Print the workflow conclusion
    """
    if conclusion == "success":
        print(color_message("Workflow run succeeded", "green"))
    else:
        print(color_message(f"Workflow run ended with state: {conclusion}", "red"))
        print(f"Failed workflow URI {workflow_uri}")


def print_failed_jobs_logs(run):
    """
    Retrieves, parses and print failed job logs for the workflow run
    """
    failed_jobs = get_failed_jobs(run)

    download_with_retry(download_logs, run, destination="workflow_logs")

    failed_steps = get_failed_steps_log_files("workflow_logs", failed_jobs)

    for failed_step, log_file in failed_steps.items():
        print(color_message(f"Step: {failed_step} failed:", "red"))
        print("\n".join(parse_log_file(log_file)))
    print(color_message("Logs might be incomplete, for complete logs check directly in the worklow logs", "blue"))


def get_failed_jobs(run):
    """
    Retrieves failed jobs for the workflow run
    """
    return [job for job in run.jobs() if job.conclusion == "failure"]


def get_failed_steps_log_files(log_dir, failed_jobs):
    failed_steps_log_files = {}
    for failed_job in failed_jobs:
        for step in failed_job.steps:
            if step.conclusion == "failure":
                failed_steps_log_files[step.name] = f"{log_dir}/{failed_job.name}/{step.number}_{step.name}.txt"
    return failed_steps_log_files


def parse_log_file(log_file):
    """
    Parse log file and return relevant line to print in GitlabCI logs.
    The function will iterate over the log file, and check a line matching the following criteria:
        - line containing [error]
        - line containing Linter|Test failures
        - line containing Traceback
    When such a line is found, the line is returned with all the lines after it
    """

    error_regex = re.compile(r'\[error\]|(Linter|Test) failures|Traceback')

    with open(log_file, 'r') as f:
        lines = f.readlines()
        for line_number, line in enumerate(lines):
            if error_regex.search(line):
                return lines[line_number:]


def download_artifacts(run, destination="."):
    """
    Download all artifacts for a given job in the specified location.
    """
    print(color_message(f"Downloading artifacts for run {run.id} to {destination}", "blue"))
    run_artifacts = run.get_artifacts()
    if run_artifacts is None:
        print("Workflow run not found.")
        raise Exit(code=1)
    if run_artifacts.totalCount == 0:
        raise ConnectionError

    # Create temp directory to store the artifact zips
    with tempfile.TemporaryDirectory() as tmpdir:
        workflow = GithubAPI('DataDog/datadog-agent-macos-build')
        for artifact in run_artifacts:
            # Download artifact
            print("Downloading artifact: ", artifact)
            zip_path = workflow.download_artifact(artifact, tmpdir)

            # Unzip it in the target destination
            with zipfile.ZipFile(zip_path, "r") as zip_ref:
                zip_ref.extractall(destination)


def download_logs(run, destination="."):
    """
    Download all logs for the given run
    """
    print(color_message(f"Downloading logs for run {run.id}", "blue"))

    with tempfile.TemporaryDirectory() as tmpdir:
        workflow = GithubAPI('DataDog/datadog-agent-macos-build')

        zip_path = workflow.download_logs(run.id, tmpdir)
        # Unzip it in the target destination
        with zipfile.ZipFile(zip_path, "r") as zip_ref:
            zip_ref.extractall(destination)


def download_with_retry(download_function, run, destination=".", retry_count=3, retry_interval=10):
    import requests

    retry = retry_count

    while retry > 0:
        try:
            download_function(run, destination)
            print(color_message(f"Download successful for run {run.id} to {destination}", "blue"))
            return
        except (requests.exceptions.RequestException, ConnectionError):
            retry -= 1
            print(f'Connectivity issue while downloading, retrying... {retry} attempts left')
            sleep(retry_interval)
        except Exception as e:
            print("Exception that is not a connectivity issue: ", type(e).__name__, " - ", e)
            raise e
    print(f'Download failed {retry_count} times, stop retry and exit')
    raise Exit(code=os.EX_TEMPFAIL)
