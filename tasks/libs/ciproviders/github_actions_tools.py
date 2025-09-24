import os
import re
import sys
import tempfile
import zipfile
from datetime import datetime
from time import sleep

from invoke.exceptions import Exit

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message


def trigger_windows_bump_workflow(
    repo="ci-platform-machine-images",
    workflow_name="windows-runner-agent-bump.yml",
    github_action_ref="main",
    new_version=None,
):
    """
    Trigger a workflow to bump windows gitlab runner
    """
    inputs = {}
    if new_version is not None:
        inputs["new-version"] = new_version

    print(
        "Creating workflow on {} on ref {} with args:\n{}".format(
            repo, github_action_ref, "\n".join([f"  - {k}: {inputs[k]}" for k in inputs])
        )
    )

    # Hack: get current time to only fetch workflows that started after now
    now = datetime.utcnow()

    gh = GithubAPI(f"DataDog/{repo}")
    result = gh.trigger_workflow(workflow_name, github_action_ref, inputs)

    if not result:
        print(f"Couldn't trigger workflow run. result={result}")
        raise Exit(code=1)

    # Since we can't get the worflow run id from a `create_dispatch` api call we are fetching the first running workflow after `now`.
    recent_runs = gh.workflow_run_for_ref_after_date(workflow_name, github_action_ref, now)
    MAX_RETRY = 10
    while not recent_runs and MAX_RETRY > 0:
        MAX_RETRY -= 1
        sleep(3)
        recent_runs = gh.workflow_run_for_ref_after_date(workflow_name, github_action_ref, now)

    if not recent_runs:
        print("Couldn't get the run workflow")
        raise Exit(code=1)

    return recent_runs[0]


def follow_workflow_run(run, repository, interval=5):
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
            github = GithubAPI(repository)
            run = github.workflow_run(run.id)
        except GithubException as e:
            failures += 1
            print(f"Workflow run not found, retrying in 15 seconds (failure {failures}/{MAX_FAILURES})")
            print("Error: ", e)
            if failures == MAX_FAILURES:
                raise Exit(code=1) from e
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

        minutes += interval
        sleep(60 * interval)


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
        try:
            print("\n".join(parse_log_file(log_file)))
        except FileNotFoundError:
            print(f'Failed to parse {log_file}, file does not exist')

    print(color_message("Logs might be incomplete, for complete logs check directly in the worklow logs", "blue"))
    print(color_message("Workflow run link: " + color_message(run.html_url, "green"), "blue"))


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

    with open(log_file) as f:
        lines = f.readlines()
        for line_number, line in enumerate(lines):
            if error_regex.search(line):
                return lines[line_number:]


def download_artifacts(run, destination, repository):
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
        workflow = GithubAPI(repository)
        for artifact in run_artifacts:
            # Download artifact
            print("Downloading artifact: ", artifact)
            zip_path = workflow.download_artifact(artifact, tmpdir)

            # Unzip it in the target destination
            with zipfile.ZipFile(zip_path, "r") as zip_ref:
                zip_ref.extractall(destination)


def download_logs(run, destination, repo):
    """
    Download all logs for the given run
    """
    print(color_message(f"Downloading logs for run {run.id}", "blue"))

    with tempfile.TemporaryDirectory() as tmpdir:
        workflow = GithubAPI(repo)

        zip_path = workflow.download_logs(run.id, tmpdir)
        # Unzip it in the target destination
        with zipfile.ZipFile(zip_path, "r") as zip_ref:
            zip_ref.extractall(destination)


def download_with_retry(
    download_function,
    run,
    destination=".",
    retry_count=3,
    retry_interval=10,
    repository=None,
):
    import requests

    retry = retry_count

    assert repository is not None, "Repository must be provided for download_with_retry"

    while retry > 0:
        try:
            download_function(run, destination, repository)
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
