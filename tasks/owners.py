from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI, get_github_teams
from tasks.libs.common.utils import guess_from_keywords, guess_from_labels, team_to_label
from tasks.libs.owners.parsing import most_frequent_agent_team, search_owners


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
