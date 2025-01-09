import os
import pathlib
import re
import subprocess
from typing import Dict

import yaml


def load_and_validate(file_name: str, default_placeholder: str, default_value: str) -> Dict[str, str]:
    p = pathlib.Path(os.path.realpath(__file__)).parent.joinpath(file_name)

    result: Dict[str, str] = {}
    with p.open(encoding='utf-8') as file_stream:
        for key, value in yaml.safe_load(file_stream).items():
            if not (type(key) is str and type(value) is str):
                raise ValueError(f"File {file_name} contains a non-string key or value. Key: {key}, Value: {value}")
            result[key] = default_value if value == default_placeholder else value
    return result


DATADOG_AGENT_GITHUB_ORG_URL = "https://github.com/DataDog"
DEFAULT_SLACK_CHANNEL = "#agent-devx-ops"
DEFAULT_JIRA_PROJECT = "AGNTR"
# Map keys in lowercase
GITHUB_SLACK_MAP = load_and_validate("github_slack_map.yaml", "DEFAULT_SLACK_CHANNEL", DEFAULT_SLACK_CHANNEL)
GITHUB_JIRA_MAP = load_and_validate("github_jira_map.yaml", "DEFAULT_JIRA_PROJECT", DEFAULT_JIRA_PROJECT)


def read_owners(owners_file):
    from codeowners import CodeOwners

    with open(owners_file, 'r') as f:
        return CodeOwners(f.read())


def check_for_missing_owners_slack_and_jira(print_missing_teams=True, owners_file=".github/CODEOWNERS"):
    owners = read_owners(owners_file)
    error = False
    for path in owners.paths:
        if not path[2] or path[2][0][0] != "TEAM":
            continue
        if path[2][0][1].lower() not in GITHUB_SLACK_MAP:
            error = True
            if print_missing_teams:
                print(f"The team {path[2][0][1]} doesn't have a slack team assigned !!")
        if path[2][0][1].lower() not in GITHUB_JIRA_MAP:
            error = True
            if print_missing_teams:
                print(f"The team {path[2][0][1]} doesn't have a jira project assigned !!")
    return error


def base_message(header, state):
    project_title = os.getenv("CI_PROJECT_TITLE")
    # commit_title needs a default string value, otherwise the re.search line below crashes
    commit_title = os.getenv("CI_COMMIT_TITLE", "")
    pipeline_url = os.getenv("CI_PIPELINE_URL")
    pipeline_id = os.getenv("CI_PIPELINE_ID")
    commit_ref_name = os.getenv("CI_COMMIT_REF_NAME")
    commit_url_gitlab = f"{os.getenv('CI_PROJECT_URL')}/commit/{os.getenv('CI_COMMIT_SHA')}"
    commit_url_github = f"{DATADOG_AGENT_GITHUB_ORG_URL}/{project_title}/commit/{os.getenv('CI_COMMIT_SHA')}"
    commit_short_sha = os.getenv("CI_COMMIT_SHORT_SHA")
    author = get_git_author()

    # Try to find a PR id (e.g #12345) in the commit title and add a link to it in the message if found.
    parsed_pr_id_found = re.search(r'.*\(#([0-9]*)\)$', commit_title)
    enhanced_commit_title = commit_title
    if parsed_pr_id_found:
        parsed_pr_id = parsed_pr_id_found.group(1)
        pr_url_github = f"{DATADOG_AGENT_GITHUB_ORG_URL}/{project_title}/pull/{parsed_pr_id}"
        enhanced_commit_title = enhanced_commit_title.replace(f"#{parsed_pr_id}", f"<{pr_url_github}|#{parsed_pr_id}>")

    return f"""{header} pipeline <{pipeline_url}|{pipeline_id}> for {commit_ref_name} {state}.
{enhanced_commit_title} (<{commit_url_gitlab}|{commit_short_sha}>)(:github: <{commit_url_github}|link>) by {author}"""


def get_git_author():
    return (
        subprocess.check_output(["git", "show", "-s", "--format='%an'", "HEAD"])
        .decode('utf-8')
        .strip()
        .replace("'", "")
    )


def send_slack_message(recipient, message):
    subprocess.run(["postmessage", recipient, message], check=True)
