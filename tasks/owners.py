from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.owners.parsing import list_owners, read_owners, search_owners
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP


@task
def find_jobowners(_, job, owners_file=".gitlab/JOBOWNERS"):
    print(", ".join(search_owners(job, owners_file)))


@task
def find_codeowners(_, path, owners_file=".github/CODEOWNERS"):
    print(", ".join(search_owners(path, owners_file)))


@task
def list_files(ctx, team, owners_file=".github/CODEOWNERS"):
    """
    List all files owned by a particular team.
    """

    valid_owners = list(list_owners(owners_file))

    if team not in valid_owners:
        raise Exit(f"unexpected owner '{team}'")

    code_owners = read_owners(owners_file)
    result = ctx.run('git ls-files', hide=True)
    files = result.stdout.splitlines()

    for file_name in files:
        normalized_owners = [owner[1].casefold().replace("@datadog/", "") for owner in code_owners.of(file_name)]
        if team in normalized_owners:
            print(file_name)


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


def channel_owners(channel: str) -> list[str]:
    """
    Returns the teams that own the slack channel
    """
    return [team for team, chan in GITHUB_SLACK_MAP.items() if chan == channel]
