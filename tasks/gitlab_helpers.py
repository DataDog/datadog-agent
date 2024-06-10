"""
Helper for generating links to CI Visibility for Gitlab CI jobs
"""

import json
import os
from urllib.parse import quote

from invoke import task

CI_VISIBILITY_URL = "https://app.datadoghq.com/ci/pipeline-executions"


@task
def generate_ci_visibility_links(_ctx, output: str | None):
    """
    Generate links to CI Visibility for the current job
    usage deva gitlab.generate-ci-visibility-links
    Generated file
    """
    ci_job_id = os.environ.get("CI_JOB_ID")
    if ci_job_id is None:
        print(color_message("CI_JOB_ID is not set, this task can run only on Gitlab, skipping...", Color.RED))
        return
    ci_job_name = os.environ.get("CI_JOB_NAME")
    if ci_job_name is None:
        print(color_message("CI_JOB_NAME is not set, this task can run only on Gitlab, skipping...", Color.RED))
        return

    gitlab_annotations_report = {
        "CI Visibility": [
            {
                "external_link": {
                    "label": "CI Visibility: This job instance",
                    "url": get_link_to_job_id(ci_job_id),
                }
            },
            {
                "external_link": {
                    "label": "CI Visibility: This job on main",
                    "url": get_link_to_job_on_main(ci_job_name),
                }
            },
        ]
    }

    content = json.dumps(gitlab_annotations_report, indent=2)

    annotation_file_name = output or f"external_links_{ci_job_id}.json"
    with open(annotation_file_name, "w") as f:
        f.write(content)
    print(f"Generated {annotation_file_name}")


def get_link_to_job_id(job_id: str):
    query_params = {
        "ci_level": "job",
        "@ci.job.id": job_id,
        "@ci.pipeline.name": "DataDog/datadog-agent",
    }
    return f"{CI_VISIBILITY_URL}?query={quote(to_query_string(query_params))}"


def get_link_to_job_on_main(job_name: str):
    # explicitly escape double quotes
    job_name = job_name.replace('"', '\\"')
    query_params = {
        "ci_level": "job",
        # wrapping in double quotes
        "@ci.job.name": f'"{job_name}"',
        "@git.branch": "main",
        "@ci.pipeline.name": "DataDog/datadog-agent",
    }
    return f"{CI_VISIBILITY_URL}?query={quote(to_query_string(query_params))}"


def to_query_string(params: dict):
    return " ".join(f"{k}:{v}" for k, v in params.items())
