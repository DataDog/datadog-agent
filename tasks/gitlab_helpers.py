"""
Helper for generating links to CI Visibility for Gitlab CI jobs
"""

from __future__ import annotations

import json
import os
import sys
import tempfile

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.libs.ciproviders.gitlab_api import (
    get_all_gitlab_ci_configurations,
    get_full_gitlab_ci_configuration,
    get_gitlab_ci_configuration,
    get_gitlab_repo,
    print_gitlab_ci_configuration,
)
from tasks.libs.civisibility import (
    get_pipeline_link_to_job_id,
    get_pipeline_link_to_job_on_main,
    get_test_link_to_job_id,
    get_test_link_to_job_on_main,
)
from tasks.libs.common.color import Color, color_message


@task
def generate_ci_visibility_links(_ctx, output: str | None):
    """
    Generate links to CI Visibility for the current job
    usage deva gitlab.generate-ci-visibility-links
    Generated file
    """
    ci_job_id = os.environ.get("CI_JOB_ID")
    if not ci_job_id:
        print(
            color_message(
                "CI_JOB_ID is not set, this task can run only on Gitlab, skipping...",
                Color.RED,
            )
        )
        return
    ci_job_name = os.environ.get("CI_JOB_NAME")
    if not ci_job_name:
        print(
            color_message(
                "CI_JOB_NAME is not set, this task can run only on Gitlab, skipping...",
                Color.RED,
            )
        )
        return

    gitlab_annotations_report = create_gitlab_annotations_report(ci_job_id, ci_job_name)

    content = json.dumps(gitlab_annotations_report, indent=2)

    annotation_file_name = output or f"external_links_{ci_job_id}.json"
    with open(annotation_file_name, "w") as f:
        f.write(content)
    print(f"Generated {annotation_file_name}")


def create_gitlab_annotations_report(ci_job_id: str, ci_job_name: str):
    return {
        "CI Visibility": [
            {
                "external_link": {
                    "label": "CI Visibility: This job instance",
                    "url": get_pipeline_link_to_job_id(ci_job_id),
                }
            },
            {
                "external_link": {
                    "label": "Test Visibility: This job test runs",
                    "url": get_test_link_to_job_id(ci_job_id),
                }
            },
            {
                "external_link": {
                    "label": "CI Visibility: This job on main",
                    "url": get_pipeline_link_to_job_on_main(ci_job_name),
                }
            },
            {
                "external_link": {
                    "label": "Test Visibility: This job test runs on main",
                    "url": get_test_link_to_job_on_main(ci_job_name),
                }
            },
        ]
    }


def print_gitlab_object(get_object, ctx, ids, repo='DataDog/datadog-agent', jq: str | None = None, jq_colors=True):
    """
    Print one or more Gitlab objects in JSON and potentially query them with jq
    """
    repo = get_gitlab_repo(repo)
    ids = [i for i in ids.split(",") if i]
    for id in ids:
        obj = get_object(repo, id)

        if jq:
            jq_flags = "-C" if jq_colors else ""
            with tempfile.NamedTemporaryFile('w', delete=True) as f:
                f.write(obj.to_json())
                f.flush()

                ctx.run(f"cat '{f.name}' | jq {jq_flags} '{jq}'")
        else:
            obj.pprint()


@task
def print_pipeline(ctx, ids, repo='DataDog/datadog-agent', jq: str | None = None, jq_colors=True):
    """
    Print one or more Gitlab pipelines in JSON and potentially query them with jq
    """

    def get_pipeline(repo, id):
        return repo.pipelines.get(id)

    print_gitlab_object(get_pipeline, ctx, ids, repo, jq, jq_colors)


@task
def print_job(ctx, ids, repo='DataDog/datadog-agent', jq: str | None = None, jq_colors=True):
    """
    Print one or more Gitlab jobs in JSON and potentially query them with jq
    """

    def get_job(repo, id):
        return repo.jobs.get(id)

    print_gitlab_object(get_job, ctx, ids, repo, jq, jq_colors)


@task
def gen_config_subset(ctx, jobs, dry_run=False, force=False):
    """
    Will generate a full .gitlab-ci.yml containing only the jobs necessary to run the target jobs
    """

    print(
        color_message(
            'Warning: This task takes into account only explicit dependencies (job `needs` / `dependencies`), implicit dependencies (stages order) are ignored',
            Color.ORANGE,
        ),
        file=sys.stderr,
    )

    jobs_to_keep = ['cancel-prev-pipelines', 'github_rate_limit_info', 'setup_agent_version']
    attributes_to_keep = 'stages', 'variables', 'default', 'workflow'

    # .gitlab-ci.yml should not be modified
    if not force and not dry_run and ctx.run('git status -s .gitlab-ci.yml', hide='stdout').stdout.strip():
        raise Exit(color_message('The .gitlab-ci.yml file should not be modified as it will be overwritten', Color.RED))

    config = get_full_gitlab_ci_configuration(ctx, '.gitlab-ci.yml')

    jobs = [j for j in jobs.split(',') if j] + jobs_to_keep
    required = set()

    def add_dependencies(job):
        nonlocal required, config

        if job in required:
            return
        required.add(job)

        dependencies = []
        if 'needs' in config[job]:
            dependencies = config[job]['needs']
        if 'dependencies' in config[job]:
            dependencies = config[job]['dependencies']

        for dep in dependencies:
            if isinstance(dep, dict):
                dep = dep['job']
            add_dependencies(dep)

    # Make a DFS to find all the jobs that are needed to run the target jobs
    for job in jobs:
        add_dependencies(job)

    new_config = {job: config[job] for job in required}

    # Remove extends
    for job in new_config.values():
        job.pop('extends', None)

    # Keep gitlab config
    for attr in attributes_to_keep:
        new_config[attr] = config[attr]

    content = yaml.safe_dump(new_config)

    if dry_run:
        print(content)
    else:
        with open('.gitlab-ci.yml', 'w') as f:
            f.write(content)

        print(color_message('The .gitlab-ci.yml file has been updated', Color.GREEN))


@task
def print_job_trace(_, job_id, repo='DataDog/datadog-agent'):
    """
    Print the trace (the log) of a Gitlab job
    """
    repo = get_gitlab_repo(repo)
    trace = str(repo.jobs.get(job_id, lazy=True).trace(), 'utf-8')

    print(trace)


@task
def print_ci(
    ctx,
    input_file: str = '.gitlab-ci.yml',
    job: str | None = None,
    sort: bool = False,
    clean: bool = True,
    keep_special_objects: bool = False,
    expand_matrix: bool = False,
    git_ref: str | None = None,
    ignore_errors: bool = False,
):
    """
    Prints the full gitlab ci configuration.

    - job: If provided, print only one job
    - clean: Apply post processing to make output more readable (remove extends, flatten lists of lists...)
    - keep_special_objects: If True, do not filter out special objects (variables, stages etc.)
    - expand_matrix: Will expand matrix jobs into multiple jobs
    - ignore_errors: If True, ignore errors in the gitlab configuration (only process yaml)
    - git_ref: If provided, use this git reference to fetch the configuration
    - NOTE: This requires a full api token access level to the repository
    """
    yml = get_gitlab_ci_configuration(
        ctx,
        input_file,
        job=job,
        clean=clean,
        expand_matrix=expand_matrix,
        git_ref=git_ref,
        ignore_errors=ignore_errors,
        keep_special_objects=keep_special_objects,
    )

    # Print
    print_gitlab_ci_configuration(yml, sort_jobs=sort)


@task
def print_entry_points(ctx):
    """
    Print gitlab ci configuration entry points.
    """
    print(color_message('info:', Color.BLUE), 'Fetching entry points...')
    entry_points = get_all_gitlab_ci_configurations(ctx, filter_configs=True, clean_configs=True)

    print(len(entry_points), 'entry points:')
    for entry_point, config in entry_points.items():
        print(f'- {color_message(entry_point, Color.BOLD)} ({len(config)} components)')
