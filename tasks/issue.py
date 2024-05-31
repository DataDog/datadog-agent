from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.issue.assign import assign_with_model, assign_with_rules
from tasks.libs.issue.model.actions import generate_model


@task
def assign_owner(_, issue_id):
    gh = GithubAPI('DataDog/datadog-agent')
    issue = gh.repo.get_issue(int(issue_id))
    owner, confidence = assign_with_model(issue)
    if confidence < 0.5:
        owner = assign_with_rules(issue, gh)
    print(owner)
    return owner


@task
def generate_the_model(_):
    generate_model()
