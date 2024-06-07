from collections import defaultdict

from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI, get_github_teams
from tasks.libs.common.utils import guess_from_keywords, guess_from_labels, team_to_label
from tasks.libs.owners.parsing import most_frequent_agent_team, read_owners, search_owners
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP


@task
def find_jobowners(_, job, owners_file=".gitlab/JOBOWNERS"):
    print(", ".join(search_owners(job, owners_file)))


@task
def find_codeowners(_, path, owners_file=".github/CODEOWNERS"):
    print(", ".join(search_owners(path, owners_file)))


@task
def guess_responsible(_, issue_id):
    gh = GithubAPI('DataDog/datadog-agent')
    issue = gh.repo.get_issue(int(issue_id))
    owner = guess_from_labels(issue)
    if owner == 'triage':
        users = [user for user in issue.assignees if gh.is_organization_member(user)]
        teams = get_github_teams(users)
        owner = most_frequent_agent_team(teams)
    if owner == 'triage':
        commenters = [c.user for c in issue.get_comments() if gh.is_organization_member(c.user)]
        teams = get_github_teams(commenters)
        owner = most_frequent_agent_team(teams)
    if owner == 'triage':
        owner = guess_from_keywords(issue)
    owner = team_to_label(owner)
    print(owner)
    return owner


def make_partition(names: list[str], owners_file: str, get_channels: bool = False) -> dict[str, set[str]]:
    """
    From a list of job / file names, will create a dictionary with the teams as keys and the names as values.

    - If get_channels, the teams will be replaced by team channels.

    Example
    -------
    If job1 belongs to team1 and team2, and job2 belongs to team2 and team3, the output will be:
    {
        "team1": {"job1"},
        "team2": {"job1", "job2"},
        "team3": {"job2"},
    }
    """
    owners = read_owners(owners_file)
    mapping = defaultdict(set)

    for name in names:
        teams = owners.of(name)
        for label, team in teams:
            if label != 'TEAM':
                continue

            if get_channels:
                team = GITHUB_SLACK_MAP.get(team.casefold(), None)
                if team is None:
                    continue

            mapping[team].add(name)

    return mapping
