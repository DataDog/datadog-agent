import os
import random
import re
from collections import defaultdict

from invoke.tasks import task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.owners.parsing import search_owners
from tasks.libs.pipeline.notifications import (
    DEFAULT_SLACK_CHANNEL,
    GITHUB_SLACK_REVIEW_MAP,
)


@task(iterable=["team_slugs"])
def ask_reviews(_, pr_id, action, team_slugs):
    gh = GithubAPI()
    pr = gh.repo.get_pull(int(pr_id))
    if pr.base.ref != 'main':
        print("We don't ask for reviews on non main target PRs.")
        return
    if action != "labeled" and _is_revert(pr):
        print("We don't ask for reviews on revert PRs creation, only on label requests.")
        return
    if any(label.name == 'no-review' for label in pr.get_labels()):
        print("This PR has the no-review label, we don't need to ask for reviews.")
        return
    # team_slugs is a list[str] thanks to @task(iterable=["team_slugs"])
    if not team_slugs:
        print("No requested teams provided, skipping.")
        return

    cleaned = []
    for slug in team_slugs:
        slug = (slug or "").strip()
        slug = slug.removeprefix("@datadog/").removeprefix("@DataDog/")  # tolerate callers passing full team handles
        if slug:
            cleaned.append(slug)
    if not cleaned:
        print("No requested teams provided, skipping.")
        return

    reviewers = [f"@datadog/{slug}" for slug in cleaned]
    print(f"Reviewers: {reviewers}")

    from slack_sdk import WebClient

    client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    emojis = client.emoji_list()
    waves = [emoji for emoji in emojis.data['emoji'] if 'wave' in emoji and 'microwave' not in emoji]

    channels = defaultdict(list)
    for reviewer in reviewers:
        channel = next(
            (chan for team, chan in GITHUB_SLACK_REVIEW_MAP.items() if team.casefold() == reviewer.casefold()),
            DEFAULT_SLACK_CHANNEL,
        )
        channels[channel].append(reviewer)

    actor = pr.user.name or pr.user.login
    for channel, reviewers in channels.items():
        stop_updating = ""
        if (pr.user.login == "renovate[bot]" or pr.user.login == "mend[bot]") and pr.title.startswith(
            "chore(deps): update integrations-core"
        ):
            stop_updating = "Add the `stop-updating` label before trying to merge this PR, to prevent it from being updated by Renovate.\n"
        message = f'Hello :{random.choice(waves)}:!\n*{actor}* is asking review for PR <{pr.html_url}/s|{pr.title}>.\nCould you please have a look?\n{stop_updating}Thanks in advance!\n'
        if channel == DEFAULT_SLACK_CHANNEL:
            missing = ", ".join(reviewers)
            message = (
                f'Hello :{random.choice(waves)}:!\n'
                f'A review channel is missing for {missing}, can you please ask them to update '
                '`github_slack_review_map.yaml` and transfer them this review '
                f'<{pr.html_url}/s|{pr.title}>?\n Thanks in advance!'
            )
        try:
            client.chat_postMessage(channel=channel, text=message)
        except Exception as e:
            message = f"An error occurred while sending a review message from {actor} for PR <{pr.html_url}/s|{pr.title}> to channel {channel}. Error: {e}"
            client.chat_postMessage(channel=DEFAULT_SLACK_CHANNEL, text=message)


def _is_revert(pr) -> bool:
    """
    Check if a PR is a revert PR.
    """
    commits = pr.get_commits()
    # Only check the first commit message
    if re.match(r"^Revert \"(.*)\"\n\nThis reverts commit (\w+).", commits[0].commit.message):
        return True
    return False


@task
def add_reviewers(ctx, pr_id, dry_run=False, owner_file=".github/CODEOWNERS"):
    """
    Add team labels and reviewers to a dependabot bump PR based on the changed dependencies
    """

    gh = GithubAPI()
    pr = gh.repo.get_pull(int(pr_id))

    requested_reviewers = []
    for page in pr.get_review_requests():
        for rr in page:
            requested_reviewers.append(rr)

    if len(requested_reviewers) > 0:
        print(
            f"This PR already has already requested review to {', '.join([rr.name for rr in requested_reviewers])}, this action should not be run on it."
        )
        return

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
