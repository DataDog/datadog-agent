"""Manages authentication to services."""

from invoke import task

from tasks.libs.common.auth import datadog_infra_token


@task
def datadog_infra(ctx, audience, datacenter="us1.ddbuild.io"):
    """Returns the http token for the given audience."""

    token = datadog_infra_token(ctx, audience, datacenter)

    print(token)
