import os
import random
import re
from collections import defaultdict

from invoke.tasks import task

from tasks.libs.ciproviders.github_api import GithubAPI, get_pr_size
from tasks.libs.owners.parsing import read_owners, search_owners
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

    requested = []
    for slug in team_slugs:
        slug = (slug or "").strip()
        slug = slug.removeprefix("@datadog/").removeprefix("@DataDog/")  # tolerate callers passing full team handles
        if slug:
            requested.append(slug)
    if not requested:
        print("No requested teams provided, skipping.")
        return

    print(f"Requested reviewers: {requested}")

    from slack_sdk import WebClient

    client = WebClient(os.environ['SLACK_DATADOG_AGENT_BOT_TOKEN'])
    emojis = client.emoji_list()
    waves = [emoji for emoji in emojis.data['emoji'] if 'wave' in emoji and 'microwave' not in emoji]

    # Compute per-team file counts and PR size
    file_counts = _get_team_file_counts(pr, requested)
    size = get_pr_size(pr)
    max_files = max(file_counts.values()) if file_counts else 0

    actor = pr.user.name or pr.user.login
    stop_updating = ""
    if (pr.user.login == "renovate[bot]" or pr.user.login == "mend[bot]") and pr.title.startswith(
        "chore(deps): update integrations-core"
    ):
        stop_updating = "Add the `stop-updating` label before trying to merge this PR, to prevent it from being updated by Renovate.\n"

    channels = defaultdict(list)
    for slug in requested:
        reviewer = f"@datadog/{slug}"
        channel = next(
            (chan for team, chan in GITHUB_SLACK_REVIEW_MAP.items() if team.casefold() == reviewer.casefold()),
            DEFAULT_SLACK_CHANNEL,
        )
        channels[channel].append(slug)

    for channel, slugs in channels.items():
        if channel == DEFAULT_SLACK_CHANNEL:
            missing_teams = ", ".join(f"@datadog/{s}" for s in slugs)
            message = (
                f'Hello :{random.choice(waves)}:!\n'
                f'A review channel is missing for {missing_teams}, can you please ask them to update '
                '`github_slack_review_map.yaml` and transfer them this review '
                f'<{pr.html_url}/s|{pr.title}>?\n Thanks in advance!'
            )
        else:
            team_lines = []
            for slug in slugs:
                nb_files = file_counts.get(slug, 0)
                role = "primary" if (max_files > 0 and nb_files == max_files) else "secondary"
                team_lines.append(f'{slug} has {nb_files} file(s) to review, as a {role} contributor.')
            team_info = '\n'.join(team_lines)
            message = (
                f'Hello :{random.choice(waves)}:!\n'
                f'*{actor}* is asking review for PR <{pr.html_url}/s|{pr.title}>.\n'
                f'This is a `{size}` PR.\n'
                f'{team_info}\n'
                f'{stop_updating}Could you please have a look? Thanks in advance!\n'
            )

        try:
            client.chat_postMessage(channel=channel, text=message)
        except Exception as e:
            error_message = f"An error occurred while sending a review message from {actor} for PR <{pr.html_url}/s|{pr.title}> to channel {channel}. Error: {e}"
            client.chat_postMessage(channel=DEFAULT_SLACK_CHANNEL, text=error_message)


def _is_revert(pr) -> bool:
    """
    Check if a PR is a revert PR.
    """
    commits = pr.get_commits()
    # Only check the first commit message
    if re.match(r"^Revert \"(.*)\"\n\nThis reverts commit (\w+).", commits[0].commit.message):
        return True
    return False


def _get_team_file_counts(pr, team_slugs, owners_file='.github/CODEOWNERS'):
    """Return a dict mapping each team slug to the number of PR files it owns."""
    owners = read_owners(owners_file)
    counts = {slug: 0 for slug in team_slugs}
    for f in pr.get_files():
        file_owners = owners.of(f.filename)
        for _kind, owner_handle in file_owners:
            normalized = owner_handle.casefold().removeprefix("@datadog/")
            for slug in team_slugs:
                if slug.casefold() == normalized:
                    counts[slug] += 1
    return counts


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
            f"This PR already has already requested review to {requested_reviewers}, this action should not be run on it."
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

    # Find the responsible team for each file that uses the dependency; count uses per team.
    owner_usage_count = {}
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
                            for owner in search_owners(file, owner_file):
                                slug = owner.casefold().removeprefix("@datadog/")
                                owner_usage_count[slug] = owner_usage_count.get(slug, 0) + 1
                            break
    if dry_run:
        print(f"Owner usage for {dependency}: {owner_usage_count}")
        return
    # Cap reviewers to avoid asking too many teams
    team_slugs = sorted(owner_usage_count, key=lambda s: -owner_usage_count[s])[:3]
    pr.create_review_request(team_reviewers=team_slugs)
