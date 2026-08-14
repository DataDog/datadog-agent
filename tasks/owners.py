import json
import os
import subprocess
import tempfile
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


def _discover_experiments(config_dir: str, manifest: str, smp_bin: str, exclude: list[str]) -> list[str]:
    """Discover experiment paths (relative to `config_dir`) via `smp experiments list`.

    Sourcing discovery from the CLI keeps SMP the single discovery authority — no duplicated os.walk
    that could drift from SMP's rules. Returns sorted unique experiment paths.
    """
    cmd = [
        smp_bin,
        'experiments',
        'list',
        '--target-config-dir',
        config_dir,
        '--manifest',
        manifest,
        '--format',
        'json',
    ]
    if exclude:
        cmd += ['--exclude-path', ','.join(exclude)]
    result = subprocess.run(cmd, capture_output=True, text=True)
    if result.returncode != 0:
        raise Exit(f"`smp experiments list` failed (exit {result.returncode}):\n{result.stderr}", code=1)

    experiments = {exp['path'] for exp in json.loads(result.stdout or '[]') if exp.get('path')}
    return sorted(experiments)


def _experiment_ownership(experiments: list[str], config_dir: str, owners_file: str) -> dict[str, list[str]]:
    """Map each experiment path to its owning team slugs, resolved from CODEOWNERS.

    Ownership is resolved at each experiment's own path (not the group folder above `cases/`), so
    per-experiment CODEOWNERS overrides are honored — a co-owned experiment inside an otherwise
    SMP-owned folder gets its real owners. Folder-level delegation still applies via normal CODEOWNERS
    precedence (a folder rule matches every experiment path under it).
    """
    base = config_dir.rstrip('/')
    return {exp: _team_slugs(search_owners(f"{base}/{exp}/", owners_file)) for exp in experiments}


def smp_inputs_impl(
    config_dir: str, manifest: str, smp_bin: str, changed_files: list[str], exclude: list[str], owners_file: str
):
    """Compute the CODEOWNERS-derived inputs for `smp experiments resolve`.

    Returns `(involved_teams, ownership)`:
    - `involved_teams`: sorted team slugs owning any of `changed_files` (pure CODEOWNERS).
    - `ownership`: `{experiment_path: [team_slug, ...]}` for every experiment SMP discovers (minus
      `exclude`). Experiment paths come from `smp experiments list`, so discovery has a single source
      of truth, and ownership is keyed per experiment so per-experiment CODEOWNERS overrides are kept.
    """
    involved = _team_slugs(make_partition(changed_files, owners_file).keys()) if changed_files else []
    ownership = _experiment_ownership(
        _discover_experiments(config_dir, manifest, smp_bin, exclude), config_dir, owners_file
    )
    return involved, ownership


@task
def smp_inputs(
    _,
    config_dir,
    smp_bin,
    manifest='test/regression/selection.yaml',
    changed_files='',
    exclude='',
    ownership_out='ownership.json',
    owners_file='.github/CODEOWNERS',
):
    """
    Emit the CODEOWNERS-derived inputs for `smp experiments resolve`.

    Writes the experiment -> owning-teams map to `--ownership-out` (JSON) and prints the comma-separated
    involved teams (owners of the changed files) to stdout. Experiments are discovered via
    `<smp-bin> experiments list` (single discovery source).

    - config-dir: the SMP target config dir (e.g. test/regression).
    - smp-bin: path to the smp binary.
    - manifest: path to the selection manifest (default test/regression/selection.yaml).
    - changed-files: path to a file listing changed files, one per line (empty => no involved teams).
    - exclude: comma-separated paths relative to config-dir to skip (e.g. ebpf).
    """
    files = []
    if changed_files:
        with open(changed_files) as f:
            files = [line.strip() for line in f if line.strip()]
    excludes = [e.strip() for e in exclude.split(',') if e.strip()]

    involved, ownership = smp_inputs_impl(config_dir, manifest, smp_bin, files, excludes, owners_file)

    with open(ownership_out, 'w') as out:
        json.dump(ownership, out, indent=2, sort_keys=True)

    print(','.join(involved))


def resolve_run_set_impl(
    config_dir: str,
    manifest: str,
    smp_bin: str,
    changed_files: list[str],
    labels: str,
    runner: str,
    exclude: list[str],
    owners_file: str,
) -> str:
    """Resolve the experiment run set to a `--experiment-path-filter` value.

    Combines the CODEOWNERS-derived inputs (`smp_inputs_impl`) with
    `<smp_bin> experiments resolve --manifest <manifest> ... --format path-filter`. The manifest holds
    the selection policy (always/codeowners/labels buckets); involvement + ownership still come from
    CODEOWNERS. Returns the comma-separated experiment paths (empty string if nothing resolves). Raises
    Exit if `smp experiments resolve` fails.
    """
    involved, ownership = smp_inputs_impl(config_dir, manifest, smp_bin, changed_files, exclude, owners_file)

    tf = tempfile.NamedTemporaryFile('w', suffix='.json', delete=False)
    try:
        json.dump(ownership, tf)
        tf.close()
        cmd = [
            smp_bin,
            'experiments',
            'resolve',
            '--target-config-dir',
            config_dir,
            '--manifest',
            manifest,
            '--runner',
            runner,
            '--ownership',
            tf.name,
            '--format',
            'path-filter',
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
    manifest='test/regression/selection.yaml',
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
    `<smp-bin> experiments resolve --manifest <manifest> ... --format path-filter`, so the whole
    "changed files + labels -> what runs" decision is one command runnable locally against a
    locally-built smp binary. Writes the comma-separated experiment paths to `--out` (empty if nothing
    resolves) and prints them.

    - smp-bin: path to the smp binary (e.g. `./smp` in CI, or a local debug build).
    - manifest: path to the selection manifest (default test/regression/selection.yaml).
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

    paths = resolve_run_set_impl(config_dir, manifest, smp_bin, files, labels, runner, excludes, owners_file)

    with open(out, 'w') as f:
        f.write(paths)
    print(paths)


def smp_pr_context_impl(branch: str, repository: str = "DataDog/datadog-agent"):
    """Resolve the open PR for `branch`; return `(labels_csv, changed_files)` (empty if no open PR).

    Uses `GITHUB_TOKEN` from the environment (mint it via `dd-octo-sts` first). Keeps the GitHub API
    calls in Python (via `GithubAPI`) rather than as curl/jq in the CI shell.
    """
    from tasks.libs.ciproviders.github_api import GithubAPI

    gh = GithubAPI(repository)
    prs = list(gh.get_pr_for_branch(head_branch_name=branch))
    if not prs:
        return "", []
    pr = prs[0]
    return ",".join(gh.get_pr_labels(pr.number)), gh.get_pr_files(pr.number)


@task
def smp_pr_context(
    _,
    branch,
    changed_files_out='changed_files.txt',
    labels_out='pr_labels.txt',
    repository='DataDog/datadog-agent',
):
    """
    Resolve the open PR for `branch` and write its applied labels + changed files, for SMP selection.

    Reads `GITHUB_TOKEN` from the environment (mint via `dd-octo-sts` first). Writes changed files (one
    per line) to `--changed-files-out` and the comma-separated applied labels to `--labels-out`; both
    are empty when no open PR is found (the resolver then defaults). This keeps the GitHub API calls
    out of the CI shell.
    """
    labels, files = smp_pr_context_impl(branch, repository)
    with open(changed_files_out, 'w') as f:
        f.write("\n".join(files))
    with open(labels_out, 'w') as f:
        f.write(labels)
    print(f"labels=[{labels}] changed_files={len(files)}")
