import os
import re
import time
from functools import lru_cache

from invoke import Exit, task

from tasks.libs.common.utils import DEFAULT_BRANCH, get_git_pretty_ref
from tasks.libs.datadog_api import create_count, send_metrics
from tasks.libs.github_actions_tools import (
    download_artifacts,
    download_with_retry,
    follow_workflow_run,
    print_failed_jobs_logs,
    print_workflow_conclusion,
    trigger_macos_workflow,
)
from tasks.libs.junit_upload_core import repack_macos_junit_tar
from tasks.release import _get_release_json_value
from tasks.libs.pipeline_notifications import read_owners


@lru_cache(maxsize=None)
def concurrency_key():
    current_ref = get_git_pretty_ref()

    # We want workflows to run to completion on the default branch and release branches
    if re.search(rf'^({DEFAULT_BRANCH}|\d+\.\d+\.x)$', current_ref):
        return None

    return current_ref


def _trigger_macos_workflow(release, destination=None, retry_download=0, retry_interval=0, **kwargs):
    github_action_ref = _get_release_json_value(f'{release}::MACOS_BUILD_VERSION')

    run = trigger_macos_workflow(
        github_action_ref=github_action_ref,
        concurrency_key=concurrency_key(),
        **kwargs,
    )

    workflow_conclusion, workflow_url = follow_workflow_run(run)

    if workflow_conclusion == "failure":
        print_failed_jobs_logs(run)

    print_workflow_conclusion(workflow_conclusion, workflow_url)

    if destination:
        download_with_retry(download_artifacts, run, destination, retry_download, retry_interval)

    return workflow_conclusion


@task
def trigger_macos(
    _,
    workflow_type="build",
    datadog_agent_ref=DEFAULT_BRANCH,
    release_version="nightly-a7",
    major_version="7",
    python_runtimes="3",
    destination=".",
    version_cache=None,
    retry_download=3,
    retry_interval=10,
):
    if workflow_type == "build":
        conclusion = _trigger_macos_workflow(
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
    elif workflow_type == "test":
        conclusion = _trigger_macos_workflow(
            release_version,
            destination,
            retry_download,
            retry_interval,
            workflow_name="test.yaml",
            datadog_agent_ref=datadog_agent_ref,
            python_runtimes=python_runtimes,
            version_cache_file_content=version_cache,
        )
        repack_macos_junit_tar(conclusion, "junit-tests_macos.tgz", "junit-tests_macos-repacked.tgz")
    elif workflow_type == "lint":
        conclusion = _trigger_macos_workflow(
            release_version,
            workflow_name="lint.yaml",
            datadog_agent_ref=datadog_agent_ref,
            python_runtimes=python_runtimes,
            version_cache_file_content=version_cache,
        )
    if conclusion != "success":
        raise Exit(message=f"Macos {workflow_type} workflow {conclusion}", code=1)


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


@task
def send_rate_limit_info_datadog(_, pipeline_id):
    from .libs.common.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')
    rate_limit_info = gh.get_rate_limit_info()
    print(f"Remaining rate limit: {rate_limit_info[0]}/{rate_limit_info[1]}")
    metric = create_count(
        metric_name='github.rate_limit.remaining',
        timestamp=int(time.time()),
        value=rate_limit_info[0],
        tags=['source:github', 'repository:datadog-agent', f'pipeline_id:{pipeline_id}'],
    )
    send_metrics([metric])


@task
def get_token_from_app(_, app_id_env='GITHUB_APP_ID', pkey_env='GITHUB_KEY_B64'):
    from .libs.common.github_api import GithubAPI

    GithubAPI.get_token_from_app(app_id_env, pkey_env)


@task
def assign_team_label(_, pr_id, changed_files):
    """
    Assigns the github team label name if a single team can
    be deduced from the changed files.
    changed_files is a comma separated string
    """
    from .libs.common.github_api import GithubAPI

    def get_team() -> str | None:
        codeowners = read_owners('.github/CODEOWNERS')

        global_team = None
        for file in changed_files:
            owners = [name for (kind, name) in codeowners.of(file) if kind == 'TEAM']
            if len(owners) != 1:
                # Multiple / no owners case
                return
            else:
                file_team = owners[0]
                if global_team is not None and file_team != global_team:
                    # Multiple owners case
                    return
                else:
                    global_team = file_team

        return global_team

    gh = GithubAPI('DataDog/datadog-agent')

    # Skip if necessary
    labels = gh.get_labels()

    if 'qa/done' in labels:
        print('Qa done, skipping')
        return

    if any(label.startswith('team/') for label in labels):
        print('This PR already has a team label, skipping')
        return

    # Find team
    changed_files = changed_files.split(',')
    team = get_team()
    if team is None:
        print('No team or multiple teams found')
        return

    # Assign label
    label_name = 'team' + team.removeprefix('@DataDog')
    if gh.add_label(pr_id, label_name):
        print(label_name, 'label assigned to the pull request')
    else:
        print('Failed to assign label')
