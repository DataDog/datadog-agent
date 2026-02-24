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


def _extract_env_var(output: str, key: str) -> str:
    for raw_line in output.splitlines():
        line = raw_line.strip()
        if line.startswith("export "):
            line = line[len("export ") :]
        prefix = f"{key}="
        if not line.startswith(prefix):
            continue
        value = line[len(prefix) :].strip()
        if (value.startswith('"') and value.endswith('"')) or (value.startswith("'") and value.endswith("'")):
            value = value[1:-1]
        if not value:
            raise RuntimeError(f"Parsed empty value for {key} from dd-auth output")
        return value
    raise RuntimeError(f"Could not parse {key} from dd-auth output")


@contextmanager
def dd_auth_api_app_keys(ctx, domain: str, api_key_env: str = "DD_API_KEY", app_key_env: str = "DD_APP_KEY"):
    """
    Fetch API/App keys from dd-auth for a given domain and temporarily set them in env.
    """
    command = (
        f"dd-auth --domain {shlex.quote(domain)} "
        f"--api-key-env {shlex.quote(api_key_env)} "
        f"--app-key-env {shlex.quote(app_key_env)} "
        "--output"
    )
    res = ctx.run(command, hide=True, warn=True)
    if res.exited != 0:
        raise RuntimeError(f"dd-auth failed for domain {domain}: {res.stderr.strip()}")

    api_key = _extract_env_var(res.stdout, api_key_env)
    app_key = _extract_env_var(res.stdout, app_key_env)

    previous_api = os.environ.get(api_key_env)
    previous_app = os.environ.get(app_key_env)
    os.environ[api_key_env] = api_key
    os.environ[app_key_env] = app_key
    try:
        yield
    finally:
        if previous_api is None:
            os.environ.pop(api_key_env, None)
        else:
            os.environ[api_key_env] = previous_api
        if previous_app is None:
            os.environ.pop(app_key_env, None)
        else:
            os.environ[app_key_env] = previous_app
