"""
Helper for generating links to CI Visibility and CI Test Visibility
"""

from urllib.parse import quote

from tasks.libs.ciproviders.gitlab_api import BASE_URL as GITLAB_BASE_URL

CI_VISIBILITY_BASE_URL = "https://app.datadoghq.com/ci"
CI_VISIBILITY_URL = f"{CI_VISIBILITY_BASE_URL}/pipeline-executions"
TEST_VISIBILITY_URL = f"{CI_VISIBILITY_BASE_URL}/test-runs"


def get_pipeline_link_to_job_id(job_id: str):
    query_params = {
        "ci_level": "job",
        "@ci.job.id": job_id,
        "@ci.pipeline.name": "DataDog/datadog-agent",
    }
    query_string = to_query_string(query_params)
    quoted_query_string = quote(string=query_string, safe="")
    return f"{CI_VISIBILITY_URL}?query={quoted_query_string}"


def get_pipeline_link_to_job_on_main(job_name: str):
    # explicitly escape double quotes
    job_name = job_name.replace('"', '\\"')
    query_params = {
        "ci_level": "job",
        # wrapping in double quotes
        "@ci.job.name": f'"{job_name}"',
        "@git.branch": "main",
        "@ci.pipeline.name": "DataDog/datadog-agent",
    }
    query_string = to_query_string(query_params)
    quoted_query_string = quote(string=query_string, safe="")
    return f"{CI_VISIBILITY_URL}?query={quoted_query_string}"


def get_test_link_to_job_id(job_id: str):
    query_params = {
        "test_level": "test",
        "@ci.job.url": f"\"{GITLAB_BASE_URL}/DataDog/datadog-agent/-/jobs/{job_id}\"",
        "@test.service": "datadog-agent",
    }
    query_string = to_query_string(query_params)
    quoted_query_string = quote(string=query_string, safe="")
    return f"{TEST_VISIBILITY_URL}?query={quoted_query_string}"


def get_test_link_to_job_on_main(job_name: str):
    job_name = job_name.replace('"', '\\"')
    query_params = {
        "test_level": "test",
        # wrapping in double quotes
        "@ci.job.name": f'"{job_name}"',
        "@git.branch": "main",
        "@test.service": "datadog-agent",
    }
    query_string = to_query_string(query_params)
    quoted_query_string = quote(string=query_string, safe="")
    return f"{TEST_VISIBILITY_URL}?query={quoted_query_string}"


def to_query_string(params: dict):
    # we cannot use `urllib.parse.urlencode` as the ci visibility query parameter
    # format expects `key:value` items separated by space
    # and `urlib.parse.urlencode` uses `key=value`
    return " ".join(f"{k}:{v}" for k, v in params.items())
