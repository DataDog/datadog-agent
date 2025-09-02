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
