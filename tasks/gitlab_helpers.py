"""Helper tasks related to the gitlab CI (website, API, configuration etc.)."""

from __future__ import annotations

import json
import os
import tempfile

import yaml
from invoke import task
from invoke.exceptions import Exit

from tasks.kernel_matrix_testing.ci import get_kmt_dashboard_links
from tasks.libs.ciproviders.gitlab_api import (
    compute_gitlab_ci_config_diff,
    get_all_gitlab_ci_configurations,
    get_gitlab_ci_configuration,
    get_gitlab_repo,
    print_gitlab_ci_configuration,
    resolve_gitlab_ci_configuration,
)
from tasks.libs.civisibility import (
    get_pipeline_link_to_job_id,
    get_pipeline_link_to_job_on_main,
    get_test_link_to_job_id,
    get_test_link_to_job_on_main,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import experimental


@task
def generate_ci_visibility_links(_ctx, output: str | None):
    """Generates links to CI Visibility for the current job.

    Usage:
        $ inv gitlab.generate-ci-visibility-links
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
    links = {
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

    kmt_links = get_kmt_dashboard_links()
    if kmt_links:
        links["KMT Dashboard"] = kmt_links

    return links


def print_gitlab_object(get_object, ctx, ids, repo='DataDog/datadog-agent', jq: str | None = None, jq_colors=True):
    """Prints one or more Gitlab objects in JSON and potentially query them with jq."""

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
    """Prints one or more Gitlab pipelines in JSON and potentially query them with jq.

    Usage:
        $ inv gitlab.print-pipeline 1234
        $ inv gitlab.print-pipeline 1234 -j .source
        $ inv gitlab.print-pipeline 1234 -j .duration,.ref,.status,.sha
    """

    def get_pipeline(repo, id):
        return repo.pipelines.get(id)

    print_gitlab_object(get_pipeline, ctx, ids, repo, jq, jq_colors)


@task
def print_job(ctx, ids, repo='DataDog/datadog-agent', jq: str | None = None, jq_colors=True):
    """Prints one or more Gitlab jobs in JSON and potentially query them with jq.

    Usage:
        $ inv gitlab.print-job 1234
        $ inv gitlab.print-job 1234 -j '.commit.id'
        $ inv gitlab.print-job 1234 -j '.pipeline.id'
        $ inv gitlab.print-job 1234 -j '.web_url.stage,.ref,.duration,.status'
        $ inv gitlab.print-job 1234 -j '.artifacts | length'
    """

    def get_job(repo, id):
        return repo.jobs.get(id)

    print_gitlab_object(get_job, ctx, ids, repo, jq, jq_colors)


@task
@experimental(
    'This task takes into account only explicit dependencies (job `needs` / `dependencies`), implicit dependencies (stages order) are ignored'
)
def gen_config_subset(ctx, jobs, dry_run=False, force=False):
    """Will generate a full .gitlab-ci.yml containing only the jobs necessary to run the target jobs `jobs`.

    That is, the resulting pipeline will have `jobs` as last jobs to run.

    Warning:
        This doesn't take implicit dependencies into account (stages order), only explicit dependencies (job `needs` / `dependencies`).

    Args:
        dry_run: Print only the new configuration without writing it to the .gitlab-ci.yml file.
        force: Force the update of the .gitlab-ci.yml file even if it has been modified.

    Example:
        $ inv gitlab.gen-config-subset tests_deb-arm64-py3
        $ inv gitlab.gen-config-subset tests_rpm-arm64-py3,tests_deb-arm64-py3 --dry-run
    """

    jobs_to_keep = ['cancel-prev-pipelines', 'github_rate_limit_info', 'setup_agent_version']
    attributes_to_keep = 'stages', 'variables', 'default', 'workflow'

    # .gitlab-ci.yml should not be modified
    if not force and not dry_run and ctx.run('git status -s .gitlab-ci.yml', hide='stdout').stdout.strip():
        raise Exit(color_message('The .gitlab-ci.yml file should not be modified as it will be overwritten', Color.RED))

    config = resolve_gitlab_ci_configuration(ctx, '.gitlab-ci.yml')

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
    """Prints the trace (the log) of a Gitlab job."""

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
    with_lint: bool = True,
):
    """Prints the full gitlab ci configuration.

    Args:
        job: If provided, print only one job
        clean: Apply post processing to make output more readable (remove extends, flatten lists of lists...)
        keep_special_objects: If True, do not filter out special objects (variables, stages etc.)
        expand_matrix: Will expand matrix jobs into multiple jobs
        with_lint: If False, do not lint the configuration
        git_ref: If provided, use this git reference to fetch the configuration

    Notes:
        This requires a full api token access level to the repository
    """

    yml = get_gitlab_ci_configuration(
        ctx,
        input_file,
        job=job,
        clean=clean,
        expand_matrix=expand_matrix,
        git_ref=git_ref,
        with_lint=with_lint,
        keep_special_objects=keep_special_objects,
    )

    # Print
    print_gitlab_ci_configuration(yml, sort_jobs=sort)


@task
def print_entry_points(ctx):
    """Prints gitlab ci configuration entry points."""

    print(color_message('info:', Color.BLUE), 'Fetching entry points...')
    entry_points = get_all_gitlab_ci_configurations(ctx, filter_configs=True, clean_configs=True)

    print(len(entry_points), 'entry points:')
    for entry_point, config in entry_points.items():
        print(f'- {color_message(entry_point, Color.BOLD)} ({len(config)} components)')


@task
def compute_gitlab_ci_config(
    ctx,
    before: str | None = None,
    after: str | None = None,
    before_file: str = 'before.gitlab-ci.yml',
    after_file: str = 'after.gitlab-ci.yml',
    diff_file: str = 'diff.gitlab-ci.yml',
):
    """Will compute the Gitlab CI full configuration for the current commit and the base commit and will compute the diff between them.

    The resulting files can be loaded via yaml and the diff file can be used to instantiate a MultiGitlabCIDiff object.

    Side effects:
        before_file, after_file and diff_file will be written with the corresponding configuration.
    """

    before_config, after_config, diff = compute_gitlab_ci_config_diff(ctx, before, after)

    print('Writing', before_file)
    with open(before_file, 'w') as f:
        f.write(yaml.safe_dump(before_config))

    print('Writing', after_file)
    with open(after_file, 'w') as f:
        f.write(yaml.safe_dump(after_config))

    print('Writing', diff_file)
    with open(diff_file, 'w') as f:
        f.write(yaml.safe_dump(diff.to_dict()))
