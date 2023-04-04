import os

from invoke import Exit, task

from .libs.github_actions_tools import (
    download_artifacts,
    follow_workflow_run,
    print_workflow_conclusion,
    trigger_macos_workflow,
)
from .utils import DEFAULT_BRANCH, load_release_versions


@task
def trigger_macos_build(
    ctx,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
    destination=".",
):
    env = load_release_versions(ctx, release_version)
    github_action_ref = env["MACOS_BUILD_VERSION"]

    run_id = trigger_macos_workflow(
        workflow="macos.yaml",
        github_action_ref=github_action_ref,
        datadog_agent_ref=datadog_agent_ref,
        release_version=release_version,
        major_version=major_version,
        python_runtimes=python_runtimes,
        # Send pipeline id and bucket branch so that the package version
        # can be constructed properly for nightlies.
        gitlab_pipeline_id=os.environ.get("CI_PIPELINE_ID", None),
        bucket_branch=os.environ.get("BUCKET_BRANCH", None),
    )

    workflow_conclusion = follow_workflow_run(run_id)

    print_workflow_conclusion(workflow_conclusion)

    download_artifacts(run_id, destination)

    if workflow_conclusion != "success":
        raise Exit(code=1)


@task
def trigger_macos_test(
    ctx,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    python_runtimes="3",
    destination=".",
):
    env = load_release_versions(ctx, release_version)
    github_action_ref = env["MACOS_BUILD_VERSION"]

    run_id = trigger_macos_workflow(
        workflow="test.yaml",
        github_action_ref=github_action_ref,
        datadog_agent_ref=datadog_agent_ref,
        python_runtimes=python_runtimes,
    )

    workflow_conclusion = follow_workflow_run(run_id)

    print_workflow_conclusion(workflow_conclusion)

    download_artifacts(run_id, destination)

    if workflow_conclusion != "success":
        raise Exit(code=1)


@task
def lint_codeowner(_):
    """
    Check every package in `pkg` has an owner
    """

    base = os.path.dirname(os.path.abspath(__file__))
    root_folder = os.path.join(base, "..")
    os.chdir(root_folder)

    owners = get_code_owners(root_folder)

    # make sure each root package has an owner
    pkgs_without_owner = find_packages_without_owner(owners, "pkg")
    if len(pkgs_without_owner) > 0:
        raise Exit(
            f'The following packages  in `pkg` directory don\'t have an owner in CODEOWNERS: {pkgs_without_owner}',
            code=1,
        )


def find_packages_without_owner(owners, folder):
    pkg_without_owners = []
    for x in os.listdir(folder):
        path = os.path.join("/" + folder, x)
        if path not in owners:
            pkg_without_owners.append(path)
    return pkg_without_owners


def get_code_owners(root_folder):
    code_owner_path = os.path.join(root_folder, ".github", "CODEOWNERS")
    owners = {}
    with open(code_owner_path) as f:
        for line in f:
            line = line.strip()
            line = line.split("#")[0]  # remove comment
            if len(line) > 0:
                parts = line.split()
                path = os.path.normpath(parts[0])
                # example /tools/retry_file_dump ['@DataDog/agent-metrics-logs']
                owners[path] = parts[1:]
    return owners
