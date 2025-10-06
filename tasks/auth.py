"""Manages authentication to services."""

import os

from invoke import Exit, task

from tasks.libs.ciproviders.github_api import generate_local_github_token
from tasks.libs.ciproviders.gitlab_api import get_gitlab_token
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

    print(get_gitlab_token(ctx, repo, verbose))

    # if running_in_ci():
    #     raise Exit(message='This task is meant to be run locally, not in CI', code=1)

    # if "GITLAB_TOKEN" in os.environ:
    #     print(os.environ["GITLAB_TOKEN"])
    # else:
    #     print(get_gitlab_token(ctx, repo, verbose))


@task
def github(ctx):
    """Get a github token."""

    if running_in_ci():
        raise Exit(message='This task is meant to be run locally, not in CI', code=1)

    if "GITHUB_TOKEN" in os.environ:
        print(os.environ["GITHUB_TOKEN"])
    else:
        print(generate_local_github_token(ctx))
