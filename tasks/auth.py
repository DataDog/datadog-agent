"""Manages authentication to services."""

from invoke import task

from tasks.libs.common.utils import running_in_ci


@task
def datadog_infra(ctx, audience, datacenter="us1.ddbuild.io", verbose=False):
    """Returns the http token for the given audience."""

    token = (
        ctx.run(
            f'authanywhere --audience {audience}'
            if running_in_ci()
            else f'ddtool auth token {audience} --datacenter "{datacenter}" --http-header',
            hide=True,
        )
        .stdout.strip()
        .removeprefix('Authorization: ')
    )

    if verbose:
        print(token)

    return token


@task
def github(ctx, verbose=False) -> str:
    """Get a github token."""
