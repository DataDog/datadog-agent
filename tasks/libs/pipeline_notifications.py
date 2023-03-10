import json
import os
import subprocess
from collections import defaultdict

from .common.gitlab import Gitlab, get_gitlab_token
from .types import FailedJobType, Test

DEFAULT_SLACK_CHANNEL = "#agent-platform"
# Map keys in lowercase
GITHUB_SLACK_MAP = {
    "@datadog/agent-platform": DEFAULT_SLACK_CHANNEL,
    "@datadog/documentation": DEFAULT_SLACK_CHANNEL,
    "@datadog/container-integrations": "#container-integrations",
    "@datadog/platform-integrations": "#platform-integrations",
    "@datadog/agent-security": "#security-and-compliance-agent-ops",
    "@datadog/agent-apm": "#apm-agent",
    "@datadog/network-device-monitoring": "#network-device-monitoring",
    "@datadog/processes": "#process-agent-ops",
    "@datadog/agent-metrics-logs": "#agent-metrics-logs",
    "@datadog/agent-shared-components": "#agent-shared-components",
    "@datadog/container-app": "#container-app",
    "@datadog/metrics-aggregation": "#metrics-aggregation",
    "@datadog/serverless": "#serverless-agent",
    "@datadog/remote-config": "#remote-config-monitoring",
    "@datadog/agent-all": "#datadog-agent-pipelines",
    "@datadog/ebpf-platform": "#ebpf-platform-ops",
    "@datadog/networks": "#network-performance-monitoring",
    "@datadog/universal-service-monitoring": "#universal-service-monitoring",
    "@datadog/windows-agent": "#windows-agent",
    "@datadog/windows-kernel-integrations": "#windows-kernel-integrations",
    "@datadog/opentelemetry": "#opentelemetry-ops",
    "@datadog/agent-e2e-testing": "#agent-testing-and-qa",
    "@datadog/software-integrity-and-trust": "#sit",
    "@datadog/single-machine-performance": "#single-machine-performance",
    "@datadog/agent-integrations": "#agent-integrations",
}


def read_owners(owners_file):
    from codeowners import CodeOwners

    with open(owners_file, 'r') as f:
        return CodeOwners(f.read())


def check_for_missing_owners_slack(print_missing_teams=True, owners_file=".github/CODEOWNERS"):
    owners = read_owners(owners_file)
    error = False
    for path in owners.paths:
        if not path[2] or path[2][0][0] != "TEAM":
            continue
        if path[2][0][1].lower() not in GITHUB_SLACK_MAP:
            error = True
            if print_missing_teams:
                print(f"The team {path[2][0][1]} doesn't have a slack team assigned !!")
    return error


def get_failed_tests(project_name, job, owners_file=".github/CODEOWNERS"):
    gitlab = Gitlab(project_name=project_name, api_token=get_gitlab_token())
    owners = read_owners(owners_file)
    test_output = gitlab.artifact(job["id"], "test_output.json", ignore_not_found=True)
    failed_tests = {}  # type: dict[tuple[str, str], Test]
    if test_output:
        for line in test_output.iter_lines():
            json_test = json.loads(line)
            if 'Test' in json_test:
                name = json_test['Test']
                package = json_test['Package']
                action = json_test["Action"]

                if action == "fail":
                    # Ignore subtests, only the parent test should be reported for now
                    # to avoid multiple reports on the same test
                    # NTH: maybe the Test object should be more flexible to incorporate
                    # subtests? This would require some postprocessing of the Test objects
                    # we yield here to merge child Test objects with their parents.
                    if '/' in name:  # Subtests have a name of the form "Test/Subtest"
                        continue
                    failed_tests[(package, name)] = Test(owners, name, package)
                elif action == "pass" and (package, name) in failed_tests:
                    print(f"Test {name} from package {package} passed after retry, removing from output")
                    del failed_tests[(package, name)]

    return failed_tests.values()


def find_job_owners(failed_jobs, owners_file=".gitlab/JOBOWNERS"):
    owners = read_owners(owners_file)
    owners_to_notify = defaultdict(list)

    for job in failed_jobs:
        # Exclude jobs that failed due to infrastructure failures
        if job["failure_type"] == FailedJobType.INFRA_FAILURE:
            continue
        job_owners = owners.of(job["name"])
        # job_owners is a list of tuples containing the type of owner (eg. USERNAME, TEAM) and the name of the owner
        # eg. [('TEAM', '@DataDog/agent-platform')]

        for kind, owner in job_owners:
            if kind == "TEAM":
                owners_to_notify[owner].append(job)

    return owners_to_notify


def base_message(header, state):
    return """{header} pipeline <{pipeline_url}|{pipeline_id}> for {commit_ref_name} {state}.
{commit_title} (<{commit_url}|{commit_short_sha}>) by {author}""".format(  # noqa: FS002
        header=header,
        pipeline_url=os.getenv("CI_PIPELINE_URL"),
        pipeline_id=os.getenv("CI_PIPELINE_ID"),
        commit_ref_name=os.getenv("CI_COMMIT_REF_NAME"),
        commit_title=os.getenv("CI_COMMIT_TITLE"),
        commit_url="{project_url}/commit/{commit_sha}".format(  # noqa: FS002
            project_url=os.getenv("CI_PROJECT_URL"), commit_sha=os.getenv("CI_COMMIT_SHA")
        ),
        commit_short_sha=os.getenv("CI_COMMIT_SHORT_SHA"),
        author=get_git_author(),
        state=state,
    )


def get_git_author():
    return (
        subprocess.check_output(["git", "show", "-s", "--format='%an'", "HEAD"])
        .decode('utf-8')
        .strip()
        .replace("'", "")
    )


def send_slack_message(recipient, message):
    subprocess.run(["postmessage", recipient, message], check=True)
