"""Manages authentication to services."""

import requests
from invoke import task

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

    infra_token = datadog_infra(ctx, audience="sdm")
    url = f"https://bti-ci-api.us1.ddbuild.io/internal/ci/gitlab/token?owner=DataDog&repository={repo}"

    res = requests.get(url, headers={'Authorization': infra_token}, timeout=10)

    if not res.ok:
        raise RuntimeError(f'Failed to retrieve Gitlab token, request failed with code {res.status_code}:\n{res.text}')

    token = res.json()['token']

    if verbose:
        print(token)

    return token
