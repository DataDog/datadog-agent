from collections import defaultdict

from invoke import task
from invoke.exceptions import Exit

from tasks.libs.owners.parsing import list_owners, read_owners, search_owners
from tasks.libs.pipeline.notifications import GITHUB_SLACK_MAP


@task
def find_jobowners(_, job, owners_file=".gitlab/JOBOWNERS"):
    print("DEPRECTATED: Use `dda info owners jobs` command instead.")


@task
def find_codeowners(_, path, owners_file=".github/CODEOWNERS"):
    # TODO(@agent-devx): Deprecate once `dda info owners code` is available and minimal `dda` version is bumped`
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


def _team_slugs(owners) -> list[str]:
    """Normalize CODEOWNERS owner strings to bare team slugs (`@DataDog/x` -> `x`), teams only."""
    slugs = set()
    for owner in owners:
        low = owner.casefold()
        if low.startswith('@datadog/'):
            slugs.add(low.replace('@datadog/', '', 1))
    return sorted(slugs)


def smp_pr_context_impl(branch: str, repository: str, owners_file: str):
    """Resolve the open PR for `branch`; return `(labels_csv, involved_teams_csv)`.

    Both are empty when there is no open PR. `labels` are the PR's applied labels; `involved_teams` are
    the CODEOWNERS-owning teams (bare slugs) of the PR's changed files. These are the only two inputs
    SMP manifest selection needs — passed to `smp experiments resolve` / `smp job submit-with-manifest`
    as `--label` and `--involved-team`. The `smp` CLI itself never parses CODEOWNERS or calls GitHub.

    Uses `GITHUB_TOKEN` from the environment (mint it via `dd-octo-sts` first).
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI(repository)
    prs = list(gh.get_pr_for_branch(head_branch_name=branch))
    if not prs:
        return "", ""
    pr = prs[0]
    labels = ",".join(gh.get_pr_labels(pr.number))
    files = gh.get_pr_files(pr.number)
    involved = _team_slugs(make_partition(files, owners_file).keys()) if files else []
    return labels, ",".join(involved)


@task
def smp_pr_context(
    _,
    branch,
    labels_out='pr_labels.txt',
    involved_teams_out='involved_teams.txt',
    repository='DataDog/datadog-agent',
    owners_file='.github/CODEOWNERS',
):
    """
    Resolve the open PR for `branch` and write the two inputs SMP manifest selection needs.

    Writes the comma-separated applied labels to `--labels-out` and the comma-separated involved teams
    (CODEOWNERS owners of the PR's changed files, as bare slugs) to `--involved-teams-out`; both are
    empty when no open PR is found. Pass these to `smp experiments resolve` / `smp job
    submit-with-manifest` as `--label` and `--involved-team`. Reads `GITHUB_TOKEN` from the environment
    (mint via `dd-octo-sts` first).
    """
    labels, involved = smp_pr_context_impl(branch, repository, owners_file)
    with open(labels_out, 'w') as f:
        f.write(labels)
    with open(involved_teams_out, 'w') as f:
        f.write(involved)
    print(f"labels=[{labels}] involved_teams=[{involved}]")
