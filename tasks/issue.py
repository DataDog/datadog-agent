import os

from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.issue.assign import assign_with_model, assign_with_rules
from tasks.libs.issue.model.actions import fetch_data_and_train_model
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP


@task
def assign_owner(_, issue_id, dry_run=False):
    gh = GithubAPI('DataDog/datadog-agent')
    issue = gh.repo.get_issue(int(issue_id))
    assignment = "model"
    owner, confidence = assign_with_model(issue)
    if confidence < 0.5:
        assignment = "rules"
        owner = assign_with_rules(issue, gh)
    print(f"Issue assigned to team/{owner} with {assignment}")
    if not dry_run:
        # Edit issue label
        issue.add_to_labels(f"team/{owner}")
        # Post message
        from slack_sdk import WebClient

        client = WebClient(os.environ['SLACK_API_TOKEN'])
        channel = next((chan for team, chan in GITHUB_SLACK_MAP.items() if owner.lower() in team), '#agent-devx-help')
        message = f':githubstatus_partial_outage: *New Community Issue*\n{issue.title} <{issue.html_url}|{gh.repo.name}#{issue_id}>\n'
        if channel == '#agent-ask-anything':
            message += "The CI bot failed to assign this issue to a team.\nPlease assign it manually."
        else:
            message += (
                "Your team was assigned automatically, using the issue content and title.\nPlease redirect if needed."
            )
        client.chat_postMessage(channel=channel, text=message)
    return owner


@task
def generate_model(_):
    fetch_data_and_train_model()
