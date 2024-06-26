from __future__ import annotations

import re
from urllib.parse import quote

PROJECT_NAME = "DataDog/datadog-agent"
PROJECT_TITLE = PROJECT_NAME.removeprefix("DataDog/")
CI_VISIBILITY_JOB_URL = 'https://app.datadoghq.com/ci/pipeline-executions?query=ci_level%3Ajob%20%40ci.pipeline.name%3ADataDog%2Fdatadog-agent%20%40git.branch%3Amain%20%40ci.job.name%3A{name}{extra_flags}&agg_m=count'
NOTIFICATION_DISCLAIMER = "If there is something wrong with the notification please contact #agent-devx-help"

AWS_S3_CP_CMD = "aws s3 cp --only-show-errors --region us-east-1 --sse AES256"
AWS_S3_LS_CMD = "aws s3api list-objects-v2 --bucket '{bucket}' --prefix '{prefix}/' --delimiter /"


def get_ci_visibility_job_url(name: str, prefix=True, extra_flags: list[str] | str = "") -> str:
    # Escape (https://docs.datadoghq.com/logs/explorer/search_syntax/#escape-special-characters-and-spaces)
    if prefix:
        # Cannot escape using double quotes for glob syntax
        name = re.sub(r"([-+=&|><!(){}[\]^\"“”~*?:\\ ])", r"\\\1", name) + '*'
        name = quote(name)
    else:
        name = quote(f'"{name}"')

    if isinstance(extra_flags, list):
        extra_flags = quote(''.join(' ' + flag for flag in extra_flags))

    return CI_VISIBILITY_JOB_URL.format(name=name, extra_flags=extra_flags)
