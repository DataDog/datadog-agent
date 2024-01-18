import os

from invoke import Exit, task

from .libs.common.utils import DEFAULT_BRANCH
from .libs.github_actions_tools import (
    download_artifacts,
    download_with_retry,
    follow_workflow_run,
    print_failed_jobs_logs,
    print_workflow_conclusion,
    trigger_macos_workflow,
)
from .release import _get_release_json_value


def _trigger_macos_workflow(release, destination=None, retry_download=0, retry_interval=0, **kwargs):
    github_action_ref = _get_release_json_value(f'{release}::MACOS_BUILD_VERSION')

    run = trigger_macos_workflow(
        github_action_ref=github_action_ref,
        **kwargs,
    )

    workflow_conclusion, workflow_url = follow_workflow_run(run)

    if workflow_conclusion == "failure":
        print_failed_jobs_logs(run)

    print_workflow_conclusion(workflow_conclusion, workflow_url)

    if destination:
        download_with_retry(download_artifacts, run, destination, retry_download, retry_interval)

    if workflow_conclusion != "success":
        raise Exit(code=1)


@task
def trigger_macos_build(
    _,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
    destination=".",
    version_cache=None,
    retry_download=3,
    retry_interval=10,
):
    _trigger_macos_workflow(
        # Provide the release version to be able to fetch the associated
        # macos-build branch from release.json for all workflows...
        release_version,
        destination,
        retry_download,
        retry_interval,
        workflow_name="macos.yaml",
        datadog_agent_ref=datadog_agent_ref,
        # ... And provide the release version as a workflow input when needed
        release_version=release_version,
        major_version=major_version,
        python_runtimes=python_runtimes,
        # Send pipeline id and bucket branch so that the package version
        # can be constructed properly for nightlies.
        gitlab_pipeline_id=os.environ.get("CI_PIPELINE_ID", None),
        bucket_branch=os.environ.get("BUCKET_BRANCH", None),
        version_cache_file_content=version_cache,
    )


@task
def trigger_macos_test(
    _,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    python_runtimes="3",
    destination=".",
    version_cache=None,
    retry_download=3,
    retry_interval=10,
):
    _trigger_macos_workflow(
        release_version,
        destination,
        retry_download,
        retry_interval,
        workflow_name="test.yaml",
        datadog_agent_ref=datadog_agent_ref,
        python_runtimes=python_runtimes,
        version_cache_file_content=version_cache,
    )


@task
def trigger_macos_lint(
    _,
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    python_runtimes="3",
    version_cache=None,
):
    _trigger_macos_workflow(
        release_version,
        workflow_name="lint.yaml",
        datadog_agent_ref=datadog_agent_ref,
        python_runtimes=python_runtimes,
        version_cache_file_content=version_cache,
    )


@task
def lint_codeowner(_):
    """
    Check every package in `pkg` has an owner
    """

    base = os.path.dirname(os.path.abspath(__file__))
    root_folder = os.path.join(base, "..")
    os.chdir(root_folder)

    owners = _get_code_owners(root_folder)

    # make sure each root package has an owner
    pkgs_without_owner = _find_packages_without_owner(owners, "pkg")
    if len(pkgs_without_owner) > 0:
        raise Exit(
            f'The following packages  in `pkg` directory don\'t have an owner in CODEOWNERS: {pkgs_without_owner}',
            code=1,
        )


def _find_packages_without_owner(owners, folder):
    pkg_without_owners = []
    for x in os.listdir(folder):
        path = os.path.join("/" + folder, x)
        if path not in owners:
            pkg_without_owners.append(path)
    return pkg_without_owners


def _get_code_owners(root_folder):
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


@task
def get_milestone_id(_, milestone):
    # Local import as github isn't part of our default set of installed
    # dependencies, and we don't want to propagate it to files importing this one
    from libs.common.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')
    m = gh.get_milestone_by_name(milestone)
    if not m:
        raise Exit(f'Milestone {milestone} wasn\'t found in the repo', code=1)
    print(m.number)
