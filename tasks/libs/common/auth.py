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
