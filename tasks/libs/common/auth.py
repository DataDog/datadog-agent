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

    return token
