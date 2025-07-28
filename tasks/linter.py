"""Module regrouping all invoke tasks used for linting the `datadog-agent` repo."""

from __future__ import annotations

import os
import re
import sys
from collections import defaultdict
from fnmatch import fnmatch
from glob import glob

import yaml
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.devcontainer import run_on_devcontainer
from tasks.libs.ciproviders.ci_config import CILintersConfig
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import (
    full_config_get_all_leaf_jobs,
    full_config_get_all_stages,
)
from tasks.libs.common.check_tools_version import check_tools_version
from tasks.libs.common.color import Color, color_message
from tasks.libs.common.constants import GITHUB_REPO_NAME
from tasks.libs.common.git import get_default_branch, get_file_modifications, get_staged_files
from tasks.libs.common.utils import gitlab_section, is_pr_context, running_in_ci
from tasks.libs.linter.gitlab import (
    ALL_GITLABCI_SUBLINTERS,
    PREPUSH_GITLABCI_SUBLINTERS,
    _gitlab_ci_jobs_codeowners_lint,
    check_change_paths_exist_gitlab_ci_jobs,
    check_change_paths_valid_gitlab_ci_jobs,
    check_needs_rules_gitlab_ci_jobs,
    check_owners_gitlab_ci_jobs,
    gitlabci_lint_task_template,
    lint_and_test_gitlab_ci_config,
    list_get_parameter_calls,
    load_or_generate_gitlab_ci_configs,
    shellcheck_gitlab_ci_jobs,
)
from tasks.libs.linter.gitlab_exceptions import (
    FailureLevel,
    GitlabLintFailure,
    MultiGitlabLintFailure,
    SingleGitlabLintFailure,
)
from tasks.libs.linter.go import run_lint_go
from tasks.libs.linter.shell import DEFAULT_SHELLCHECK_EXCLUDES, shellcheck_linter
from tasks.libs.owners.parsing import read_owners
from tasks.libs.types.copyright import CopyrightLinter, LintFailure
from tasks.test_core import process_input_args, process_result
from tasks.update_go import _update_go_mods, _update_references


# === GO === #
@task(iterable=['flavors'])
@run_on_devcontainer
def go(
    ctx,
    module=None,
    targets=None,
    flavor=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    cpus=None,
    timeout: int | None = None,
    golangci_lint_kwargs="",
    headless_mode=False,
    include_sds=False,
    only_modified_packages=False,
    verbose=False,
    run_on=None,  # noqa: U100, F841. Used by the run_on_devcontainer decorator
    debug=False,
):
    """Runs go linters on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    Args:
        timeout: Number of minutes after which the linter should time out.
        headless_mode: Allows you to output the result in a single json file.
        debug: prints the go version and the golangci-lint debug information to help debugging lint discrepancies between versions.

    Example invokation:
        $ dda inv linter.go --targets=./pkg/collector/check,./pkg/aggregator
        $ dda inv linter.go --module=.
    """

    check_tools_version(ctx, ['golangci-lint', 'go'], debug=debug)

    modules, flavor = process_input_args(
        ctx,
        module,
        targets,
        flavor,
        headless_mode,
        build_tags=build_tags,
        only_modified_packages=only_modified_packages,
        lint=True,
    )

    if not modules:
        print(color_message("No modules to lint", "yellow"))
        return

    lint_result, execution_times = run_lint_go(
        ctx=ctx,
        modules=modules,
        flavor=flavor,
        build=build,
        build_tags=build_tags,
        build_include=build_include,
        build_exclude=build_exclude,
        rtloader_root=rtloader_root,
        cpus=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
        include_sds=include_sds,
        verbose=verbose,
        recursive=not only_modified_packages,  # Disable recursive linting when only modified packages is enabled, to avoid linting a package and all its subpackages
    )

    if not headless_mode:
        with gitlab_section('Linter execution time'):
            print(color_message('Execution time summary:', 'bold'))
            for e in execution_times:
                print(f'- {e.name}: {e.duration:.1f}s')

    with gitlab_section('Linter failures'):
        success = process_result(flavor=flavor, result=lint_result)

    if success:
        if not headless_mode:
            print(color_message("All linters passed", "green"))
    else:
        # Exit if any of the modules failed on any phase
        raise Exit(code=1)


@task
def update_go(_):
    _update_references(warn=False, version="1.2.3", dry_run=True)
    _update_go_mods(warn=False, version="1.2.3", include_otel_modules=True, dry_run=True)


# === PYTHON === #
@task
def python(ctx):
    """Lints Python files.

    See 'setup.cfg' and 'pyproject.toml' file for configuration. If
    running locally, you probably want to use the pre-commit instead.
    """

    print(
        f"""Remember to set up pre-commit to lint your files before committing:
    https://github.com/DataDog/datadog-agent/blob/{get_default_branch()}/docs/dev/agent_dev_env.md#pre-commit-hooks"""
    )

    if running_in_ci():
        # We want to the CI to fail if there are any issues
        ctx.run("ruff format --check .")
        ctx.run("ruff check .")
    else:
        # Otherwise we just need to format the files
        ctx.run("ruff format .")
        ctx.run("ruff check --fix .")

    ctx.run("vulture")
    ctx.run("mypy")


# === GITHUB === #
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


@task
def github_actions_shellcheck(
    ctx,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast=False,
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

    try:
        shellcheck_linter(ctx, scripts, exclude, shellcheck_args, fail_fast, only_errors)
    # shellcheck_linter raises GitlabLintFailure, bit of a misnomer here as we are using it for github actions
    except GitlabLintFailure as e:
        print(e.pretty_print())
        raise Exit(code=e.exit_code) from e


# === GITLAB === #
## === Main linter tasks === ##
@task
def full_gitlab_ci(
    ctx,
    configs_or_diff_file: str | None = None,
    use_diff: bool = False,
    verbosity: int = 0,
    fail_fast: bool = False,
    pre_push_linters: bool = False,
    *,
    test: str = "all",
    custom_context: str = "",
    shellcheck_exclude: str = "",
    shellcheck_args: str = "",
    shellcheck_only_errors: bool = False,
    path_jobowners: str = '.gitlab/JOBOWNERS',
):
    """Top-level task for running all gitlabci-related linters.

    Having them all run from a single function like this prevents needing to regenerate the config at every step, which can take a while.

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will lint only the changed jobs, otherwise will lint the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
        fail_fast: If True, will stop at the first linting failure.
        pre_push_linters: If True, will run only the linters configured to run on pre-push, otherwise will run all linters.
        test: The context preset to test the gitlab ci with containing environment variables.
        custom_context: A custom context to test the gitlab ci config with.
        shellcheck_exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        shellcheck_only_errors: Show only shellcheck errors, not warnings.
        path_jobowners: Path to a JOBOWNERS file defining which jobs are owned by which teams
    """

    def body(jobs, full_config, **kwargs):
        failures: list[SingleGitlabLintFailure] = []
        ci_linters_config = CILintersConfig(
            lint=True,
            all_jobs=full_config_get_all_leaf_jobs(full_config),
            all_stages=full_config_get_all_stages(full_config),
        )
        jobowners = read_owners(path_jobowners, remove_default_pattern=True)

        all_args = {
            "full_config": full_config,
            "test": test,
            "custom_context": custom_context,
            "ctx": ctx,
            "jobs": jobs,
            "exclude": shellcheck_exclude,
            "verbose": verbosity > 0,
            "shellcheck_args": shellcheck_args,
            "only_errors": shellcheck_only_errors,
            "ci_linters_config": ci_linters_config,
            "jobowners": jobowners,
        }

        to_run = PREPUSH_GITLABCI_SUBLINTERS if pre_push_linters else ALL_GITLABCI_SUBLINTERS
        min_level = [FailureLevel.ERROR, FailureLevel.WARNING, FailureLevel.IGNORED][verbosity]
        for sublinter in to_run:
            try:
                sublinter(**all_args)
            except GitlabLintFailure as e:
                with gitlab_section(f'GitlabCI linter "{e._linter_name}" failures', collapsed=True, echo=True):
                    # Show everything regardless of verbosity if we are running in CI
                    print(
                        e.pretty_print(
                            min_level=FailureLevel.IGNORED if running_in_ci() else min_level, show_linter_name=False
                        )
                    )
                    print(
                        f'{color_message("Result:", Color.BOLD)} {color_message("PASS", Color.GREEN) if e.exit_code == 0 else color_message("FAIL", Color.RED)} ({e.level.pretty_print()})'
                    )
                if fail_fast and e.level == FailureLevel.ERROR:
                    raise Exit(code=e.exit_code) from e
                failures.extend(e.get_individual_failures())

        # Custom handling for errors, bypassing the default behavior in gitlabci_lint_task_template
        if failures:
            with gitlab_section('GitlabCI linter summary', echo=True):
                # Group failures by linter name
                grouped_linter_failures: defaultdict[str, list[SingleGitlabLintFailure]] = defaultdict(list)
                for failure in failures:
                    if failure.level >= min_level:
                        grouped_linter_failures[failure._linter_name].append(failure)

                for linter, linter_failures in grouped_linter_failures.items():
                    with gitlab_section(f'GitlabCI linter "{linter}" failures', echo=True):
                        for failure in linter_failures:
                            print(failure.pretty_print(min_level=min_level, show_linter_name=False))

                summary_failure = MultiGitlabLintFailure(failures=failures)
                print(
                    f'{color_message("Result:", Color.BOLD)} {color_message("PASS", Color.GREEN) if summary_failure.exit_code == 0 else color_message("FAIL", Color.RED)} ({summary_failure.level.pretty_print()})'
                )
            raise Exit(code=summary_failure.exit_code)

        print(f'[{color_message("OK", Color.GREEN)}] All gitlabci linters passed successfully.')

    gitlabci_lint_task_template(
        body, ctx=ctx, configs_or_diff_file=configs_or_diff_file, use_diff=use_diff, verbosity=verbosity
    )


@task
def gitlab_ci(
    ctx,
    configs_file: str | None = None,
    input_file=".gitlab-ci.yml",
    test="all",
    custom_context=None,
    verbosity: int = 0,
):
    """Lints Gitlab CI files in the datadog-agent repository.

    This will lint the main gitlab ci file with different
    variable contexts and lint other triggered gitlab ci configs.

    Args:
        configs_file: Path to a yaml file containing a full gitlabci config
        input_file: If configs_file is not specified, 'root' gitlabci file from which to generate the config
        test: The context preset to test the gitlab ci file with containing environment variables.
        custom_context: A custom context to test the gitlab ci file with.
    """
    # If we have to generate a gitlabci config object, use a minimally-resolved one
    # We do not need the fully-resolved one here, and this is much faster
    configs = load_or_generate_gitlab_ci_configs(
        ctx, configs_file, input_file=input_file, resolve_only_includes=True, postprocess_options=False
    )
    try:
        lint_and_test_gitlab_ci_config(configs, test=test, custom_context=custom_context)
    except GitlabLintFailure as e:
        min_level = [FailureLevel.ERROR, FailureLevel.WARNING, FailureLevel.IGNORED][verbosity]
        print(e.pretty_print(min_level=min_level))
        raise Exit(code=e.exit_code) from e


@task
def gitlab_ci_shellcheck(
    ctx,
    configs_or_diff_file: str | None = None,
    use_diff: bool = False,
    verbosity: int = 0,
    exclude=DEFAULT_SHELLCHECK_EXCLUDES,
    shellcheck_args="",
    fail_fast: bool = False,
    only_errors: bool = False,
):
    """Verifies that shell scripts with gitlab config are valid.

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will use the diff file to extract jobs, otherwise will use the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
        exclude: A comma separated list of shellcheck error codes to exclude.
        shellcheck_args: Additional arguments to pass to shellcheck.
        fail_fast: If True, will stop at the first error.
        only_errors: Show only errors, not warnings.
    """

    def body(jobs, **kwargs):
        shellcheck_gitlab_ci_jobs(
            ctx,
            jobs,
            exclude=exclude,
            verbose=verbosity > 0,
            shellcheck_args=shellcheck_args,
            fail_fast=fail_fast,
            only_errors=only_errors,
        )

    gitlabci_lint_task_template(
        body,
        ctx=ctx,
        configs_or_diff_file=configs_or_diff_file,
        use_diff=use_diff,
        verbosity=verbosity,
    )


## === SSM-related === ##
@task
def list_parameters(_, type):
    """List all SSM parameters used in the datadog-agent repository."""
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


## === Job structure rules === ##
@task
def gitlab_change_paths(
    ctx,
    configs_or_diff_file: str | None = None,
    use_diff: bool = False,
    verbosity: int = 0,
):
    """
    Verifies that rules: changes: paths match existing files in the repository.

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will use the diff file to extract jobs, otherwise will use the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
    """

    def body(jobs, **kwargs):
        check_change_paths_valid_gitlab_ci_jobs(jobs)

    gitlabci_lint_task_template(
        body,
        ctx=ctx,
        configs_or_diff_file=configs_or_diff_file,
        use_diff=use_diff,
        verbosity=verbosity,
    )


@task
def gitlab_ci_jobs_needs_rules(
    ctx,
    configs_or_diff_file: str | None = None,
    use_diff: bool = False,
    verbosity: int = 0,
):
    """Verifies that each added / modified job contains `needs` and also `rules`.

    It is possible to declare a job not following these rules within `.gitlab/.ci-linters.yml`.
    All configurations are checked (even downstream ones).

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will use the diff file to extract jobs, otherwise will use the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
    See:
      https://datadoghq.atlassian.net/wiki/spaces/ADX/pages/4059234597/Gitlab+CI+configuration+guidelines#datadog-agent
    """

    def body(jobs, full_config, **kwargs):
        ci_linters_config = CILintersConfig(
            lint=True,
            all_jobs=full_config_get_all_leaf_jobs(full_config),
            all_stages=full_config_get_all_stages(full_config),
        )
        check_needs_rules_gitlab_ci_jobs(jobs, ci_linters_config)

    gitlabci_lint_task_template(
        body,
        ctx=ctx,
        configs_or_diff_file=configs_or_diff_file,
        use_diff=use_diff,
        verbosity=verbosity,
    )


@task
def job_change_path(
    ctx, configs_or_diff_file: str | None = None, use_diff: bool = False, verbosity: int = 0, job_pattern: str = ".*"
):
    """Verifies that the jobs defined within job_files contain a change path rule.

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will use the diff file to extract jobs, otherwise will use the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
        job_pattern: Regex pattern for selecting which jobs to run the check on
    """

    def body(jobs, **kwargs):
        compiled_job_pattern = re.compile(job_pattern)
        filtered_jobs = [(job_name, job) for (job_name, job) in jobs if re.match(compiled_job_pattern, job_name)]
        check_change_paths_exist_gitlab_ci_jobs(filtered_jobs)

    gitlabci_lint_task_template(
        body,
        ctx=ctx,
        configs_or_diff_file=configs_or_diff_file,
        use_diff=use_diff,
        verbosity=verbosity,
    )


## === Job ownership === ##
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
        print(f'[{color_message("INFO", Color.BLUE)}] No added / modified job files, skipping lint')
        return

    with open(path_codeowners) as f:
        parsed_owners = f.readlines()

    # Keep only gitlab related lines to avoid defaults
    parsed_owners = [line for line in parsed_owners if '/.gitlab/' in line]
    gitlab_owners = CodeOwners('\n'.join(parsed_owners))

    try:
        _gitlab_ci_jobs_codeowners_lint(modified_yml_files, gitlab_owners)
    except GitlabLintFailure as e:
        print(e.pretty_print())
        raise Exit(code=e.exit_code) from e
    print(f'[{color_message("OK", Color.GREEN)}] All checked job files have a CODEOWNER defined in {path_codeowners}.')


@task
def gitlab_ci_jobs_owners(
    ctx,
    configs_or_diff_file: str | None = None,
    use_diff: bool = False,
    verbosity: int = 0,
    path_jobowners='.gitlab/JOBOWNERS',
):
    """Verifies that each job is defined within JOBOWNERS files.

    Args:
        configs_or_diff_file: Path to either :
            A diff file used to build MultiGitlabCIDiff obtained by compute-gitlab-ci-config. In this case make sure to set `use_diff` to False.
            A 'full' gitlabci config file containing all jobs and stages.
            Will be autogenerated if not specified, based on the value of `use_diff`.
        use_diff: If True, will use the diff file to extract jobs, otherwise will use the full gitlabci config file.
        verbosity: The verbosity level of the linter, 0 will print only errors, 1 for warnings, 2 for info and ignored failures.
        path_jobowners: Path to the JOBOWNERS file
    """

    def body(jobs, full_config, **kwargs):
        ci_linters_config = CILintersConfig(
            lint=True,
            all_jobs=full_config_get_all_leaf_jobs(full_config),
            all_stages=full_config_get_all_stages(full_config),
        )
        jobowners = read_owners(path_jobowners, remove_default_pattern=True)
        check_owners_gitlab_ci_jobs(jobs, ci_linters_config, jobowners=jobowners)

    gitlabci_lint_task_template(
        body,
        ctx=ctx,
        configs_or_diff_file=configs_or_diff_file,
        use_diff=use_diff,
        verbosity=verbosity,
    )


# === MISC === #
@task
def copyrights(ctx, fix=False, dry_run=False, debug=False, only_staged_files=False):
    """Checks that all Go files contain the appropriate copyright header.

    If '--fix' is provided as an option, it will try to fix problems as
    it finds them. If '--dry_run' is provided when fixing, no changes to
    the files will be applied.
    """

    files = None

    if only_staged_files:
        staged_files = get_staged_files(ctx)
        files = [path for path in staged_files if path.endswith(".go")]

    try:
        CopyrightLinter(debug=debug).assert_compliance(fix=fix, dry_run=dry_run, files=files)
    except LintFailure:
        # the linter prints useful messages on its own, so no need to print the exception
        sys.exit(1)


@task
def filenames(ctx):
    """Scans files to ensure there are no filenames too long or containing illegal characters."""

    files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
    failure = False

    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        for filename in files:
            if any(char in filename for char in forbidden_chars):
                print(f"Error: Found illegal character in path {filename}")
                failure = True

    print("Checking filename length")
    # Approximated length of the prefix of the repo during the windows release build
    prefix_length = 160
    # Maximum length supported by the win32 API
    max_length = 255
    for filename in files:
        if (
            not filename.startswith(('tools/windows/DatadogAgentInstaller', 'test/workload-checks', 'test/regression'))
            and prefix_length + len(filename) > max_length
        ):
            print(
                f"Error: path {filename} is too long ({prefix_length + len(filename) - max_length} characters too many)"
            )
            failure = True

    if failure:
        raise Exit(code=1)
