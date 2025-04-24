import os
import re
import time
from collections import Counter
from functools import lru_cache
from typing import List

from invoke import Exit, task

from tasks.libs.common.utils import DEFAULT_BRANCH, DEFAULT_INTEGRATIONS_CORE_BRANCH, RELEASE_JSON_DEPENDENCIES, get_git_pretty_ref
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
from tasks.libs.pipeline_notifications import read_owners
from tasks.release import _get_release_json_value


@lru_cache(maxsize=None)
def concurrency_key():
    current_ref = get_git_pretty_ref()

    # We want workflows to run to completion on the default branch and release branches
    if re.search(rf'^({DEFAULT_BRANCH}|\d+\.\d+\.x)$', current_ref):
        return None

    return current_ref


def _trigger_macos_workflow(destination=None, retry_download=0, retry_interval=0, **kwargs):
    github_action_ref = _get_release_json_value(f'{RELEASE_JSON_DEPENDENCIES}::MACOS_BUILD_VERSION')

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
    major_version="7",
    python_runtimes="3",
    destination=".",
    version_cache=None,
    retry_download=3,
    retry_interval=10,
    fast_tests=None,
    integrations_core_ref=DEFAULT_INTEGRATIONS_CORE_BRANCH,
):
    if workflow_type == "build":
        conclusion = _trigger_macos_workflow(
            destination,
            retry_download,
            retry_interval,
            workflow_name="macos.yaml",
            datadog_agent_ref=datadog_agent_ref,
            major_version=major_version,
            python_runtimes=python_runtimes,
            # Send pipeline id and bucket branch so that the package version
            # can be constructed properly for nightlies.
            gitlab_pipeline_id=os.environ.get("CI_PIPELINE_ID", None),
            bucket_branch=os.environ.get("BUCKET_BRANCH", None),
            version_cache_file_content=version_cache,
            integrations_core_ref=integrations_core_ref,
        )
    elif workflow_type == "test":
        conclusion = _trigger_macos_workflow(
            destination,
            retry_download,
            retry_interval,
            workflow_name="test.yaml",
            datadog_agent_ref=datadog_agent_ref,
            python_runtimes=python_runtimes,
            version_cache_file_content=version_cache,
            fast_tests=fast_tests,
        )
        repack_macos_junit_tar(conclusion, "junit-tests_macos.tgz", "junit-tests_macos-repacked.tgz")
    elif workflow_type == "lint":
        conclusion = _trigger_macos_workflow(
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

    gh = GithubAPI()
    m = gh.get_milestone_by_name(milestone)
    if not m:
        raise Exit(f'Milestone {milestone} wasn\'t found in the repo', code=1)
    print(m.number)


@task
def send_rate_limit_info_datadog(_, pipeline_id):
    from .libs.common.github_api import GithubAPI

    gh = GithubAPI()
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


def _get_teams(changed_files, owners_file='.github/CODEOWNERS') -> List[str]:
    codeowners = read_owners(owners_file)

    team_counter = Counter()
    for file in changed_files:
        owners = [name for (kind, name) in codeowners.of(file) if kind == 'TEAM']
        team_counter.update(owners)

    team_count = team_counter.most_common()
    if team_count == []:
        return []

    _, best_count = team_count[0]
    best_teams = [team.casefold() for (team, count) in team_count if count == best_count]

    return best_teams


def _get_team_labels():
    import toml

    with open('.ddqa/config.toml', 'r') as f:
        data = toml.loads(f.read())

    labels = []
    for team in data['teams'].values():
        labels.extend(team.get('github_labels', []))
    return labels


@task
def assign_team_label(_, pr_id=-1):
    """
    Assigns the github team label name if teams can
    be deduced from the changed files
    """
    import github

    from tasks.libs.common.github_api import GithubAPI

    gh = GithubAPI('DataDog/datadog-agent')

    labels = gh.get_pr_labels(pr_id)

    # Skip if necessary
    if 'qa/done' in labels or 'qa/no-code-change' in labels:
        print('Qa done or no code change, skipping')
        return

    if any(label.startswith('team/') for label in labels):
        print('This PR already has a team label, skipping')
        return

    # Find team
    teams = _get_teams(gh.get_pr_files(pr_id))
    if teams == []:
        print('No team found')
        return

    # Get labels
    all_team_labels = _get_team_labels()
    team_labels = [f"team{team.removeprefix('@datadog')}" for team in teams]

    # Assign label
    for label_name in team_labels:
        if label_name not in all_team_labels:
            print(label_name, 'cannot be found in .ddqa/config.toml, skipping')
        else:
            try:
                gh.add_pr_label(pr_id, label_name)
                print(label_name, 'label assigned to the pull request')
            except github.GithubException.GithubException:
                print(f'Failed to assign label {label_name}')


@task
def check_qa_labels(_, labels: str):
    """
    Check if the PR has one of qa/[done|no-code-change|rc-required] label
    """
    labels = set(labels.split(" "))
    all_qa_labels = {'qa/done', 'qa/no-code-change', 'qa/rc-required'}
    qa_labels = all_qa_labels.intersection(labels)
    docs = "\n".join(
        [
            "You must set one of:",
            "- 'qa/no-code-change' if your PR does not contain changes to the agent code or has no impact to the agent functionalities",
            "  Examples: code owner changes, e2e test framework changes, documentation changes",
            "- 'qa/done' if your PR contains changes impacting the Agent binary code that are validated through automated tests, double checked through manual validation if needed.",
            "  If you want additional validation by a second person, you can ask reviewers to do it. Describe how to set up an environment for manual tests in the PR description. Manual validation is expected to happen on every commit before merge.",
            "  Any manual validation step should then map to an automated test. Manual validation should not substitute automation, minus exceptions not supported by test tooling yet.",
            "- 'qa/rc-required' if your PR changes require validation on the Release Candidate. Examples are changes that need workloads that we cannot emulate, or changes that require validation on prod during RC deployment",
            "",
            "See https://datadoghq.atlassian.net/wiki/spaces/agent/pages/3341649081/QA+Best+Practices for more details.",
        ]
    )
    if len(qa_labels) == 0:
        raise Exit(f"No QA label set.\n{docs}", code=1)
    if len(qa_labels) > 1:
        raise Exit(f"More than one QA label set.\n{docs}", code=1)
    print("QA label set correctly")
