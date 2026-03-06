from __future__ import annotations

import os
import pathlib
import re
from datetime import datetime, timezone

import yaml

from tasks.libs.owners.parsing import read_owners


def load_and_validate(
    file_name: str, default_placeholder: str, default_value: str, relpath: bool = True
) -> dict[str, str]:
    if relpath:
        p = pathlib.Path(os.path.realpath(__file__)).parent.joinpath(file_name)
    else:
        p = pathlib.Path(file_name)

    result: dict[str, str] = {}
    with p.open(encoding='utf-8') as file_stream:
        for key, value in yaml.safe_load(file_stream).items():
            if not (isinstance(key, str) and isinstance(value, str)):
                raise ValueError(f"File {file_name} contains a non-string key or value. Key: {key}, Value: {value}")
            result[key] = default_value if value == default_placeholder else value
    return result


class ProxyMap(dict):
    def __init__(self, file_name, channel_type):
        self.file_name = file_name
        self.channel_type = channel_type
        self._local_map = None
        self._final_map = None

    def _ensure_initialized(self):
        if self._final_map is not None:
            return

        if self._local_map is None:
            self._local_map = load_and_validate(self.file_name, "DEFAULT_SLACK_CHANNEL", DEFAULT_SLACK_CHANNEL)

        self._final_map = self._local_map.copy()
        try:
            from tasks.libs.pipeline.packs import get_team_channels

            for team in self._local_map:
                notif, review = get_team_channels(team)
                pack_val = notif if self.channel_type == 'notification' else review
                if pack_val:
                    self._final_map[team] = pack_val
        except Exception:
            # Fallback to local map on error (e.g. no network, no ddtool)
            pass

    def __getitem__(self, key):
        self._ensure_initialized()
        return self._final_map[key]

    def __iter__(self):
        self._ensure_initialized()
        return iter(self._final_map)

    def __len__(self):
        self._ensure_initialized()
        return len(self._final_map)

    def __contains__(self, key):
        self._ensure_initialized()
        return key in self._final_map

    def keys(self):
        self._ensure_initialized()
        return self._final_map.keys()

    def values(self):
        self._ensure_initialized()
        return self._final_map.values()

    def items(self):
        self._ensure_initialized()
        return self._final_map.items()

    def get(self, key, default=None):
        self._ensure_initialized()
        return self._final_map.get(key, default)

    def clear(self):
        # Needed for some tests that clear the map
        self._final_map = {}
        self._local_map = {}

    def __setitem__(self, key, value):
        self._ensure_initialized()
        self._final_map[key] = value

    def __repr__(self):
        self._ensure_initialized()
        return repr(self._final_map)


GITHUB_BASE_URL = "https://github.com"
DEFAULT_SLACK_CHANNEL = "#agent-devx-ops"
HELP_SLACK_CHANNEL = "#agent-devx-help"
DEFAULT_JIRA_PROJECT = "AGNTR"
# Map keys in lowercase
GITHUB_SLACK_MAP = ProxyMap("github_slack_map.yaml", "notification")
GITHUB_JIRA_MAP = load_and_validate("github_jira_map.yaml", "DEFAULT_JIRA_PROJECT", DEFAULT_JIRA_PROJECT)
GITHUB_SLACK_REVIEW_MAP = ProxyMap("github_slack_review_map.yaml", "review")


def check_for_missing_owners_slack_and_jira(print_missing_teams=True, owners_file=".github/CODEOWNERS"):
    owners = read_owners(owners_file)
    error = False
    teams = {p[2][0][1].lower() for p in owners.paths if p[2] and p[2][0][0] == "TEAM"}
    for team in teams:
        for gh_map, map_name in [
            (GITHUB_SLACK_MAP, 'slack'),
            (GITHUB_JIRA_MAP, 'jira'),
            (GITHUB_SLACK_REVIEW_MAP, 'slack review'),
        ]:
            if team not in gh_map:
                error = True
                if print_missing_teams:
                    print(f"The team {team} is missing from the Github {map_name} map. Please update!!")
    return error


def get_pr_from_commit(commit_title: str, project_name: str) -> tuple[str, str] | None:
    """
    Tries to find a GitHub PR id within a commit title (eg: "Fix PR (#27584)"),
    and returns the corresponding PR URL.

    commit_title: the commit title to parse
    project_name: the GitHub project from which the PR originates, in the "org/repo" format
    """

    parsed_pr_id_found = re.search(r'.*#([0-9]+)', commit_title)
    if not parsed_pr_id_found:
        return None

    parsed_pr_id = parsed_pr_id_found.group(1)

    return parsed_pr_id, f"{GITHUB_BASE_URL}/{project_name}/pull/{parsed_pr_id}"


def warn_new_commits(release_managers, team, branch, next_rc):
    from slack_sdk import WebClient

    today = datetime.today()
    rc_date = datetime(today.year, today.month, today.day, hour=14, minute=0, second=0, tzinfo=timezone.utc)
    rc_schedule_link = "https://github.com/DataDog/datadog-agent/blob/main/.github/workflows/create_rc_pr.yml#L6"
    message = "Hello :wave:\n"
    message += f":announcement: We detected new commits on the {branch} release branch of `integrations-core`.\n"
    message += f"Could you please release and tag your repo to prepare the {next_rc} `datadog-agent` release candidate planned <{rc_schedule_link}|{rc_date.strftime('%Y-%m-%d %H:%M')}> UTC?\n"
    message += "Thanks in advance!\n"
    message += f"cc {' '.join(release_managers)}"
    client = WebClient(os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
    client.chat_postMessage(channel=f"#{team}", text=message)


def warn_new_tags(message):
    from slack_sdk import WebClient

    client = WebClient(os.environ["SLACK_DATADOG_AGENT_BOT_TOKEN"])
    client.chat_postMessage(channel="#agent-release-sync", text=message)
