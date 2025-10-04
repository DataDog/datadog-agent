"""Manages authentication to services."""

import os

from invoke import Exit, task

from tasks.libs.ciproviders.github_api import generate_local_github_token
from tasks.libs.ciproviders.gitlab_api import get_gitlab_repo, get_gitlab_token
from tasks.libs.common.auth import datadog_infra_token
from tasks.libs.common.utils import running_in_ci


@task
def datadog_infra(ctx, audience, datacenter="us1.ddbuild.io"):
    """Returns the http token for the given audience."""

    token = datadog_infra_token(ctx, audience, datacenter)

    print(token)


@task
def gitlab(ctx, repo='datadog-agent', verbose=False):
    """Get a gitlab token."""

    if running_in_ci():
        raise Exit(message='This task is meant to be run locally, not in CI', code=1)

    if "GITLAB_TOKEN" in os.environ:
        print(os.environ["GITLAB_TOKEN"])
    else:
        print(get_gitlab_token(ctx, repo, verbose))


@task
def test_gitlab(ctx, repo='datadog-agent', n=100):
    import time
    import sys

    if "GITLAB_TOKEN" in os.environ:
        print('Warning: GITLAB_TOKEN is set in the environment')

    n_errors = 0
    reponame = 'DataDog/' + repo
    for i in range(n):
        print(f'#{i+1}/{n}')
        try:
            repo = get_gitlab_repo(reponame)
            id = 78266844
            p = repo.pipelines.get(id, lazy=False)
            print('Got pipeline', p.status)
        except Exception:
            import traceback
            traceback.print_exc()
            n_errors += 1

        print('--------------------')
        sys.stdout.flush()

    print(f'Total errors: {n_errors}/{n}')


@task
def github(ctx):
    """Get a github token."""

    if running_in_ci():
        raise Exit(message='This task is meant to be run locally, not in CI', code=1)

    if "GITHUB_TOKEN" in os.environ:
        print(os.environ["GITHUB_TOKEN"])
    else:
        print(generate_local_github_token(ctx))
