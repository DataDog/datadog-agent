from __future__ import annotations

import re
import sys
from collections import defaultdict
from glob import glob
from typing import Any

import yaml
from invoke.exceptions import Exit

from tasks.libs.ciproviders.ci_config import CILintersConfig
from tasks.libs.ciproviders.gitlab_api import (
    MultiGitlabCIDiff,
    compute_gitlab_ci_config_diff,
    get_all_gitlab_ci_configurations,
    get_preset_contexts,
    is_leaf_job,
    load_context,
    read_includes,
    retrieve_all_paths,
    test_gitlab_configuration,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.utils import gitlab_section
from tasks.libs.linter.shell import DEFAULT_SHELLCHECK_EXCLUDES, flatten_script, shellcheck_linter
from tasks.libs.owners.parsing import read_owners


# === Task code bodies === #
def lint_and_test_gitlab_ci_config(
    configs: dict[str, dict],
    test="all",
    custom_context=None,
):
    """Lints and tests the validity of the gitlabci config object passed in argument.

    Args:
        test: The context preset to test the gitlab ci file with containing environment variables.
        custom_context: A custom context to test the gitlab ci file with.
    """
    for config_filename, config_object in configs.items():
        with gitlab_section(f"Testing {config_filename}", echo=True):
            # Only the main config should be tested with all contexts
            if config_filename == ".gitlab-ci.yml":
                all_contexts = []
                if custom_context:
                    all_contexts = load_context(custom_context)
                else:
                    all_contexts = get_preset_contexts(test)

                print(f'{color_message("info", Color.BLUE)}: We will test {len(all_contexts)} contexts')
                for context in all_contexts:
                    print("Test gitlab configuration with context: ", context)
                    test_gitlab_configuration(
                        entry_point=config_filename, config_object=config_object, context=dict(context)
                    )
            else:
                test_gitlab_configuration(entry_point=config_filename, config_object=config_object)


def shellcheck_gitlab_ci_jobs(
    ctx,
    jobs: list[tuple[str, dict]],
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    verbose: bool = False,
    shellcheck_args="",
    fail_fast: bool = False,
    use_bat: str | None = None,
    only_errors: bool = False,
):
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

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, use_bat, only_errors)


def check_change_paths_gitlab_ci_jobs(jobs: list[tuple[str, dict]]):
    """Verifies that rules: changes: paths in the given jobs match existing files in the repo"""
    error_paths = []
    for _, job in jobs:
        for path in set(retrieve_all_paths(job)):
            files = glob(path, recursive=True)
            if len(files) == 0:
                error_paths.append(path)
    if error_paths:
        raise Exit(
            f"{color_message('No files found for paths', Color.RED)}:\n{chr(10).join(' - ' + path for path in error_paths)}"
        )


def check_change_paths_gitlab_ci_config(ctx, configs: dict[str, dict], job_files: list[str] | None = None):
    """Verifies that the jobs defined within job_files contain a change path rule in the given config."""
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

    # Fetch all test jobs
    job_files = job_files or (['.gitlab/e2e/e2e.yml'] + list(glob('.gitlab/e2e/install_packages/*.yml')))
    test_config = read_includes(ctx, job_files, return_config=True, add_file_path=True)
    tests = [(test, data['_file_path']) for test, data in test_config.items() if test[0] != '.']

    def contains_valid_change_rule(rule):
        """Verifies that the job rule contains the required change path configuration."""

        if 'changes' not in rule or 'paths' not in rule['changes']:
            return False

        # The change paths should be more than just test files
        return any(
            not path.startswith(('test/', './test/', 'test\\', '.\\test\\')) for path in rule['changes']['paths']
        )

        # Verify that all tests contain a change path rule

    tests_without_change_path = defaultdict(list)
    tests_without_change_path_allowed = defaultdict(list)
    for test, filepath in tests:
        for entry_point, config in configs.items():
            if test not in config:
                continue
            if "rules" in config[test] and not any(
                contains_valid_change_rule(rule) for rule in config[test]['rules'] if isinstance(rule, dict)
            ):
                if test in tests_without_change_path_allow_list:
                    tests_without_change_path_allowed[f"{filepath} ({entry_point})"].append(test)
                else:
                    tests_without_change_path[f"{filepath} ({entry_point})"].append(test)

    if len(tests_without_change_path_allowed) != 0:
        with gitlab_section('Allow-listed jobs', collapsed=True):
            print(
                color_message(
                    'warning: The following tests do not contain required change paths rule but are allowed:',
                    Color.ORANGE,
                )
            )
            for filepath_and_entrypoint, tests in tests_without_change_path_allowed.items():
                print(f"- {color_message(filepath_and_entrypoint, Color.BLUE)}: {', '.join(tests)}")
            print(color_message('warning: End of allow-listed jobs', Color.ORANGE))
            print()

    if len(tests_without_change_path) != 0:
        print(color_message("error: Tests without required change paths rule:", "red"), file=sys.stderr)
        for filepath_and_entrypoint, tests in tests_without_change_path.items():
            print(f"- {color_message(filepath_and_entrypoint, Color.BLUE)}: {', '.join(tests)}", file=sys.stderr)

        raise RuntimeError(
            color_message(
                'Some tests do not contain required change paths rule, they must contain at least one non-test path.',
                Color.RED,
            )
        )

    print(color_message("success: All tests contain a change paths rule or are allow-listed", "green"))


def check_needs_rules_gitlab_ci_jobs(jobs: list[tuple[str, dict]], ci_linters_config: CILintersConfig):
    """Verifies that the specified jobs contain `needs` and also `rules`."""
    # Verify the jobs
    error_jobs = []
    n_ignored = 0
    for job, contents in jobs:
        error = "needs" not in contents or "rules" not in contents
        to_ignore = (
            job in ci_linters_config.needs_rules_jobs or contents['stage'] in ci_linters_config.needs_rules_stages
        )

        if to_ignore:
            if error:
                n_ignored += 1
            continue

        if error:
            error_jobs.append((job, contents['stage']))

    if n_ignored:
        print(
            f'{color_message("Info", Color.BLUE)}: {n_ignored} ignored jobs (jobs / stages defined in {ci_linters_config.path}:needs-rules)'
        )

    if error_jobs:
        error_jobs = sorted(error_jobs)
        error_jobs = '\n'.join(f'- {job} ({stage} stage)' for job, stage in error_jobs)

        raise Exit(
            f"{color_message('Error', Color.RED)}: The following jobs are missing 'needs' or 'rules' section:\n{error_jobs}\nJobs should have needs and rules, see https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/4059234597/Gitlab+CI+configuration+guidelines#datadog-agent for details.\nIf you really want to have a job without needs / rules, you can add it to {ci_linters_config.path}",
            code=1,
        )
    else:
        print(f'{color_message("Success", Color.GREEN)}: All jobs have "needs" and "rules"')


def check_owners_gitlab_ci_jobs(
    jobs: list[tuple[str, dict]],
    ci_linters_config: CILintersConfig,
    path_jobowners: str = '.gitlab/JOBOWNERS',
):
    jobowners = read_owners(path_jobowners, remove_default_pattern=True)
    job_names = [name for (name, _) in jobs]
    error_jobs = []
    n_ignored = 0
    for job in job_names:
        owners = [name for (kind, name) in jobowners.of(job) if kind == 'TEAM']
        if not owners:
            if job in ci_linters_config.job_owners_jobs:
                n_ignored += 1
            else:
                error_jobs.append(job)

    if n_ignored:
        print(
            f'{color_message("Info", Color.BLUE)}: {n_ignored} ignored jobs (jobs defined in {ci_linters_config.path}:job-owners)'
        )

    if error_jobs:
        error_jobs = '\n'.join(f'- {job}' for job in sorted(error_jobs))
        raise Exit(
            f"{color_message('Error', Color.RED)}: These jobs are not defined in {path_jobowners}:\n{error_jobs}"
        )

    print(f'{color_message("Success", Color.GREEN)}: All jobs have owners defined in {path_jobowners}')


# === Task-specific helpers === #
class SSMParameterCall:
    def __init__(self, file, line_nb, with_wrapper=False, with_env_var=False):
        """
        Initialize an SSMParameterCall instance.

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


def _gitlab_ci_jobs_codeowners_lint(path_codeowners, modified_yml_files, gitlab_owners):
    error_files = []
    for path in modified_yml_files:
        teams = [team for kind, team in gitlab_owners.of(path) if kind == 'TEAM']
        if not teams:
            error_files.append(path)

    if error_files:
        error_files = '\n'.join(f'- {path}' for path in sorted(error_files))

        raise Exit(
            f"{color_message('Error', Color.RED)}: These files should have specific CODEOWNERS rules within {path_codeowners} starting with '/.gitlab/<stage_name>'):\n{error_files}"
        )
    else:
        print(f'{color_message("Success", Color.GREEN)}: All files have CODEOWNERS rules within {path_codeowners}')


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
        print(f"{color_message('Info', Color.BLUE)}: No added / modified jobs, skipping lint")
        return []

    return jobs


def load_or_generate_gitlab_ci_configs(ctx, yaml_to_load: str | None = None, **kwargs) -> dict[str, dict]:
    """
    Load a "full" gitlabci config object from file, or re-generate it if needed.
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
    """
    Similar to `load_or_generate_gitlab_ci_configs`, but returns a 'diff triplet'.

    We call a "diff triplet" a triplet of `(diff, before_config, after_config)`, as generated by the `compute_gitlab_ci_config` task.
    Any extra kwargs are passed as-is to `compute_gitlab_ci_config_diff`.
    """
    if yaml_to_load:
        with open(yaml_to_load) as f:
            diff = MultiGitlabCIDiff.from_dict(yaml.safe_load(f))
            return diff.before, diff.after, diff  # type: ignore

    before, after, diff = compute_gitlab_ci_config_diff(ctx, **kwargs)
    return before, after, diff
