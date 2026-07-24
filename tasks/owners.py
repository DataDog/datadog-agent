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


@task
def smp_preselect(_, config_dir, changed_files, exclude="", owners_file=".github/CODEOWNERS"):
    """
    Print the comma-separated experiment folders (relative to config_dir) to
    pre-tick in the SMP experiment-selection comment: a folder is included when
    its CODEOWNERS-owning team also owns at least one file listed in the
    `changed_files` file. Used by the smp-experiment-selection-comment CI job.

    - config_dir:    discovery root, e.g. test/regression
    - changed_files: path to a file listing the PR's changed paths, one per line
    - exclude:       comma-separated top-level folder names to skip (e.g.
                     "quality_gates,ebpf" -- always-run and separate-runner suites)

    A selectable folder is any directory directly containing a `cases/` dir
    (matching the SMP CLI's discovery), addressed by its path relative to
    config_dir (e.g. "logs", "logs/syslog"). Ownership is resolved from the
    checked-out CODEOWNERS on both sides, so the result depends only on the PR.
    """
    import os

    def _teams(owner_tuples):
        # keep TEAM entries only (drop individual users), normalized to bare slugs
        return {team.casefold().replace("@datadog/", "") for label, team in owner_tuples if label == 'TEAM'}

    code_owners = read_owners(owners_file)

    with open(changed_files) as f:
        changed = [line.strip() for line in f if line.strip()]
    involved_teams = {t.casefold().replace("@datadog/", "") for t in make_partition(changed, owners_file)}

    excluded = {e.strip() for e in exclude.split(",") if e.strip()}

    selected = []
    for root, dirs, _files in os.walk(config_dir):
        if "cases" not in dirs:
            continue
        rel = os.path.relpath(root, config_dir)
        if rel == "." or rel.split(os.sep)[0] in excluded:
            continue
        if _teams(code_owners.of(f"{config_dir}/{rel}/")) & involved_teams:
            selected.append(rel)

    print(",".join(sorted(set(selected))))


def channel_owners(channel: str) -> list[str]:
    """
    Returns the teams that own the slack channel
    """
    return [team for team, chan in GITHUB_SLACK_MAP.items() if chan == channel]
