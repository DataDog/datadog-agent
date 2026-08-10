import json
import os
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


def _discover_smp_leaves(config_dir: str, exclude: list[str]) -> list[str]:
    """Discover SMP leaves under `config_dir`.

    A leaf is a directory that directly contains a `cases/` subdirectory; its path relative to
    `config_dir` (forward-slash) is the leaf path. Skips any leaf at or under an `exclude` path
    (relative to `config_dir`, e.g. `ebpf`).
    """
    leaves = []
    for root, dirs, _files in os.walk(config_dir):
        if 'cases' in dirs:
            rel = os.path.relpath(root, config_dir).replace(os.sep, '/')
            if rel != '.' and not any(rel == ex or rel.startswith(f"{ex}/") for ex in exclude):
                leaves.append(rel)
            dirs.remove('cases')  # experiments under cases/ are not themselves leaves
    return sorted(leaves)


def _leaf_ownership(leaves: list[str], config_dir: str, owners_file: str) -> dict[str, list[str]]:
    """Map each leaf path to its owning team slugs, resolved from CODEOWNERS."""
    base = config_dir.rstrip('/')
    return {leaf: _team_slugs(search_owners(f"{base}/{leaf}/", owners_file)) for leaf in leaves}


def smp_inputs_impl(config_dir: str, changed_files: list[str], exclude: list[str], owners_file: str):
    """Compute the CODEOWNERS-derived inputs for `smp experiments resolve` (offline, pure).

    Returns `(involved_teams, ownership)`:
    - `involved_teams`: sorted team slugs owning any of `changed_files`.
    - `ownership`: `{leaf_path: [team_slug, ...]}` for every leaf under `config_dir` (minus `exclude`).
    """
    involved = _team_slugs(make_partition(changed_files, owners_file).keys()) if changed_files else []
    ownership = _leaf_ownership(_discover_smp_leaves(config_dir, exclude), config_dir, owners_file)
    return involved, ownership


@task
def smp_inputs(
    _,
    config_dir,
    changed_files='',
    exclude='',
    ownership_out='ownership.json',
    owners_file='.github/CODEOWNERS',
):
    """
    Emit the CODEOWNERS-derived inputs for `smp experiments resolve`.

    Writes the leaf -> owning-teams map to `--ownership-out` (JSON) and prints the comma-separated
    involved teams (owners of the changed files) to stdout.

    - config-dir: the SMP target config dir (e.g. test/regression).
    - changed-files: path to a file listing changed files, one per line (empty => no involved teams).
    - exclude: comma-separated paths relative to config-dir to skip (e.g. ebpf).
    """
    files = []
    if changed_files:
        with open(changed_files) as f:
            files = [line.strip() for line in f if line.strip()]
    excludes = [e.strip() for e in exclude.split(',') if e.strip()]

    involved, ownership = smp_inputs_impl(config_dir, files, excludes, owners_file)

    with open(ownership_out, 'w') as out:
        json.dump(ownership, out, indent=2, sort_keys=True)

    print(','.join(involved))


def resolve_run_set_impl(
    config_dir: str, smp_bin: str, changed_files: list[str], labels: str, runner: str, exclude: list[str], owners_file: str
) -> str:
    """Resolve the experiment run set to a `--experiment-path-filter` value.

    Combines the CODEOWNERS-derived inputs (`smp_inputs_impl`) with
    `<smp_bin> experiments resolve ... --format path-filter`. Returns the comma-separated leaf paths
    (empty string if nothing resolves). Raises Exit if `smp experiments resolve` fails.
    """
    import subprocess
    import tempfile

    involved, ownership = smp_inputs_impl(config_dir, changed_files, exclude, owners_file)

    tf = tempfile.NamedTemporaryFile('w', suffix='.json', delete=False)
    try:
        json.dump(ownership, tf)
        tf.close()
        cmd = [
            smp_bin, 'experiments', 'resolve',
            '--target-config-dir', config_dir,
            '--runner', runner,
            '--ownership', tf.name,
            '--format', 'path-filter',
        ]
        if exclude:
            cmd += ['--exclude-path', ','.join(exclude)]
        if involved:
            cmd += ['--involved-team', ','.join(involved)]
        if labels:
            cmd += ['--label', labels]
        result = subprocess.run(cmd, capture_output=True, text=True)
    finally:
        os.unlink(tf.name)

    if result.returncode != 0:
        raise Exit(f"`smp experiments resolve` failed (exit {result.returncode}):\n{result.stderr}", code=1)
    return result.stdout.strip()


@task
def smp_resolve(
    _,
    config_dir,
    smp_bin,
    changed_files='',
    labels='',
    runner='container',
    exclude='',
    out='smp_experiment_path_filter.txt',
    owners_file='.github/CODEOWNERS',
):
    """
    Resolve the SMP experiment run set for a PR and write the `--experiment-path-filter` value.

    Combines `owners.smp-inputs` (CODEOWNERS -> involved teams + ownership) with
    `<smp-bin> experiments resolve ... --format path-filter`, so the whole
    "changed files + labels -> what runs" decision is one command runnable locally against a
    locally-built smp binary. Writes the comma-separated leaf paths to `--out` (empty if nothing
    resolves) and prints them.

    - smp-bin: path to the smp binary (e.g. `./smp` in CI, or a local debug build).
    - changed-files: file listing the PR's changed files, one per line (empty => no involved teams).
    - labels: comma-separated labels applied to the PR.
    - runner: runner to resolve for (default container).
    - exclude: comma-separated paths relative to config-dir to skip, e.g. ebpf.
    """
    files = []
    if changed_files:
        with open(changed_files) as f:
            files = [line.strip() for line in f if line.strip()]
    excludes = [e.strip() for e in exclude.split(',') if e.strip()]

    paths = resolve_run_set_impl(config_dir, smp_bin, files, labels, runner, excludes, owners_file)

    with open(out, 'w') as f:
        f.write(paths)
    print(paths)
