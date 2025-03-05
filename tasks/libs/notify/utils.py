from __future__ import annotations

import os
import re
from typing import Any
from urllib.parse import quote

from tasks.libs.pipeline.notifications import HELP_SLACK_CHANNEL

PROJECT_NAME = "DataDog/datadog-agent"
CI_VISIBILITY_JOB_URL = 'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20%40ci.job.name%3A{name}{extra_flags}&agg_m=count{extra_args}'
NOTIFICATION_DISCLAIMER = f"If there is something wrong with the notification please contact {HELP_SLACK_CHANNEL}"
CHANNEL_BROADCAST = '#agent-devx-ops'
PIPELINES_CHANNEL = '#datadog-agent-pipelines'
DEPLOY_PIPELINES_CHANNEL = '#datadog-agent-deploy-pipelines'
AWS_S3_CP_CMD = "aws s3 cp --only-show-errors --region us-east-1 --sse AES256"
AWS_S3_LS_CMD = "aws s3api list-objects-v2 --bucket '{bucket}' --prefix '{prefix}/' --delimiter /"


def get_ci_visibility_job_url(
    name: str, prefix=True, extra_flags: list[str] | str = "", extra_args: dict[str, Any] | str = ''
) -> str:
    """
    Returns the link to a query matching the job (or its prefix by default) in ci visibility

    - prefix: Match same prefixes but not the whole job name (adds '*' at the end)
    - extra_flags: List of flags to add to the query (e.g. status:error, -@error.domain:provider)
    - extra_args: List of arguments to add to the URI (e.g. start=..., paused=true)
    """
    # Escape (https://docs.datadoghq.com/logs/explorer/search_syntax/#escape-special-characters-and-spaces)
    fully_escaped = re.sub(r"([-+=&|><!(){}[\]^\"“”~*?:\\ ])", r"\\\1", name)

    if prefix:
        # Cannot escape using double quotes for glob syntax
        name = fully_escaped + '*'
        name = quote(name)
    elif '"' not in name and '\\' not in name:
        name = quote(f'"{name}"')
    else:
        name = quote(fully_escaped)

    if isinstance(extra_flags, list):
        extra_flags = quote(''.join(' ' + flag for flag in extra_flags))

    if isinstance(extra_args, dict):
        extra_args = ''.join([f'&{key}={value}' for key, value in extra_args.items()])

    return CI_VISIBILITY_JOB_URL.format(name=name, extra_flags=extra_flags, extra_args=extra_args)


def should_notify(pipeline_id):
    """
    Check if the pipeline should notify the channel: only for non-downstream pipelines, unless conductor triggered it
    """
    from tasks.libs.ciproviders.gitlab_api import get_pipeline

    pipeline = get_pipeline(PROJECT_NAME, pipeline_id)
    return pipeline.source != 'pipeline' or pipeline.source == 'pipeline' and 'DDR_WORKFLOW_ID' in os.environ


def get_pipeline_type():
    """
    Return the type of notification to send (related to the type of pipeline, amongst 'deploy', 'trigger' and 'merge')
    """
    if os.environ.get('DEPLOY_AGENT', '') == 'true':
        return 'deploy'
    elif os.environ.get('TRIGGERED_PIPELINE', 'false') == 'true' or 'DDR_WORKFLOW_ID' in os.environ:
        return 'trigger'
    else:
        return 'merge'
