from __future__ import annotations

import os
import re
import sys
from glob import glob
from tempfile import TemporaryDirectory

import yaml
from invoke import Exit, task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import (
    is_leaf_job,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.utils import gitlab_section, is_pr_context

from .gitlab import get_gitlab_ci_lintable_jobs

# - SC2086 corresponds to using variables in this way $VAR instead of "$VAR" (used in every jobs).
# - SC2016 corresponds to avoid using '$VAR' inside single quotes since it doesn't expand.
# - SC2046 corresponds to avoid using $(...) to prevent word splitting.
DEFAULT_SHELLCHECK_EXCLUDES = 'SC2059,SC2028,SC2086,SC2016,SC2046'


@task
def releasenote(ctx):
    """Lints release notes with Reno."""

    branch = os.environ.get("BRANCH_NAME")
    pr_id = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_id, "release note")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        if github.is_release_note_needed(pr_id):
            if not github.contains_release_note(pr_id):
                print(
                    f"{color_message('Error', 'red')}: No releasenote was found for this PR. Please add one using 'reno'"
                    ", see https://datadoghq.dev/datadog-agent/guidelines/contributing/#reno"
                    ", or apply the label 'changelog/no-changelog' to the PR.",
                    file=sys.stderr,
                )
                raise Exit(code=1)
            ctx.run("reno lint")
        else:
            print("'changelog/no-changelog' label found on the PR: skipping linting")


def flatten_script(script: str | list[str]) -> str:
    """Flatten a script into a single string."""

    if isinstance(script, list):
        return '\n'.join(flatten_script(line) for line in script)

    if script is None:
        return ''

    return script.strip()


def shellcheck_linter(
    ctx,
    scripts: dict[str, str],
    exclude: str,
    shellcheck_args: str,
    fail_fast: bool,
    use_bat: str | None,
    only_errors=False,
):
    """Lints bash scripts within `scripts` using shellcheck.

    Args:
        scripts: A dictionary of job names and their scripts.
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        use_bat: If True (or None), will (try to) use bat to display the script.
        only_errors: Show only errors, not warnings.

    Note:
        Will raise an Exit if any errors are found.
    """

    exclude = ' '.join(f'-e {e}' for e in exclude.split(','))

    if use_bat is None:
        use_bat = ctx.run('which bat', warn=True, hide=True)
    elif use_bat.casefold() == 'false':
        use_bat = False

    results = {}
    with TemporaryDirectory() as tmpdir:
        for i, (script_name, script) in enumerate(scripts.items()):
            with open(f'{tmpdir}/{i}.sh', 'w') as f:
                f.write(script)

            res = ctx.run(f"shellcheck {shellcheck_args} {exclude} '{tmpdir}/{i}.sh'", warn=True, hide=True)
            if res.stderr or res.stdout:
                if res.return_code or not only_errors:
                    results[script_name] = {
                        'output': (res.stderr + '\n' + res.stdout + '\n').strip(),
                        'code': res.return_code,
                        'id': i,
                    }

                if res.return_code and fail_fast:
                    break

        if results:
            with gitlab_section(color_message("Shellcheck errors / warnings", color=Color.ORANGE), collapsed=True):
                for script, result in sorted(results.items()):
                    with gitlab_section(f"Shellcheck errors for {script}"):
                        print(f"--- {color_message(script, Color.BLUE)} ---")
                        print(f'[{script}] Script:')
                        if use_bat:
                            res = ctx.run(
                                f"bat --color=always --file-name={script} -l bash {tmpdir}/{result['id']}.sh", hide=True
                            )
                            # Avoid buffering issues
                            print(res.stderr)
                            print(res.stdout)
                        else:
                            with open(f'{tmpdir}/{result["id"]}.sh') as f:
                                print(f.read())
                        print(f'\n[{script}] {color_message("Error", Color.RED)}:')
                        print(result['output'])

            if any(result['code'] != 0 for result in results.values()):
                raise Exit(
                    f"{color_message('Error', Color.RED)}: {len(results)} shellcheck errors / warnings found, please fix them",
                    code=1,
                )


@task
def gitlab_ci_shellcheck(
    ctx,
    diff_file=None,
    config_file=None,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast=False,
    verbose=False,
    use_bat=None,
    only_errors=False,
):
    """Verifies that shell scripts with gitlab config are valid.

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config.
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config.
    """

    # Used by the CI to skip linting if no changes
    if diff_file and not os.path.exists(diff_file):
        print('No diff file found, skipping lint')
        return

    jobs, full_config = get_gitlab_ci_lintable_jobs(diff_file, config_file)

    # No change, info already printed in get_gitlab_ci_lintable_jobs
    if not full_config:
        return

    scripts = {}
    for job, content in jobs:
        # Skip jobs that are not executed
        if not is_leaf_job(job, content):
            continue

        # Shellcheck is only for bash like scripts
        is_powershell = any(
            'powershell' in flatten_script(content.get(keyword, ''))
            for keyword in ('before_script', 'script', 'after_script')
        )
        if is_powershell:
            continue

        if verbose:
            print('Verifying job:', job)

        # Lint scripts
        for keyword in ('before_script', 'script', 'after_script'):
            if keyword in content:
                scripts[f'{job}.{keyword}'] = f'#!/bin/bash\n{flatten_script(content[keyword]).strip()}\n'

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, use_bat, only_errors)


@task
def github_actions_shellcheck(
    ctx,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast=False,
    use_bat=None,
    only_errors=False,
    all_files=False,
):
    """Lint github action workflows with shellcheck."""

    if all_files:
        files = glob('.github/workflows/*.yml')
    else:
        files = ctx.run(
            "git diff --name-only \"$(git merge-base main HEAD)\" | grep -E '.github/workflows/.*\\.yml'", warn=True
        ).stdout.splitlines()

    if not files:
        print('No github action workflow files to lint, skipping')
        return

    scripts = {}
    for file in files:
        with open(file) as f:
            workflow = yaml.safe_load(f)

        for job_name, job in workflow.get('jobs').items():
            for i, step in enumerate(job['steps']):
                step_name = step.get('name', f'step-{i + 1:02d}').replace(' ', '_')
                if 'run' in step:
                    script = step['run']
                    if isinstance(script, list):
                        script = '\n'.join(script)

                    # "Escape" ${{...}} which is github actions only syntax
                    script = re.sub(r'\${{(.*)}}', r'\\$\\{\\{\1\\}\\}', script, flags=re.MULTILINE)

                    # We suppose all jobs are bash like scripts and not powershell or other exotic shells
                    script = '#!/bin/bash\n' + script.strip() + '\n'
                    scripts[f'{file.removeprefix(".github/workflows/")}-{job_name}-{step_name}'] = script

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, use_bat, only_errors)
