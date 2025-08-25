"""Manages authentication to services."""

from invoke import task

from tasks.libs.ciproviders.gitlab_api import get_gitlab_token
from tasks.libs.common.auth import datadog_infra_token


@task
def datadog_infra(ctx, audience, datacenter="us1.ddbuild.io"):
    """Returns the http token for the given audience."""

    token = datadog_infra_token(ctx, audience, datacenter)

    print(token)

    return token


@task
def gitlab(ctx, repo='datadog-agent', verbose=False):
    """Get a gitlab token."""

    print(get_gitlab_token(ctx, repo, verbose))
