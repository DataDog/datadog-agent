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
    fast_tests=None,
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
            fast_tests=fast_tests,
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
def publish_comment_on_pr(_, branch_name, pipeline_id, job_name, executed_test: bool):
    from .libs.common.github_api import GithubAPI

    jobs_regex = re.compile(r"  - ([a-z-A-Z_0-9]*)")
    pipeline_id_regex = re.compile(r"pipeline ([0-9]*)")
    if branch_name == "" or job_name == "":
        return
    print("Branch name: ", branch_name)
    print("Pipeline id: ", pipeline_id)
    print("Job name: ", job_name)
    print("Executed test: ", executed_test)

    executed_test = True if executed_test == "true" else False

    gh = GithubAPI("DataDog/datadog-agent")

    pr = gh.get_pr_for_branch(branch_name)[0]
    print(pr)
    # If the branch is not linked to any PR we stop here
    if pr is None:
        return

    comment = gh.find_comment(pr.number, "[Fast Unit Tests Report]")
    print(comment)
    if comment is None:
        print("Here")
        if executed_test:
            print("Here 2")
            return
        msg = create_msg(pipeline_id, [job_name])
        print("Message: ", msg)
        gh.publish_comment(pr.number, msg)
        return

    print("body: ", comment.body)
    previous_comment_pipeline_id = pipeline_id_regex.findall(comment.body)[0]
    print("match: ", previous_comment_pipeline_id)
    print(previous_comment_pipeline_id)
    # An older pipeline should edit a message corresponding to a newer pipeline
    if previous_comment_pipeline_id > pipeline_id:
        return

    previous_comment_jobs = []
    if previous_comment_pipeline_id == pipeline_id:
        previous_comment_jobs = jobs_regex.findall(comment.body)
    print("previous_comment_jobs: ", previous_comment_jobs)

    if executed_test:
        if job_name in previous_comment_jobs:
            previous_comment_jobs.remove(job_name)
        msg = create_msg(pipeline_id, previous_comment_jobs)
        if len(previous_comment_jobs) == 0:
            gh.delete_comment(pr.number, comment.id)
        else:
            gh.update_comment(pr.number, comment.id, msg)
        return

    if not executed_test:
        if job_name not in previous_comment_jobs:
            previous_comment_jobs.append(job_name)
            msg = create_msg(pipeline_id, previous_comment_jobs)
            gh.update_comment(pr.number, comment.id, msg)

    return


def create_msg(pipeline_id, job_list):
    msg = f'''
[Fast Unit Tests Report]

Warning: On pipeline {pipeline_id} the following jobs did not run any unit tests:
'''
    for job in job_list:
        msg += f"  - {job}\n"
    msg += "\n"
    msg += "If you modified Go files and expected unit tests to run in these jobs, please double check the job logs, if you think tests should have been executed reach out #agent-platform"

    return msg
