"""GitLab API helpers for the Jev e2e tooling.

Only what the Jev executor needs: the short-lived API token (same mechanism
as tasks/libs/ciproviders/gitlab_api.py: authanywhere 'sdm' audience in CI,
GITLAB_TOKEN or ddtool locally, exchanged via the bti-ci-api service) and
the pipeline jobs listing used to build the Jev test universe
(tasks/libs/dynamic_test/jev_selection.py).

The executed-tests side of the evaluation is NOT handled here: it is the
DynTestEvaluator's own CI Visibility lookup (tasks/libs/dynamic_test/evaluator.py).
"""

from __future__ import annotations  # python 3.9 compat

import json
import os
import urllib.request

from pr_context import run_cmd

# Base URL of the GitLab API; overridable for tests/other instances
_API_V4 = os.environ.get("CI_API_V4_URL", "https://gitlab.ddbuild.io/api/v4")
_PROJECT = os.environ.get("CI_PROJECT_ID") or "DataDog%2Fdatadog-agent"


def get_gitlab_token() -> str:
    """GitLab API token, for listing the pipeline's jobs.

    Same mechanism as tasks.libs.ciproviders.gitlab_api.get_gitlab_token: a
    short-lived token from the bti-ci-api service, authorized with an 'sdm'
    infra token (authanywhere comes with the CI build image; ddtool on
    laptops). $GITLAB_TOKEN overrides.
    """
    if os.environ.get("GITLAB_TOKEN"):
        return os.environ["GITLAB_TOKEN"]
    if os.environ.get("CI"):
        out = run_cmd(["authanywhere", "--audience", "sdm"]).strip()
    else:
        out = run_cmd(["ddtool", "auth", "token", "sdm", "--datacenter", "us1.ddbuild.io", "--http-header"]).strip()
    bearer = out.removeprefix("Authorization: ").strip()
    req = urllib.request.Request(
        f"https://bti-ci-api.us1.ddbuild.io/internal/ci/gitlab/token?owner=DataDog&repository=datadog-agent",
        headers={"Authorization": bearer},
    )
    with urllib.request.urlopen(req, timeout=30) as resp:
        return json.load(resp)["token"]


def gitlab_pipeline_jobs(pipeline_id: str) -> list[dict]:
    """Jobs that ran (success/failed) in a GitLab pipeline, paginated.

    The job names are the full matrix names (e.g.
    'new-e2e-fleet-config: [linux, --run "TestFleetConfig$"]'), matching what
    CI Visibility reports as @ci.job.name.
    """
    headers = {"PRIVATE-TOKEN": get_gitlab_token()}
    jobs, page = [], 1
    while True:
        url = f"{_API_V4}/projects/{_PROJECT}/pipelines/{pipeline_id}/jobs?per_page=100&page={page}&scope[]=success&scope[]=failed"
        with urllib.request.urlopen(urllib.request.Request(url, headers=headers), timeout=60) as resp:
            batch = json.load(resp)
        jobs.extend(batch)
        if len(batch) < 100:
            break
        page += 1
    return jobs
