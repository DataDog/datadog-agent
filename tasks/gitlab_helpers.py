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
from tasks.libs.pipeline.stats import get_max_duration


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


required_jobs = [
    "go_mod_tidy_check",
    "tests_deb-x64-py3",
    "tests_deb-arm64-py3",
    "tests_rpm-x64-py3",
    "tests_rpm-arm64-py3",
    "slack_teams_channels_check",
    "lint_windows-x64",
    "lint_linux-x64",
    "lint_linux-arm64",
    "lint_flavor_iot_linux-x64",
    "lint_flavor_dogstatsd_linux-x64",
    "tests_ebpf_arm64",
    "tests_ebpf_x64",
    "do-not-merge",
    "agent_deb-x64-a7",
    "agent_rpm-x64-a7",
    "agent_suse-x64-a7",
    "deploy_deb_testing-a7_x64",
    "deploy_suse_rpm_testing_x64-a7",
    "new-e2e-agent-platform-install-script-debian-a7-x86_64",
    "new-e2e-agent-platform-install-script-suse-a7-x86_64",
    "new-e2e-agent-platform-install-script-ubuntu-a7-x86_64",
    "new-e2e-agent-platform-install-script-debian-iot-agent-a7-x86_64",
    "agent_heroku_deb-x64-a7",
    "iot_agent_deb-x64",
    "dogstatsd_deb-x64",
    "iot_agent_rpm-x64",
    "dogstatsd_rpm-x64",
    "security_go_generate_check",
    "lint_python",
    "new-e2e-agent-platform-install-script-centos-a7-x86_64",
    "single-machine-performance-regression_detector",
    "lint_licenses",
    "lint_components",
    "lint_filename",
    "lint_codeowners",
    "lint_copyrights",
    "lint_shell",
    "tests_macos_gitlab_amd64",
    "check_pkg_size",
]


@task
def regression_detector_stats(ctx, verbose=False):
    """Prints stats to see if the regression detector is in the critical path."""

    # Dev branches
    pipelines = (
        53904471,
        53909215,
        53905400,
        53900991,
        53899736,
        53900570,
        53901363,
        53896138,
        53895136,
        53687308,
        53892855,
    )

    print(len(pipelines), 'pipelines')
    crit_pipelines = []
    for p in pipelines:
        print(p)
        if is_regression_detector_in_critical_path(ctx, p, verbose):
            crit_pipelines.append(p)
            print('Regression detector is in the critical path')

    print(
        f'{len(crit_pipelines)} out of {len(pipelines)} pipelines ({len(crit_pipelines)/len(pipelines)*100:.2f}%) have the regression detector in the critical path: {" ".join(map(str, crit_pipelines))}'
    )


@task
def is_regression_detector_in_critical_path(ctx, pipeline_id, verbose=True) -> bool:
    from datetime import datetime

    pipeline = get_gitlab_repo().pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(all=True, per_page=100)

    # Only required jobs
    job_ends = {job.name: job.finished_at for job in jobs if any(job.name.startswith(rj) for rj in required_jobs)}
    job_ends = sorted(
        (datetime.fromisoformat(end) - datetime.fromisoformat(pipeline.created_at), job)
        for (job, end) in job_ends.items()
        if end
    )

    assert any(
        'regression_detector' in job for _, job in job_ends
    ), f'No regression detector job for pipeline {pipeline_id}'

    if verbose:
        for end, job in job_ends:
            print(f'{end.seconds // 3600}:{end.seconds // 60 % 60:02d} -> {job}')

    last_job = job_ends[-1][1]
    is_regression_detector = 'regression_detector' in last_job

    if verbose:
        if is_regression_detector:
            print('Regression detector is in the critical path')
        else:
            print('Regression detector is NOT in the critical path')

    return is_regression_detector


@task
def get_all_required_jobs_duration(ctx):
    pipelines = [
        54575993,
        54574489,
        54572789,
        54574495,
        54574273,
        54577666,
        54574886,
        54580926,
        54580479,
        54572875,
        54573820,
        54574484,
        #
        54513564,
        54568372,
        54641452,
        54642662,
        54643508,
        54645117,
        54650708,
        54651475,
        54651661,
        54651665,
        54651826,
        54651835,
        54653946,
        54653964,
        54654699,
        54660591,
        54662339,
        54665456,
        54665495,
        54671008,
        54675938,
        54678250,
        54680440,
        54686805,
        54687976,
        54688221,
        54698099,
        54698663,
        54700381,
        54701055,
        54703861,
        54704506,
        54705145,
    ]
    durations = []

    print(len(pipelines), 'pipelines')

    # repo = get_gitlab_repo()
    for i, p in enumerate(pipelines):
        print(f'#{i + 1}/{len(pipelines)}: {p}')
        # d = repo.pipelines.get(p).duration
        # if not d:
        #     continue
        # print(d)
        # durations.append(d)
        d = get_required_jobs_duration(ctx, p)
        durations.append(d)

    durations = sorted(durations)
    print(
        f'Median duration: {durations[len(durations) // 2]//3600}:{durations[len(durations) // 2] // 60 % 60} ({durations[len(durations) // 2]})'
    )


@task
def get_required_jobs_duration(ctx, pipeline_id):
    d = get_max_duration('DataDog/datadog-agent', pipeline_id)[0]

    print(f'{d // 3600}:{d // 60 % 60:02d}')

    return d


@task
def r(ctx, pipeline_id):
    from datetime import datetime

    pipeline = get_gitlab_repo().pipelines.get(pipeline_id)
    jobs = pipeline.jobs.list(all=True, per_page=100)

    # Only required jobs
    job_ends = {
        job.name: (datetime.fromisoformat(job.finished_at) - datetime.fromisoformat(pipeline.created_at))
        for job in jobs
        if job.finished_at
    }
    # job_ends = sorted(
    #     (datetime.fromisoformat(end) - datetime.fromisoformat(pipeline.created_at), job)
    #     for (job, end) in job_ends.items()
    #     if end
    # )

    # assert any(
    #     'regression_detector' in job for _, job in job_ends
    # ), f'No regression detector job for pipeline {pipeline_id}'

    # for end, job in job_ends:
    #     print(f'{end.seconds // 3600}:{end.seconds // 60 % 60:02d} -> {job}')

    print('end reg', job_ends['single-machine-performance-regression_detector'])
    print('end comment', job_ends['single-machine-performance-regression_detector-pr-comment'])
    print('end send-pipeline-stats', job_ends['send_pipeline_stats'])
