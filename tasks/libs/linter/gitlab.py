from __future__ import annotations

import functools
import re
from collections.abc import Callable
from glob import glob
from typing import Any

import yaml
from codeowners import CodeOwners
from invoke.exceptions import Exit

from tasks.libs.ciproviders.ci_config import CILintersConfig
from tasks.libs.ciproviders.gitlab_api import (
    MultiGitlabCIDiff,
    compute_gitlab_ci_config_diff,
    get_all_gitlab_ci_configurations,
    get_preset_contexts,
    is_leaf_job,
    load_context,
    retrieve_all_paths,
    test_gitlab_configuration,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import gitlab_section, running_in_ci
from tasks.libs.linter.gitlab_exceptions import (
    FailureLevel,
    GitlabLintFailure,
    MultiGitlabLintFailure,
    SingleGitlabLintFailure,
)
from tasks.libs.linter.shell import DEFAULT_SHELLCHECK_EXCLUDES, flatten_script, shellcheck_linter

ALL_GITLABCI_SUBLINTERS: list[Callable] = []
PREPUSH_GITLABCI_SUBLINTERS: list[Callable] = []


# === Task code bodies === #
def gitlabci_sublinter(
    name: str, info_message: str, success_message: str, run_on_prepush: bool = True, allow_fail: bool = False
):
    """Decorator for registering a gitlabci sublinter, and setting up some common config options."""

    # Weird pattern here, but it is necessary for implementing "parametrized" decorators
    def decorator(func):
        @functools.wraps(func)
        def wrapper(*args, **kwargs):
            print(f'[{color_message("INFO", Color.BLUE)}] {info_message}')
            try:
                func(*args, **kwargs)
            except GitlabLintFailure as e:
                e.set_linter_name(name)
                if allow_fail:
                    e.ignore()
                raise e

            # We never get here if the function raises an exception
            print(f"[{color_message('OK', Color.GREEN)}] {success_message}")

        ALL_GITLABCI_SUBLINTERS.append(wrapper)
        if run_on_prepush:
            PREPUSH_GITLABCI_SUBLINTERS.append(wrapper)
        return wrapper

    return decorator


def gitlabci_lint_task_template(
    task_body: Callable, ctx, configs_or_diff_file: str | None = None, use_diff: bool = False, verbosity: int = 0
):
    """Generic task template for gitlabci linting tasks.

    This function handles config loading/generation & failure printing.
    The actual task-specific logic should be passed in `task_body`.
    """
    full_config: dict[str, dict]
    if use_diff:
        _, _, diff = load_or_generate_gitlab_ci_diff(ctx, configs_or_diff_file)
        jobs = extract_gitlab_ci_jobs(diff=diff)
        full_config = diff.after  # type: ignore
    else:
        configs = load_or_generate_gitlab_ci_configs(ctx, configs_or_diff_file)
        jobs = extract_gitlab_ci_jobs(configs=configs)
        full_config = configs  # type: ignore

    # No change, info already printed in extract_gitlab_ci_jobs
    if not jobs:
        return

    min_level = [FailureLevel.ERROR, FailureLevel.WARNING, FailureLevel.IGNORED][verbosity]

    try:
        task_body(jobs=jobs, full_config=full_config)
    except GitlabLintFailure as e:
        print(e.pretty_print(min_level=min_level))
        raise Exit(code=e.exit_code) from e


@gitlabci_sublinter(
    name="config-check",
    info_message='Running main gitlabci config linter...',
    success_message='All contexts tested successfully.',
)
def lint_and_test_gitlab_ci_config(full_config: dict[str, dict], test="all", custom_context=None, *args, **kwargs):
    """Lints and tests the validity of the gitlabci config object passed in argument.

    Args:
        test: The context preset to test the gitlab ci file with containing environment variables.
        custom_context: A custom context to test the gitlab ci file with.
    """
    for config_filename, config_object in full_config.items():
        with gitlab_section(f"Testing {config_filename}", echo=True, collapsed=True):
            # Only the main config should be tested with all contexts
            if config_filename == ".gitlab-ci.yml":
                all_contexts = []
                if custom_context:
                    all_contexts = load_context(custom_context)
                else:
                    all_contexts = get_preset_contexts(test)

                print(f'[{color_message("INFO", Color.BLUE)}] We will test {len(all_contexts)} contexts')
                for context in all_contexts:
                    print("Test gitlab configuration with context: ", context)
                    test_gitlab_configuration(
                        entry_point=config_filename, config_object=config_object, context=dict(context)
                    )
            else:
                test_gitlab_configuration(entry_point=config_filename, config_object=config_object)


@gitlabci_sublinter(
    name="shellcheck",
    info_message='Running shellcheck on gitlabci scripts...',
    success_message='All scripts checked successfully.',
    run_on_prepush=False,
    allow_fail=True,
)
def shellcheck_gitlab_ci_jobs(
    ctx,
    jobs: list[tuple[str, dict]],
    exclude: str = DEFAULT_SHELLCHECK_EXCLUDES,
    verbose: bool = False,
    shellcheck_args="",
    fail_fast: bool = False,
    only_errors: bool = False,
    *args,
    **kwargs,
):
    """Lints the scripts for the given job objects using shellcheck.

    Args:
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        use_bat: If True (or None), will (try to) use bat to display the script.
        only_errors: Show only errors, not warnings.

    Note:
        Will raise an Exit if any errors are found.
    """
    # TODO(@agent-devx): Remove this once we have shellcheck in CI
    if running_in_ci():
        # Shellcheck is not installed in the CI environment, so we skip it
        print(f'[{color_message("INFO", Color.BLUE)}] Skipping shellcheck in CI environment')
        return

    scripts = {}
    for job, content in jobs:
        # Skip jobs that are not executed
        if not is_leaf_job(job, content):
            continue

        # Shellcheck is only for bash like scripts
        is_powershell = any(
            'powershell' in flatten_script(content.get(keyword, ''))
            for keyword in ('before_script', 'script', 'after_script')
        )
        if is_powershell:
            continue

        if verbose:
            print('Verifying job:', job)

        # Lint scripts
        for keyword in ('before_script', 'script', 'after_script'):
            if keyword in content:
                scripts[f'{job}.{keyword}'] = f'#!/bin/bash\n{flatten_script(content[keyword]).strip()}\n'

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, only_errors)


@gitlabci_sublinter(
    name="change-paths-valid",
    info_message='Checking change: paths: rules defined in gitlabci jobs are valid...',
    success_message='All change: paths: rules defined in gitlabci jobs are valid.',
)
def check_change_paths_valid_gitlab_ci_jobs(jobs: list[tuple[str, dict]], *args, **kwargs):
    """Verifies that rules: changes: paths in the given jobs match existing files in the repo"""
    failures = []
    for job_name, job in jobs:
        for path in set(retrieve_all_paths(job)):
            files = glob(path, recursive=True)
            if len(files) == 0:
                failures.append(
                    SingleGitlabLintFailure(
                        _details=f"Path '{path}' does not match any files in the repository",
                        failing_job_name=job_name,
                        _level=FailureLevel.ERROR,
                    )
                )
    if failures:
        if len(failures) == 1:
            raise failures[0]
        raise MultiGitlabLintFailure(failures=failures)


@gitlabci_sublinter(
    name="change-paths-defined",
    info_message='Checking gitlabci jobs have defined change: paths: rules...',
    success_message='All gitlabci jobs have defined change: paths: rules.',
    allow_fail=True,
)
def check_change_paths_exist_gitlab_ci_jobs(jobs: list[tuple[str, dict[str, Any]]], *args, **kwargs):
    """Verifies that the jobs passed in contain a change path rule in the given config."""
    tests_without_change_path_allow_list = {
        'generate-fips-e2e-pipeline',
        'generate-flakes-finder-pipeline',
        'k8s-e2e-cspm-dev',
        'k8s-e2e-cspm-main',
        'k8s-e2e-otlp-dev',
        'k8s-e2e-otlp-main',
        'new-e2e-agent-platform-install-script-amazonlinux-a6-arm64',
        'new-e2e-agent-platform-install-script-amazonlinux-a6-x86_64',
        'new-e2e-agent-platform-install-script-amazonlinux-a7-arm64',
        'new-e2e-agent-platform-install-script-amazonlinux-a7-x64',
        'new-e2e-agent-platform-install-script-centos-a6-x86_64',
        'new-e2e-agent-platform-install-script-centos-a7-x86_64',
        'new-e2e-agent-platform-install-script-centos-dogstatsd-a7-x86_64',
        'new-e2e-agent-platform-install-script-centos-fips-a6-x86_64',
        'new-e2e-agent-platform-install-script-centos-fips-a7-x86_64',
        'new-e2e-agent-platform-install-script-centos-fips-dogstatsd-a7-x86_64',
        'new-e2e-agent-platform-install-script-centos-fips-iot-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-centos-iot-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-debian-a6-arm64',
        'new-e2e-agent-platform-install-script-debian-a6-x86_64',
        'new-e2e-agent-platform-install-script-debian-a7-arm64',
        'new-e2e-agent-platform-install-script-debian-a7-x86_64',
        'new-e2e-agent-platform-install-script-debian-dogstatsd-a7-x86_64',
        'new-e2e-agent-platform-install-script-debian-heroku-agent-a6-x86_64',
        'new-e2e-agent-platform-install-script-debian-heroku-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-debian-iot-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-suse-a6-x86_64',
        'new-e2e-agent-platform-install-script-suse-a7-arm64',
        'new-e2e-agent-platform-install-script-suse-a7-x86_64',
        'new-e2e-agent-platform-install-script-suse-dogstatsd-a7-x86_64',
        'new-e2e-agent-platform-install-script-suse-iot-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-a6-arm64',
        'new-e2e-agent-platform-install-script-ubuntu-a6-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-a7-arm64',
        'new-e2e-agent-platform-install-script-ubuntu-a7-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-dogstatsd-a7-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-heroku-agent-a6-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-heroku-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-ubuntu-iot-agent-a7-x86_64',
        'new-e2e-agent-platform-install-script-docker',
        'new-e2e-agent-platform-install-script-upgrade6-amazonlinux-x64',
        'new-e2e-agent-platform-install-script-upgrade6-centos-fips-x86_64',
        'new-e2e-agent-platform-install-script-upgrade6-centos-x86_64',
        'new-e2e-agent-platform-install-script-upgrade6-debian-x86_64',
        'new-e2e-agent-platform-install-script-upgrade6-suse-x86_64',
        'new-e2e-agent-platform-install-script-upgrade6-ubuntu-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-amazonlinux-iot-agent-x64',
        'new-e2e-agent-platform-install-script-upgrade7-amazonlinux-x64',
        'new-e2e-agent-platform-install-script-upgrade7-centos-fips-iot-agent-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-centos-fips-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-centos-iot-agent-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-centos-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-debian-iot-agent-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-debian-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-suse-iot-agent-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-suse-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-ubuntu-iot-agent-x86_64',
        'new-e2e-agent-platform-install-script-upgrade7-ubuntu-x86_64',
        'new-e2e-agent-platform-rpm-centos6-a7-x86_64',
        'new-e2e-agent-platform-step-by-step-amazonlinux-a6-arm64',
        'new-e2e-agent-platform-step-by-step-amazonlinux-a6-x86_64',
        'new-e2e-agent-platform-step-by-step-amazonlinux-a7-arm64',
        'new-e2e-agent-platform-step-by-step-amazonlinux-a7-x64',
        'new-e2e-agent-platform-step-by-step-centos-a6-x86_64',
        'new-e2e-agent-platform-step-by-step-centos-a7-x86_64',
        'new-e2e-agent-platform-step-by-step-debian-a6-arm64',
        'new-e2e-agent-platform-step-by-step-debian-a6-x86_64',
        'new-e2e-agent-platform-step-by-step-debian-a7-arm64',
        'new-e2e-agent-platform-step-by-step-debian-a7-x64',
        'new-e2e-agent-platform-step-by-step-suse-a6-x86_64',
        'new-e2e-agent-platform-step-by-step-suse-a7-arm64',
        'new-e2e-agent-platform-step-by-step-suse-a7-x86_64',
        'new-e2e-agent-platform-step-by-step-ubuntu-a6-arm64',
        'new-e2e-agent-platform-step-by-step-ubuntu-a6-x86_64',
        'new-e2e-agent-platform-step-by-step-ubuntu-a7-arm64',
        'new-e2e-agent-platform-step-by-step-ubuntu-a7-x86_64',
        'new-e2e-agent-runtimes',
        'new-e2e-agent-configuration',
        'new-e2e-cws',
        'new-e2e-language-detection',
        'new-e2e-npm-docker',
        'new-e2e-eks-cleanup',
        'new-e2e-npm-packages',
        'new-e2e-orchestrator',
        'new-e2e-package-signing-amazonlinux-a6-x86_64',
        'new-e2e-package-signing-debian-a7-x86_64',
        'new-e2e-package-signing-suse-a7-x86_64',
        'new-e2e_windows_powershell_module_test',
        'new-e2e-eks-cleanup-on-failure',
        'trigger-flakes-finder',
        'trigger-fips-e2e',
    }

    def contains_valid_change_rule(job_rules):
        """Verifies that the job rule contains the required change path configuration."""

        if 'changes' not in job_rules or 'paths' not in job_rules['changes']:
            return False

        # The change paths should be more than just test files
        return any(
            not path.startswith(('test/', './test/', 'test\\', '.\\test\\')) for path in job_rules['changes']['paths']
        )

        # Verify that all tests contain a change path rule

    failures = []
    for job_name, job in jobs:
        if "rules" in job and not any(
            contains_valid_change_rule(rule) for rule in job['rules'] if isinstance(rule, dict)
        ):
            failures.append(
                SingleGitlabLintFailure(
                    _details=f"Job does not contain a valid change paths rule{', but is allow-listed' if job_name in tests_without_change_path_allow_list else ''}.",
                    failing_job_name=job_name,
                    _level=FailureLevel.IGNORED
                    if job_name in tests_without_change_path_allow_list
                    else FailureLevel.ERROR,
                )
            )

    if failures:
        if len(failures) == 1:
            raise failures[0]

        raise MultiGitlabLintFailure(failures=failures)


@gitlabci_sublinter(
    name="needs-rules-defined",
    info_message='Checking gitlabci jobs have defined needs: and rules: sections...',
    success_message='All gitlabci jobs have defined needs: and rules: sections.',
)
def check_needs_rules_gitlab_ci_jobs(jobs: list[tuple[str, dict]], ci_linters_config: CILintersConfig, *args, **kwargs):
    """Verifies that the specified jobs contain `needs` and also `rules`."""
    # Verify the jobs
    failures = []
    for job_name, job in jobs:
        error = "needs" not in job or "rules" not in job
        to_ignore = (
            job_name in ci_linters_config.needs_rules_jobs or job['stage'] in ci_linters_config.needs_rules_stages
        )

        if error:
            failures.append(
                SingleGitlabLintFailure(
                    _details=f"Job is missing `needs` or `rules` key{', but is allow-listed' if to_ignore else ''}.",
                    failing_job_name=job_name,
                    _level=FailureLevel.IGNORED if to_ignore else FailureLevel.ERROR,
                )
            )

    if failures:
        if len(failures) == 1:
            raise failures[0]

        raise MultiGitlabLintFailure(failures=failures)


@gitlabci_sublinter(
    name="jobowners-exist",
    info_message='Checking gitlabci jobs have defined JOBOWNERS...',
    success_message='All gitlabci jobs have defined JOBOWNERS.',
)
def check_owners_gitlab_ci_jobs(
    jobs: list[tuple[str, dict]], ci_linters_config: CILintersConfig, jobowners: CodeOwners, *args, **kwargs
):
    job_names = [name for (name, _) in jobs]
    failures = []
    for job in job_names:
        owners = [name for (kind, name) in jobowners.of(job) if kind == 'TEAM']
        if not owners:
            failures.append(
                SingleGitlabLintFailure(
                    _details=f"Job does not have any non-default owners defined{', but is allow-listed' if job in ci_linters_config.job_owners_jobs else ''}.",
                    failing_job_name=job,
                    _level=FailureLevel.IGNORED if job in ci_linters_config.job_owners_jobs else FailureLevel.ERROR,
                )
            )

    if failures:
        if len(failures) == 1:
            raise failures[0]

        raise MultiGitlabLintFailure(failures=failures)


# === Task-specific helpers === #
class SSMParameterCall:
    def __init__(self, file, line_nb, with_wrapper=False, with_env_var=False):
        """Initialize an SSMParameterCall instance.

        Args:
            file (str): The name of the file where the SSM parameter call is located.
            line_nb (int): The line number in the file where the SSM parameter call is located.
            with_wrapper (bool, optional): If the call is using the wrapper. Defaults to False.
            with_env_var (bool, optional): If the call is using an environment variable defined in .gitlab-ci.yml. Defaults to False.
        """
        self.file = file
        self.line_nb = line_nb
        self.with_wrapper = with_wrapper
        self.with_env_var = with_env_var

    def __str__(self):
        message = ""
        if not self.with_wrapper:
            message += "Please use the dedicated `fetch_secret.(sh|ps1)`."
        if not self.with_env_var:
            message += " Save your parameter name as environment variable in .gitlab-ci.yml file."
        return f"{self.file}:{self.line_nb + 1}. {message}"

    def __repr__(self):
        return str(self)


def list_get_parameter_calls(file):
    aws_ssm_call = re.compile(r"^.+ssm get-parameter.+--name +(?P<param>[^ ]+).*$")
    # remove the first letter of the script name because '\f' is badly interpreted for windows paths
    wrapper_call = re.compile(r"^.+etch_secret.(sh|ps1)[\"]? (-parameterName )?+(?P<param>[^ )]+).*$")
    calls = []
    with open(file) as f:
        try:
            for nb, line in enumerate(f):
                m = aws_ssm_call.match(line.strip())
                if m:
                    # Remove possible quotes
                    param = m["param"].replace('"', '').replace("'", "")
                    calls.append(
                        SSMParameterCall(file, nb, with_env_var=(param.startswith("$") or "os.environ" in param))
                    )
                m = wrapper_call.match(line.strip())
                param = m["param"].replace('"', '').replace("'", "") if m else None
                if m and not (param.startswith("$") or "os.environ" in param):
                    calls.append(SSMParameterCall(file, nb, with_wrapper=True))
        except UnicodeDecodeError:
            pass
    return calls


def _gitlab_ci_jobs_codeowners_lint(modified_yml_files, gitlab_owners):
    failures = []
    for path in modified_yml_files:
        teams = [team for kind, team in gitlab_owners.of(path) if kind == 'TEAM']
        if not teams:
            failures.append(
                SingleGitlabLintFailure(
                    _details=f"File '{path}' does not have any matching non-default CODEOWNERS rule",
                    failing_job_name=path,
                    _level=FailureLevel.ERROR,
                )
            )

    if failures:
        if len(failures) == 1:
            raise failures[0]

        raise MultiGitlabLintFailure(failures=failures)


# === "Plumbing" methods === #
# Note: Using * prevents passing positional, avoiding confusion between configs and diff
def extract_gitlab_ci_jobs(
    *, configs: dict[str, dict] | None = None, diff: MultiGitlabCIDiff | None = None
) -> list[tuple[str, dict[str, Any]]]:
    """Retrieves the jobs from full gitlab ci configuration file or from a diff file.

    Args:
        diff: Diff object used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config.
        configs: "Full" gitlab ci configuration object, obtained by `get_all_gitlab_ci_configurations`.

    Returns:
        A list of (job_name: str, job_contents: dict) tuples.
        If `only_names` is True, it will be a simple list of all the job names.
    """
    # Dict of entrypoint -> config object, of the format returned by `get_all_gitlab_ci_configurations`

    # Unfortunately a MultiGitlabCIDiff is not always truthy (see its __bool__), so we have to check explicitely
    assert (configs is not None or diff is not None) and not (
        configs is not None and diff is not None
    ), "Please pass exactly one of a config object or a diff object"

    if diff is not None:
        jobs = [(job, contents) for _, job, contents, _ in diff.iter_jobs(added=True, modified=True, only_leaves=True)]
    else:
        jobs = [
            (job, job_contents)
            for contents in configs.values()  # type: ignore
            for job, job_contents in contents.items()
            if is_leaf_job(job, job_contents)
        ]

    if not jobs:
        print(f'[{color_message("INFO", Color.BLUE)}] No added / modified job files, skipping lint')
        return []

    return jobs


def load_or_generate_gitlab_ci_configs(ctx, yaml_to_load: str | None = None, **kwargs) -> dict[str, dict]:
    """Load a "full" gitlabci config object from file, or re-generate it if needed.

    "Full" in this context means that:
    - The "full" configuration object is a dict of entrypoint -> gitlabci configuration object
    - In each sub-object, all `include`s, `reference`s and `extend`s have been resolved.

    Args:
        `yaml_to_load`: Path to a yaml file containing a full gitlabci config object
    Any other kwargs will be passed to `get_all_gitlab_ci_configurations`, called if need to regenerate the config.
    """
    if yaml_to_load:
        with open(yaml_to_load) as f:
            return yaml.safe_load(f)

    return get_all_gitlab_ci_configurations(ctx, **kwargs)


def load_or_generate_gitlab_ci_diff(
    ctx, yaml_to_load: str | None = None, **kwargs
) -> tuple[dict[str, dict], dict[str, dict], MultiGitlabCIDiff]:
    """Similar to `load_or_generate_gitlab_ci_configs`, but returns a 'diff triplet'.

    We call a "diff triplet" a triplet of `(diff, before_config, after_config)`, as generated by the `compute_gitlab_ci_config` task.
    Any extra kwargs are passed as-is to `compute_gitlab_ci_config_diff`.
    """
    if yaml_to_load:
        with open(yaml_to_load) as f:
            diff = MultiGitlabCIDiff.from_dict(yaml.safe_load(f))
            return diff.before, diff.after, diff  # type: ignore

    before, after, diff = compute_gitlab_ci_config_diff(ctx, **kwargs)
    return before, after, diff
