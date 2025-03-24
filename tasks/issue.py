import os
import random
import re

from invoke import task

from tasks.libs.ciproviders.github_api import GithubAPI, ask_review_actor
from tasks.libs.issue.assign import assign_with_model, assign_with_rules
from tasks.libs.issue.model.actions import fetch_data_and_train_model
from tasks.libs.owners.parsing import search_owners
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

        client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
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


@task
def ask_reviews(_, pr_id):
    gh = GithubAPI()
    pr = gh.repo.get_pull(int(pr_id))
    if 'backport' in pr.title.casefold():
        print("This is a backport PR, we don't need to ask for reviews.")
        return
    if any(label.name == 'ask-review' for label in pr.get_labels()):
        actor = ask_review_actor(pr)
        reviewers = [f"@datadog/{team.slug}" for team in pr.requested_teams]

        from slack_sdk import WebClient

        client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
        emojis = client.emoji_list()
        waves = [emoji for emoji in emojis.data['emoji'] if 'wave' in emoji and 'microwave' not in emoji]
        for reviewer in reviewers:
            channel = next(
                (chan for team, chan in GITHUB_SLACK_REVIEW_MAP.items() if team.casefold() == reviewer.casefold()),
                HELP_SLACK_CHANNEL,
            )
            message = f'Hello :{random.choice(waves)}:!\n*{actor}* is asking review for PR <{pr.html_url}/s|{pr.title}>.\nCould you please have a look?\nThanks in advance!'
            if channel == HELP_SLACK_CHANNEL:
                message = f'Hello :{random.choice(waves)}:!\nA review channel is missing for {reviewer}, can you please ask them to update `github_slack_review_map.yaml` and transfer them this review <{pr.html_url}/s|{pr.title}>?\n Thanks in advance!'
            try:
                client.chat_postMessage(channel=channel, text=message)
            except Exception as e:
                message = f"An error occurred while sending a review message from {actor} for PR <{pr.html_url}/s|{pr.title}> to channel {channel}. Error: {e}"
                client.chat_postMessage(channel='#agent-devx-ops', text=message)


@task
def add_reviewers(ctx, pr_id, dry_run=False, owner_file=".github/CODEOWNERS"):
    """
    Add team labels and reviewers to a dependabot bump PR based on the changed dependencies
    """

    gh = GithubAPI()
    pr = gh.repo.get_pull(int(pr_id))

    if pr.user.login != "dependabot[bot]":
        print("This is not a (dependabot) bump PR, this action should not be run on it.")
        return

    folder = ""
    if pr.title.startswith("Bump the "):
        match = re.match(r"^Bump the (\S+) group (.*$)", pr.title)
        if match.group(2).startswith("in"):
            match_folder = re.match(r"^in (\S+).*$", match.group(2))
            folder = match_folder.group(1).removeprefix("/")
    else:
        match = re.match(r"^Bump (\S+) from (\S+) to (\S+)( in .*)?$", pr.title)
        if match.group(4):
            match_folder = re.match(r"^ in (\S+).*$", match.group(4))
            folder = match_folder.group(1).removeprefix("/")
    dependency = match.group(1)

    # Find the responsible person for each file
    owners = set()
    git_files = ctx.run("git ls-files | grep -e \"^.*.go$\"", hide=True).stdout
    for file in git_files.splitlines():
        if not file.startswith(folder):
            continue
        in_import = False
        with open(file) as f:
            for line in f:
                # Look for the import block
                if "import (" in line:
                    in_import = True
                if in_import:
                    # Early exit at the end of the import block
                    if ")" in line:
                        break
                    else:
                        if dependency in line:
                            owners.update(set(search_owners(file, owner_file)))
                            break
    if dry_run:
        print(f"Owners for {dependency}: {owners}")
        return
    # Teams are added by slug, so we need to remove the @DataDog/ prefix
    pr.create_review_request(team_reviewers=[owner.casefold().removeprefix("@datadog/") for owner in owners])
