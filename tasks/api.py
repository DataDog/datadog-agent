# Used to interact with agent-ci-api service

import json
import os
import shutil
import subprocess

from invoke import task


def get_datacenter(env):
    if env == 'prod':
        return 'us1.ddbuild.io'
    elif env == 'staging':
        return 'us1.staging.dog'
    else:
        raise ValueError(f'Unknown environment: {env}. Supported environments are: prod, staging.')


def cast_type(value: str):
    """We can cast types using a type prefix, will parse this prefix."""

    if value.startswith('int:'):
        return int(value[4:])
    elif value.startswith('float:'):
        return float(value[6:])
    elif value.startswith('bool:'):
        return bool(value[5:])
    else:
        return value


@task(default=True)
def run(
    ctx,
    endpoint,
    method='',
    ty='',
    attrs='',
    payload='',
    env='prod',
    localport=8080,
    prefix='internal/agent-ci-api/',
    jq='auto',
    query='.errors // .data',
    silent_curl=True,
    dry_run=False,
):
    """Triggers the agent-ci-api service.

    Args:
        endpoint: The endpoint name (see `prefix` for full endpoint).
        method: The HTTP method to use (default is 'get' if payload is empty, otherwise 'post').
        ty: The RAPID type.
        attrs: The RAPID attributes.
        payload: Raw payload to send. Prefer using `ty` and `attrs` if possible.
        env: One of 'prod', 'staging' or 'local'.
        localport: The port to use for the local endpoint.
        prefix: The prefix to use for the endpoint.
        jq: One of 'no', 'auto' or 'yes'. Will pipe the json result to jq for pretty printing if jq present.
        query: jq query for the output.
        silent_curl: If True, will silence curl verbose output.
        dry_run: If True, will not execute the command but print it instead.

    Examples:
        $ dda inv -- api hello --env staging
        $ dda inv -- api hello --env local
        # Note that we can cast types using a type prefix, e.g. `int:`, `float:` or `bool:`
        $ dda inv -- api stackcleaner/job --env staging --ty stackcleaner_workflow_request --attrs job_name=abc,job_id=123,pipeline_id=1234,ref=cc,ignore_lock=bool:true
    """

    assert env in (
        'prod',
        'staging',
        'local',
    ), f'Unknown environment: {env}. Supported environments are: prod, staging, local.'
    if payload or ty or attrs:
        assert not (payload and (ty or attrs)), 'Cannot specify payload and type/attributes at the same time.'

    # Construct the payload
    if ty or attrs:
        assert ty and attrs, 'Type and attributes must be specified together.'

        # From the comma separated attributes, we build a payload
        all_attrs = {kv.split('=')[0]: cast_type('='.join(kv.split('=')[1:])) for kv in attrs.split(',')}
        body = {
            'data': {
                'type': ty,
                'attributes': all_attrs,
            }
        }
        payload = json.dumps(body)

    method = method or ('get' if payload == '' else 'post')
    has_jq = shutil.which('jq') is not None

    is_local = env == 'local'
    from_ci = 'CI_JOB_ID' in os.environ
    dc = None if is_local else get_datacenter(env)

    token = ''
    if not is_local:
        token = (
            '-H "$(authanywhere --audience rapid-agent-devx)"'
            if from_ci
            else f'-H "$(ddtool auth token rapid-agent-devx --datacenter {dc} --http-header)"'
        )
    extra_header = f'"X-DdOrigin: {os.environ["CI_JOB_ID"]}"' if from_ci else '"X-DdOrigin: curl-authanywhere"'
    url = (
        f"http://localhost:{localport}/{prefix}{endpoint}"
        if is_local
        else f"https://agent-ci-api.{dc}/{prefix}{endpoint}"
    )
    silent = '-s' if silent_curl else ''
    use_jq = jq == 'yes' or (jq == 'auto' and has_jq and not from_ci)
    if payload:
        payload = f'-d \'{payload}\''

    cmd = f'curl {silent} -X {method.upper()} {url} {token} -H {extra_header} {payload}'

    if dry_run:
        print(f'Would run: {cmd}')
        return

    result = ctx.run(cmd, hide=use_jq)
    if not result.ok:
        raise RuntimeError(f'Command failed with exit code {result.exited}:\n{cmd}\n{result.stderr}')
    elif use_jq:
        jq_result = subprocess.run(['jq', '-C', query], input=result.stdout, text=True)

        # Jq parsing failed, ignore (otherwise everything is already printed)
        if jq_result.returncode != 0:
            print(result.stdout)


@task
def hello(ctx, env='prod'):
    """Verifies that the agent-ci-api service is running."""

    return run(ctx, endpoint='hello', env=env)
