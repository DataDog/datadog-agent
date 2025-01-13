import os
import random

from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI, ask_review_actor
from tasks.libs.issue.assign import assign_with_model, assign_with_rules
from tasks.libs.issue.model.actions import fetch_data_and_train_model
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP, GITHUB_SLACK_REVIEW_MAP, HELP_SLACK_CHANNEL


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
        channel = next((chan for team, chan in GITHUB_SLACK_MAP.items() if owner.lower() in team), HELP_SLACK_CHANNEL)
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


WAVES = [
    "wave",
    "waveboi",
    "wastelands-wave",
    "wave_hello",
    "wave-hokusai",
    "wave_moomin",
    "wave2",
    "wave3",
    "wallee-wave",
    "vaporeon_wave",
    "turtle-wave",
    "softwave",
    "shiba-wave",
    "minion-wave",
    "meow_wave_comfy",
    "mario-wave",
    "link-wave",
    "kirby_wave",
    "frog-wave",
    "fox_wave",
    "duckwave",
    "cyr-wave",
    "cozy-wave",
    "cat-wave",
    "capy-wave",
    "bufo-wave",
    "bongo-wave",
    "blobwave",
    "birb-wave",
    "arnaud-wave",
]


@task
def ask_reviews(_, pr_id):
    gh = GithubAPI()
    pr = gh.repo.get_pull(int(pr_id))
    if any(label.name == 'ask-review' for label in pr.get_labels()):
        actor = ask_review_actor(pr)
        reviewers = [f"@datadog/{team.slug}" for team in pr.requested_teams]

        from slack_sdk import WebClient

        client = WebClient(os.environ['SLACK_API_TOKEN'])
        for reviewer in reviewers:
            channel = next(
                (chan for team, chan in GITHUB_SLACK_REVIEW_MAP.items() if team.casefold() == reviewer.casefold()),
                HELP_SLACK_CHANNEL,
            )
            message = f'Hello :{random.choice(WAVES)}:!\n*{actor}* is asking review for PR <{pr.html_url}/s|{pr.title}>.\nCould you please have a look?\nThanks in advance!'
            if channel == HELP_SLACK_CHANNEL:
                message = f'Hello :{random.choice(WAVES)}:!\nA review channel is missing for {reviewer}, can you please ask them to update `github_slack_review_map.yaml` and transfer them this review <{pr.html_url}/s|{pr.title}>?\n Thanks in advance!'
            try:
                client.chat_postMessage(channel=channel, text=message)
            except Exception as e:
                message = f"An error occurred while sending a review message from {actor} for PR <{pr.html_url}/s|{pr.title}> to channel {channel}. Error: {e}"
                client.chat_postMessage(channel='#agent-devx-ops', text=message)
