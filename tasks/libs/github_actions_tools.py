import sys
import tempfile
import zipfile
from datetime import datetime, timedelta
from time import sleep

from invoke.exceptions import Exit

from ..utils import DEFAULT_BRANCH
from .common.color import color_message
from .common.github_workflows import GithubException, GithubWorkflows, get_github_app_token


def create_or_refresh_macos_build_github_workflows(github_workflows=None):
    # If no token or token is going to expire, refresh token
    if (
        github_workflows is None
        or datetime.utcnow() + timedelta(minutes=5) > github_workflows.api_token_expiration_date
    ):
        token, expiration_date = get_github_app_token()
        return GithubWorkflows(
            repository="DataDog/datadog-agent-macos-build", api_token=token, api_token_expiration_date=expiration_date
        )
    return github_workflows


def trigger_macos_workflow(
    workflow="macos.yaml",
    github_action_ref="master",
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version=None,
    major_version=None,
    python_runtimes="3",
    gitlab_pipeline_id=None,
    bucket_branch=None,
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

    print(
        "Creating workflow on datadog-agent-macos-build on commit {} with args:\n{}".format(  # noqa: FS002
            github_action_ref, "\n".join([f"  - {k}: {inputs[k]}" for k in inputs])
        )
    )

    # Hack: get current time to only fetch workflows that started after now.
    now = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")

    # The workflow trigger endpoint doesn't return anything. You need to fetch the workflow run id
    # by yourself.
    create_or_refresh_macos_build_github_workflows().trigger_workflow(workflow, github_action_ref, inputs)

    # Thus the following hack: query the latest run for ref, wait until we get a non-completed run
    # that started after we triggered the workflow.
    # In practice, this should almost never be a problem, even if the Agent 6 and 7 jobs run at the
    # same time, given that these two jobs will target different github_action_ref on RCs / releases.
    MAX_RETRIES = 10  # Retry up to 10 times
    for i in range(MAX_RETRIES):
        print(f"Fetching triggered workflow (try {i + 1}/{MAX_RETRIES})")
        run = get_macos_workflow_run_for_ref(workflow, github_action_ref)
        if run is not None and run.get("created_at", datetime.fromtimestamp(0).strftime("%Y-%m-%dT%H:%M:%SZ")) >= now:
            return run.get("id")

        sleep(5)

    # Something went wrong :(
    print("Couldn't fetch workflow run that was triggered.")
    raise Exit(code=1)


def get_macos_workflow_run_for_ref(workflow="macos.yaml", github_action_ref="master"):
    """
    Get the latest workflow for the given ref.
    """
    return create_or_refresh_macos_build_github_workflows().latest_workflow_run_for_ref(workflow, github_action_ref)


def follow_workflow_run(run_id):
    """
    Follow the workflow run until completion.
    """

    try:
        github_workflows = create_or_refresh_macos_build_github_workflows()
        run = github_workflows.workflow_run(run_id)
    except GithubException:
        raise Exit(code=1)

    if run is None:
        print("Workflow run not found.")
        raise Exit(code=1)

    print(color_message("Workflow run link: " + color_message(run["html_url"], "green"), "blue"))

    minutes = 0
    failures = 0
    MAX_FAILURES = 5
    while True:
        # Do not fail outright for temporary failures
        try:
            github_workflows = create_or_refresh_macos_build_github_workflows(github_workflows)
            run = github_workflows.workflow_run(run_id)
        except GithubException:
            failures += 1
            print(f"Workflow run not found, retrying in 15 seconds (failure {failures}/{MAX_FAILURES})")
            if failures == MAX_FAILURES:
                raise Exit(code=1)
            sleep(15)
            continue

        status = run["status"]
        conclusion = run["conclusion"]

        if status == "completed":
            if conclusion == "success":
                print(color_message("Workflow run succeeded", "green"))
                return
            else:
                print(color_message(f"Workflow run ended with state: {conclusion}", "red"))
                raise Exit(code=1)
        else:
            print(f"Workflow still running... ({minutes}m)")
            # For some unknown reason, in Gitlab these lines do not get flushed, leading to not being
            # able to see where's the job at in the logs. The following line forces the flush.
            sys.stdout.flush()

        minutes += 1
        sleep(60)


def download_artifacts(run_id, destination="."):
    """
    Download all artifacts for a given job in the specified location.
    """
    print(color_message(f"Downloading artifacts for run {run_id} to {destination}", "blue"))

    github_workflows = create_or_refresh_macos_build_github_workflows()
    run_artifacts = github_workflows.workflow_run_artifacts(run_id)
    if run_artifacts is None:
        print("Workflow run not found.")
        raise Exit(code=1)

    # Create temp directory to store the artifact zips
    with tempfile.TemporaryDirectory() as tmpdir:
        for artifact in run_artifacts["artifacts"]:
            # Download artifact
            github_workflows = create_or_refresh_macos_build_github_workflows(github_workflows)
            zip_path = github_workflows.download_artifact(artifact["id"], tmpdir)

            # Unzip it in the target destination
            with zipfile.ZipFile(zip_path, "r") as zip_ref:
                zip_ref.extractall(destination)
