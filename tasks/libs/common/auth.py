from tasks.libs.common.utils import running_in_ci


def datadog_infra_token(ctx, audience, datacenter="us1.ddbuild.io"):
    """Returns the http token for the given audience to access Datadog infrastructure services."""

    in_ci = running_in_ci()
    cmd = (
        f'authanywhere --audience {audience}'
        if in_ci
        else f'ddtool auth token {audience} --datacenter "{datacenter}" --http-header'
    )

    print(f"[AUTH DEBUG] Getting infra token for audience={audience}, in_ci={in_ci}, cmd={cmd}")

    try:
        result = ctx.run(cmd, hide=True, warn=True)
        print(f"[AUTH DEBUG] Command exit code: {result.return_code}")
        if result.return_code != 0:
            print(f"[AUTH DEBUG] Command failed with stderr: {result.stderr}")
            print(f"[AUTH DEBUG] Command stdout: {result.stdout}")
            raise RuntimeError(f"Failed to get infra token: {result.stderr}")
    except Exception as e:
        print(f"[AUTH DEBUG] Exception running command: {e}")
        raise

    token = result.stdout.strip().removeprefix('Authorization: ')
    print(f"[AUTH DEBUG] Token retrieved, starts with 'Bearer ': {token.startswith('Bearer ')}")

    assert token.startswith('Bearer ') and token.count(' ') == 1, (
        "`authanywhere` returned an invalid token."
        if running_in_ci()
        else "`ddtool auth token` returned an invalid token, it might be due to ddtool outdated. Please run `brew update && brew upgrade ddtool`."
    )

    return token
