# Used to interact with agent-ci-api service

import os
import shutil
import subprocess

import requests
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
    payload=None,
    env='prod',
    localport=8080,
    prefix='internal/agent-ci-api/',
    jq='auto',
    query='.errors // .data',
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
        payload = {
            'data': {
                'type': ty,
                'attributes': all_attrs,
            }
        }

    method = method or ('get' if payload is None else 'post')
    has_jq = shutil.which('jq') is not None

    is_local = env == 'local'
    from_ci = 'CI_JOB_ID' in os.environ
    dc = None if is_local else get_datacenter(env)

    token = ''
    if not is_local:
        token = (
            ctx.run(
                'authanywhere --audience rapid-agent-devx'
                if from_ci
                else f'ddtool auth token rapid-agent-devx --datacenter {dc} --http-header',
                hide=True,
            )
            .stdout.strip()
            .removeprefix('Authorization: ')
        )
    origin_header = os.environ["CI_JOB_ID"] if from_ci else "curl-authanywhere"
    url = (
        f"http://localhost:{localport}/{prefix}{endpoint}"
        if is_local
        else f"https://agent-ci-api.{dc}/{prefix}{endpoint}"
    )
    use_jq = jq == 'yes' or (jq == 'auto' and has_jq and not from_ci)

    url = (
        f"http://localhost:{localport}/{prefix}{endpoint}"
        if is_local
        else f"https://agent-ci-api.{dc}/{prefix}{endpoint}"
    )
    result = requests.request(
        method.upper(),
        url,
        json=payload,
        headers={
            'Authorization': token,
            'X-DdOrigin': origin_header,
            'Content-Type': 'application/json',
        },
    )
    if not result.ok:
        raise RuntimeError(f'Request failed with code {result.status_code}:\n{result.text}')
    elif use_jq:
        jq_result = subprocess.run(['jq', '-C', query], input=result.text, text=True)

        # Jq parsing failed, ignore (otherwise everything is already printed)
        if jq_result.returncode != 0:
            print(result.text)


@task
def hello(ctx, env='prod'):
    """Verifies that the agent-ci-api service is running."""

    return run(ctx, endpoint='hello', env=env)
