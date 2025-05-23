from __future__ import annotations

import os
import re
import sys
from collections import defaultdict
from fnmatch import fnmatch
from glob import glob
from tempfile import TemporaryDirectory

import yaml
from invoke import Exit, task

from tasks.libs.ciproviders.ci_config import CILintersConfig
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import (
    MultiGitlabCIDiff,
    full_config_get_all_leaf_jobs,
    full_config_get_all_stages,
    generate_gitlab_full_configuration,
    get_all_gitlab_ci_configurations,
    get_gitlab_ci_configuration,
    get_preset_contexts,
    is_leaf_job,
    load_context,
    read_includes,
    retrieve_all_paths,
    test_gitlab_configuration,
)
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.git import get_file_modifications
from tasks.libs.common.utils import gitlab_section, is_pr_context
from tasks.libs.owners.parsing import read_owners

# - SC2086 corresponds to using variables in this way $VAR instead of "$VAR" (used in every jobs).
# - SC2016 corresponds to avoid using '$VAR' inside single quotes since it doesn't expand.
# - SC2046 corresponds to avoid using $(...) to prevent word splitting.
DEFAULT_SHELLCHECK_EXCLUDES = 'SC2059,SC2028,SC2086,SC2016,SC2046'


@task
def list_parameters(_, type):
    """
    List all SSM parameters used in the datadog-agent repository.
    """
    if type == "ssm":
        section_pattern = re.compile(r"aws ssm variables")
    elif type == "vault":
        section_pattern = re.compile(r"vault variables")
    else:
        raise Exit(f"{color_message('Error', Color.RED)}: pattern must be in [ssm, vault], not |{type}|")
    in_param_section = False
    param_owner = re.compile(r"^[^:]+: (?P<param>[^ ]+) +# +(?P<owner>.+)$")
    params = defaultdict(list)
    with open(".gitlab-ci.yml") as f:
        for line in f:
            section = section_pattern.search(line)
            if section:
                in_param_section = not in_param_section
            if in_param_section:
                if len(line.strip()) == 0:
                    break
                m = param_owner.match(line.strip())
                if m:
                    params[m.group("owner")].append(m.group("param"))
    for owner in params.keys():
        print(f"Owner:{owner}")
        for param in params[owner]:
            print(f"  - {param}")


@task
def ssm_parameters(ctx, mode="all", folders=None):
    """Lints SSM parameters in the datadog-agent repository."""

    modes = ["env", "wrapper", "all"]
    if mode not in modes:
        raise Exit(f"Invalid mode: {mode}. Must be one of {modes}")
    if folders is None:
        lint_folders = [".github", ".gitlab", "test"]
    else:
        lint_folders = folders.split(",")
    repo_files = ctx.run("git ls-files", hide="both")
    error_files = []
    for filename in repo_files.stdout.split("\n"):
        if any(filename.startswith(f) for f in lint_folders):
            calls = list_get_parameter_calls(filename)
            if calls:
                error_files.extend(calls)
    if mode == "env":
        error_files = [f for f in error_files if not f.with_env_var]
    elif mode == "wrapper":
        error_files = [f for f in error_files if not f.with_wrapper]
    if error_files:
        print(
            f"[{color_message('ERROR', Color.RED)}] The following files contain unexpected syntax for aws ssm get-parameter:"
        )
        for filename in error_files:
            print(f"  - {filename}")
        raise Exit(code=1)
    print(f"[{color_message('OK', Color.GREEN)}] All files are correctly using wrapper for secret parameters.")


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


@task
def gitlab_ci(ctx, test="all", custom_context=None, input_file=".gitlab-ci.yml"):
    """Lints Gitlab CI files in the datadog-agent repository.

    This will lint the main gitlab ci file with different
    variable contexts and lint other triggered gitlab ci configs.

    Args:
        test: The context preset to test the gitlab ci file with containing environment variables.
        custom_context: A custom context to test the gitlab ci file with.
    """
    print(f'{color_message("info", Color.BLUE)}: Fetching Gitlab CI configurations...')
    configs = get_all_gitlab_ci_configurations(ctx, input_file=input_file, with_lint=False)

    for entry_point, input_config in configs.items():
        with gitlab_section(f"Testing {entry_point}", echo=True):
            # Only the main config should be tested with all contexts
            if entry_point == ".gitlab-ci.yml":
                all_contexts = []
                if custom_context:
                    all_contexts = load_context(custom_context)
                else:
                    all_contexts = get_preset_contexts(test)

                print(f'{color_message("info", Color.BLUE)}: We will test {len(all_contexts)} contexts')
                for context in all_contexts:
                    print("Test gitlab configuration with context: ", context)
                    test_gitlab_configuration(ctx, entry_point, input_config, dict(context))
            else:
                test_gitlab_configuration(ctx, entry_point, input_config)


def get_gitlab_ci_lintable_jobs(diff_file, config_file, only_names=False):
    """Retrieves the jobs from full gitlab ci configuration file or from a diff file.

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config.
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config.
    """

    assert diff_file or config_file and not (diff_file and config_file), (
        "You must provide either a diff file or a config file and not both"
    )

    # Load all the jobs from the files
    if config_file:
        with open(config_file) as f:
            full_config = yaml.safe_load(f)
            jobs = [
                (job, job_contents)
                for contents in full_config.values()
                for job, job_contents in contents.items()
                if is_leaf_job(job, job_contents)
            ]
    else:
        with open(diff_file) as f:
            diff = MultiGitlabCIDiff.from_dict(yaml.safe_load(f))

        full_config = diff.after
        jobs = [(job, contents) for _, job, contents, _ in diff.iter_jobs(added=True, modified=True, only_leaves=True)]

    if not jobs:
        print(f"{color_message('Info', Color.BLUE)}: No added / modified jobs, skipping lint")
        return [], {}

    if only_names:
        jobs = [job for job, _ in jobs]

    return jobs, full_config


@task
def gitlab_ci_jobs_needs_rules(_, diff_file=None, config_file=None):
    """Verifies that each added / modified job contains `needs` and also `rules`.

    It is possible to declare a job not following these rules within `.gitlab/.ci-linters.yml`.
    All configurations are checked (even downstream ones).

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config

    See:
      https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/4059234597/Gitlab+CI+configuration+guidelines#datadog-agent
    """

    jobs, full_config = get_gitlab_ci_lintable_jobs(diff_file, config_file)

    # No change, info already printed in get_gitlab_ci_lintable_jobs
    if not full_config:
        return

    ci_linters_config = CILintersConfig(
        lint=True,
        all_jobs=full_config_get_all_leaf_jobs(full_config),
        all_stages=full_config_get_all_stages(full_config),
    )

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


@task
def releasenote(ctx):
    """Lints release notes with Reno."""

    branch = os.environ.get("BRANCH_NAME")
    pr_id = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_id, "release note")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        if github.is_release_note_needed(pr_id):
            if not github.contains_release_note(pr_id):
                print(
                    f"{color_message('Error', 'red')}: No releasenote was found for this PR. Please add one using 'reno'"
                    ", see https://datadoghq.dev/datadog-agent/guidelines/contributing/#reno"
                    ", or apply the label 'changelog/no-changelog' to the PR.",
                    file=sys.stderr,
                )
                raise Exit(code=1)
            ctx.run("reno lint")
        else:
            print("'changelog/no-changelog' label found on the PR: skipping linting")


@task(iterable=['job_files'])
def job_change_path(ctx, job_files=None):
    """Verifies that the jobs defined within job_files contain a change path rule."""

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

    job_files = job_files or (['.gitlab/e2e/e2e.yml'] + list(glob('.gitlab/e2e/install_packages/*.yml')))

    # Read and parse gitlab config
    # The config is filtered to only include jobs
    config = get_gitlab_ci_configuration(ctx, ".gitlab-ci.yml")

    # Fetch all test jobs
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
        if "rules" in config[test] and not any(
            contains_valid_change_rule(rule) for rule in config[test]['rules'] if isinstance(rule, dict)
        ):
            if test in tests_without_change_path_allow_list:
                tests_without_change_path_allowed[filepath].append(test)
            else:
                tests_without_change_path[filepath].append(test)

    if len(tests_without_change_path_allowed) != 0:
        with gitlab_section('Allow-listed jobs', collapsed=True):
            print(
                color_message(
                    'warning: The following tests do not contain required change paths rule but are allowed:',
                    Color.ORANGE,
                )
            )
            for filepath, tests in tests_without_change_path_allowed.items():
                print(f"- {color_message(filepath, Color.BLUE)}: {', '.join(tests)}")
            print(color_message('warning: End of allow-listed jobs', Color.ORANGE))
            print()

    if len(tests_without_change_path) != 0:
        print(color_message("error: Tests without required change paths rule:", "red"), file=sys.stderr)
        for filepath, tests in tests_without_change_path.items():
            print(f"- {color_message(filepath, Color.BLUE)}: {', '.join(tests)}", file=sys.stderr)

        raise RuntimeError(
            color_message(
                'Some tests do not contain required change paths rule, they must contain at least one non-test path.',
                Color.RED,
            )
        )
    else:
        print(color_message("success: All tests contain a change paths rule or are allow-listed", "green"))


@task
def gitlab_change_paths(ctx):
    """Verifies that rules: changes: paths match existing files in the repository."""

    # Read gitlab config
    config = generate_gitlab_full_configuration(ctx, ".gitlab-ci.yml", {}, return_dump=False, apply_postprocessing=True)
    error_paths = []
    for path in set(retrieve_all_paths(config)):
        files = glob(path, recursive=True)
        if len(files) == 0:
            error_paths.append(path)
    if error_paths:
        raise Exit(
            f"{color_message('No files found for paths', Color.RED)}:\n{chr(10).join(' - ' + path for path in error_paths)}"
        )
    print(f"All rule:changes:paths from gitlab-ci are {color_message('valid', Color.GREEN)}.")


def _gitlab_ci_jobs_owners_lint(jobs, jobowners, ci_linters_config, path_jobowners):
    error_jobs = []
    n_ignored = 0
    for job in jobs:
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
    else:
        print(f'{color_message("Success", Color.GREEN)}: All jobs have owners defined in {path_jobowners}')


@task
def gitlab_ci_jobs_owners(_, diff_file=None, config_file=None, path_jobowners='.gitlab/JOBOWNERS'):
    """Verifies that each job is defined within JOBOWNERS files.

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config
        path_jobowners: Path to the JOBOWNERS file
    """

    jobs, full_config = get_gitlab_ci_lintable_jobs(diff_file, config_file, only_names=True)

    # No change, info already printed in get_gitlab_ci_lintable_jobs
    if not full_config:
        return

    ci_linters_config = CILintersConfig(
        lint=True,
        all_jobs=full_config_get_all_leaf_jobs(full_config),
        all_stages=full_config_get_all_stages(full_config),
    )

    jobowners = read_owners(path_jobowners, remove_default_pattern=True)

    _gitlab_ci_jobs_owners_lint(jobs, jobowners, ci_linters_config, path_jobowners)


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


@task
def gitlab_ci_jobs_codeowners(ctx, path_codeowners='.github/CODEOWNERS', all_files=False):
    """Verifies that added / modified job files are defined within CODEOWNERS.

    Args:
        all_files: If True, lint all job files. If False, lint only added / modified job.
    """

    from codeowners import CodeOwners

    if all_files:
        modified_yml_files = glob('.gitlab/**/*.yml', recursive=True)
    else:
        modified_yml_files = get_file_modifications(ctx, added=True, modified=True, only_names=True)
        modified_yml_files = [path for path in modified_yml_files if fnmatch(path, '.gitlab/**.yml')]

    if not modified_yml_files:
        print(f'{color_message("Info", Color.BLUE)}: No added / modified job files, skipping lint')
        return

    with open(path_codeowners) as f:
        parsed_owners = f.readlines()

    # Keep only gitlab related lines to avoid defaults
    parsed_owners = [line for line in parsed_owners if '/.gitlab/' in line]
    gitlab_owners = CodeOwners('\n'.join(parsed_owners))

    _gitlab_ci_jobs_codeowners_lint(path_codeowners, modified_yml_files, gitlab_owners)


def flatten_script(script: str | list[str]) -> str:
    """Flatten a script into a single string."""

    if isinstance(script, list):
        return '\n'.join(flatten_script(line) for line in script)

    if script is None:
        return ''

    return script.strip()


def shellcheck_linter(
    ctx,
    scripts: dict[str, str],
    exclude: str,
    shellcheck_args: str,
    fail_fast: bool,
    use_bat: str | None,
    only_errors=False,
):
    """Lints bash scripts within `scripts` using shellcheck.

    Args:
        scripts: A dictionary of job names and their scripts.
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        use_bat: If True (or None), will (try to) use bat to display the script.
        only_errors: Show only errors, not warnings.

    Note:
        Will raise an Exit if any errors are found.
    """

    exclude = ' '.join(f'-e {e}' for e in exclude.split(','))

    if use_bat is None:
        use_bat = ctx.run('which bat', warn=True, hide=True)
    elif use_bat.casefold() == 'false':
        use_bat = False

    results = {}
    with TemporaryDirectory() as tmpdir:
        for i, (script_name, script) in enumerate(scripts.items()):
            with open(f'{tmpdir}/{i}.sh', 'w') as f:
                f.write(script)

            res = ctx.run(f"shellcheck {shellcheck_args} {exclude} '{tmpdir}/{i}.sh'", warn=True, hide=True)
            if res.stderr or res.stdout:
                if res.return_code or not only_errors:
                    results[script_name] = {
                        'output': (res.stderr + '\n' + res.stdout + '\n').strip(),
                        'code': res.return_code,
                        'id': i,
                    }

                if res.return_code and fail_fast:
                    break

        if results:
            with gitlab_section(color_message("Shellcheck errors / warnings", color=Color.ORANGE), collapsed=True):
                for script, result in sorted(results.items()):
                    with gitlab_section(f"Shellcheck errors for {script}"):
                        print(f"--- {color_message(script, Color.BLUE)} ---")
                        print(f'[{script}] Script:')
                        if use_bat:
                            res = ctx.run(
                                f"bat --color=always --file-name={script} -l bash {tmpdir}/{result['id']}.sh", hide=True
                            )
                            # Avoid buffering issues
                            print(res.stderr)
                            print(res.stdout)
                        else:
                            with open(f'{tmpdir}/{result["id"]}.sh') as f:
                                print(f.read())
                        print(f'\n[{script}] {color_message("Error", Color.RED)}:')
                        print(result['output'])

            if any(result['code'] != 0 for result in results.values()):
                raise Exit(
                    f"{color_message('Error', Color.RED)}: {len(results)} shellcheck errors / warnings found, please fix them",
                    code=1,
                )


@task
def gitlab_ci_shellcheck(
    ctx,
    diff_file=None,
    config_file=None,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast=False,
    verbose=False,
    use_bat=None,
    only_errors=False,
):
    """Verifies that shell scripts with gitlab config are valid.

    Args:
        diff_file: Path to the diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config.
        config_file: Path to the full gitlab ci configuration file obtained by compute-gitlab-ci-config.
    """

    # Used by the CI to skip linting if no changes
    if diff_file and not os.path.exists(diff_file):
        print('No diff file found, skipping lint')
        return

    jobs, full_config = get_gitlab_ci_lintable_jobs(diff_file, config_file)

    # No change, info already printed in get_gitlab_ci_lintable_jobs
    if not full_config:
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

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, use_bat, only_errors)


@task
def github_actions_shellcheck(
    ctx,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast=False,
    use_bat=None,
    only_errors=False,
    all_files=False,
):
    """Lint github action workflows with shellcheck."""

    if all_files:
        files = glob('.github/workflows/*.yml')
    else:
        files = ctx.run(
            "git diff --name-only \"$(git merge-base main HEAD)\" | grep -E '.github/workflows/.*\\.yml'", warn=True
        ).stdout.splitlines()

    if not files:
        print('No github action workflow files to lint, skipping')
        return

    scripts = {}
    for file in files:
        with open(file) as f:
            workflow = yaml.safe_load(f)

        for job_name, job in workflow.get('jobs').items():
            for i, step in enumerate(job['steps']):
                step_name = step.get('name', f'step-{i + 1:02d}').replace(' ', '_')
                if 'run' in step:
                    script = step['run']
                    if isinstance(script, list):
                        script = '\n'.join(script)

                    # "Escape" ${{...}} which is github actions only syntax
                    script = re.sub(r'\${{(.*)}}', r'\\$\\{\\{\1\\}\\}', script, flags=re.MULTILINE)

                    # We suppose all jobs are bash like scripts and not powershell or other exotic shells
                    script = '#!/bin/bash\n' + script.strip() + '\n'
                    scripts[f'{file.removeprefix(".github/workflows/")}-{job_name}-{step_name}'] = script

    shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, use_bat, only_errors)
