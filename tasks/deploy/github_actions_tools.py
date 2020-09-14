import tempfile
import zipfile
from datetime import datetime
from time import sleep

from invoke.exceptions import Exit

from .color import color_message
from .github import Github


def trigger_macos_workflow(
    github_action_ref="master",
    datadog_agent_ref="master",
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
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

    print(
        "Creating workflow on datadog-agent-macos-build on commit {} with args:\n{}".format(
            github_action_ref, "\n".join(["  - {}: {}".format(k, inputs[k]) for k in inputs])
        )
    )

    # Hack: get current time to only fetch workflows that started after now.
    now = datetime.utcnow().strftime("%Y-%m-%dT%H:%M:%SZ")

    # The workflow trigger endpoint doesn't return anything. You need to fetch the workflow run id
    # by yourself.
    res = Github().trigger_workflow("DataDog/datadog-agent-macos-build", "macos.yaml", github_action_ref, inputs)
    print(res)
    # Thus the following hack: query the latest run for ref, wait until we get a non-completed run
    # that started after we triggered the workflow
    retries = 1
    MAX_RETRIES = 10
    while retries <= MAX_RETRIES:
        print("Fetching triggered workflow (try {}/{})".format(retries, MAX_RETRIES))
        run = get_macos_workflow_run_for_ref(github_action_ref)
        if run is not None and run.get("created_at", datetime.fromtimestamp(0).strftime("%Y-%m-%dT%H:%M:%SZ")) >= now:
            return run.get("id")

        retries += 1
        sleep(5)

    # Something went wrong :(
    print("Couldn't fetch workflow run that was triggered.")
    raise Exit(code=1)


def get_macos_workflow_run_for_ref(github_action_ref="master"):
    """
    Get the latest workflow for the given ref.
    """
    return Github().latest_workflow_run_for_ref("DataDog/datadog-agent-macos-build", "macos.yaml", github_action_ref)


def follow_workflow_run(run_id):
    """
    Follow the workflow run until completion.
    """

    run = Github().workflow_run("DataDog/datadog-agent-macos-build", run_id)
    if run is None:
        print("Workflow run not found.")
        raise Exit(code=1)

    print(color_message("Workflow run link: " + color_message(run["html_url"], "green",), "blue",))

    while True:
        run = Github().workflow_run("DataDog/datadog-agent-macos-build", run_id)

        status = run["status"]
        conclusion = run["conclusion"]

        if status == "completed":
            if conclusion == "success":
                print(color_message("Workflow run succeeded", "green",))
                return
            else:
                print(color_message("Workflow run ended with state: {}".format(conclusion), "red",))
                raise Exit(code=1)
        else:
            print("Workflow still running...")

        sleep(60)


def download_artifacts(run_id, destination="."):
    """
    Gets links to the zip files containing the artifacts.
    """

    print(color_message("Downloading artifacts for run {} to {}".format(run_id, destination), "blue"))

    run_artifacts = Github().workflow_run_artifacts("DataDog/datadog-agent-macos-build", run_id)
    if run_artifacts is None:
        print("Workflow run not found.")
        raise Exit(code=1)

    # Create temp directory to store the artifact zips
    with tempfile.TemporaryDirectory() as tmpdir:
        for artifact in run_artifacts["artifacts"]:
            # Download artifact
            zip_path = Github().download_artifact("DataDog/datadog-agent-macos-build", artifact["id"], tmpdir)

            # Unzip it in the target destination
            with zipfile.ZipFile(zip_path, "r") as zip_ref:
                zip_ref.extractall(destination)
