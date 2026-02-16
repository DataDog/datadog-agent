import os
import shlex
from contextlib import contextmanager

from tasks.libs.common.utils import running_in_ci


def datadog_infra_token(ctx, audience, datacenter="us1.ddbuild.io"):
    """Returns the http token for the given audience to access Datadog infrastructure services."""

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

    assert token.startswith('Bearer ') and token.count(' ') == 1, (
        "`authanywhere` returned an invalid token."
        if running_in_ci()
        else "`ddtool auth token` returned an invalid token, it might be due to ddtool outdated. Please run `brew update && brew upgrade ddtool`."
    )

    return token


def get_aws_vault_env(ctx, account: str):
    """Returns the environment variables to authenticate with aws-vault for the given account."""
    if running_in_ci():
        raise RuntimeError("aws-vault is not meant to be used in CI")
    res = ctx.run(f"aws-vault export {account}", hide=True)
    env = {}
    for line in res.stdout.splitlines():
        if "=" in line:
            key, value = line.split("=", 1)
            env[key] = value
    return env



@contextmanager
def dd_auth_api_app_keys(ctx, domain: str):
    """
    Fetch API/App keys from dd-auth for a given domain and temporarily set them in env.
    """
    res = ctx.run(f"dd-auth --domain {shlex.quote(domain)} --output", hide=True, warn=True)
    if not res.ok:
        raise RuntimeError(f"dd-auth failed for domain {domain}")

    output_vars = {}
    for line in res.stdout.splitlines():
        if "=" in line:
            key, value = line.split("=", 1)
            output_vars[key] = value

    env_vars = ['DD_API_KEY', 'DD_APP_KEY']
    new_values, prev_values = {}, {}
    for env_var in env_vars:
        if env_var not in output_vars:
            raise RuntimeError(f"dd-auth output does not contain {env_var}")

        new_values[env_var] = output_vars[env_var]
        prev_values[env_var] = os.environ.get(env_var)

    for env_var in env_vars:
        os.environ[env_var] = new_values[env_var]

    try:
        yield
    finally:
        for env_var in env_vars:
            if prev_values[env_var] is None:
                os.environ.pop(env_var, None)
            else:
                os.environ[env_var] = prev_values[env_var]
