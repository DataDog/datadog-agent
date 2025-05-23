"""GitHub-related linting tasks (PRs and GitHub Actions)"""

import os
import re
import sys
from glob import glob

import yaml
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.common.color import color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.utils import is_pr_context

from .shell import DEFAULT_SHELLCHECK_EXCLUDES, shellcheck_linter


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
