# Used to interact with agent-ci-api service

import json
import os
import subprocess

from invoke import task


def get_datacenter(env):
    if env == 'prod':
        return 'us1.ddbuild.io'
    elif env == 'staging':
        return 'us1.staging.dog'
    else:
        raise ValueError(f'Unknown environment: {env}. Supported environments are: prod, staging.')


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
    silent_curl=True,
):
    """Triggers the agent-ci-api service.

    Args:
        env: One of 'prod', 'staging' or 'local'.
        jq: One of 'no', 'auto' or 'yes'. Will pipe the json result to jq for pretty printing if jq present.
        ty: The RAPID type.
        attrs: The RAPID attributes.

    Example:
        $ dda inv -- api stackcleaner/workflow --env staging --ty stackcleaner_workflow_request --attrs debug_job_name=abc,debug_job_id=123,debug_pipeline_id=1234,debug_ref=cc
    """

    assert env in ('prod', 'staging', 'local'), (
        f'Unknown environment: {env}. Supported environments are: prod, staging, local.'
    )
    if payload or ty or attrs:
        assert not (payload and (ty or attrs)), 'Cannot specify payload and type/attributes at the same time.'

    # Construct the payload
    if ty or attrs:
        assert ty and attrs, 'Type and attributes must be specified together.'

        all_attrs = {kv.split('=')[0]: '='.join(kv.split('=')[1:]) for kv in attrs.split(',')}
        body = {
            'data': {
                'type': ty,
                'attributes': all_attrs,
            }
        }
        payload = json.dumps(body)

    method = method or ('get' if payload == '' else 'post')
    has_jq = ctx.run('which jq', hide=True, warn=True).ok
    is_local = env == 'local'
    from_ci = 'CI_JOB_ID' in os.environ

    token = (
        "$(authanywhere)"
        if from_ci
        else '"$(ddtool auth token rapid-agent-devx --datacenter us1.staging.dog --http-header)"'
    )
    extra_header = '"X-DdOrigin: curl-authanywhere"' if from_ci else '"X-DdOrigin: curl-local"'

    url = (
        f"http://localhost:{localport}/{prefix}{endpoint}"
        if is_local
        else f"https://agent-ci-api.{get_datacenter(env)}/{prefix}{endpoint}"
    )
    silent = '-s' if silent_curl else ''

    if payload:
        payload = f'-d \'{payload}\''

    cmd = f'curl {silent} -X {method.upper()} {url} -H {token} -H {extra_header} {payload}'

    result = ctx.run(cmd, hide=True)
    if not result.ok:
        raise RuntimeError(f'Command failed with exit code {result.exited}:\n{cmd}\n{result.stderr}')
    elif jq == 'yes' or (jq == 'auto' and has_jq):
        jq_result = subprocess.run(['jq', '-C', '.'], input=result.stdout, text=True)

        # Jq parsing failed, ignore (otherwise everything is already printed)
        if jq_result.returncode != 0:
            print(result.stdout)
