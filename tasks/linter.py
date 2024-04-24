import os
import re
import sys
from collections import defaultdict
from typing import List

from invoke import Exit, task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.flavor import AgentFlavor
from tasks.go import run_golangci_lint
from tasks.libs.ciproviders.github_api import GithubAPI
from tasks.libs.ciproviders.gitlab_api import (
    generate_gitlab_full_configuration,
    get_gitlab_repo,
    get_preset_contexts,
    load_context,
)
from tasks.libs.common.check_tools_version import check_tools_version
from tasks.libs.common.utils import DEFAULT_BRANCH, GITHUB_REPO_NAME, color_message, is_pr_context
from tasks.libs.types.copyright import CopyrightLinter
from tasks.modules import GoModule
from tasks.test_core import ModuleLintResult, process_input_args, process_module_results, test_core
from tasks.update_go import _update_go_mods, _update_references


@task
def python(ctx):
    """
    Lints Python files.
    See 'setup.cfg' and 'pyproject.toml' file for configuration.
    If running locally, you probably want to use the pre-commit instead.
    """

    print(
        f"""Remember to set up pre-commit to lint your files before committing:
    https://github.com/DataDog/datadog-agent/blob/{DEFAULT_BRANCH}/docs/dev/agent_dev_env.md#pre-commit-hooks"""
    )

    ctx.run("ruff check --fix .")
    ctx.run("vulture --ignore-decorators @task --ignore-names 'test_*,Test*' tasks")


@task
def copyrights(_, fix=False, dry_run=False, debug=False):
    """
    Checks that all Go files contain the appropriate copyright header. If '--fix'
    is provided as an option, it will try to fix problems as it finds them. If
    '--dry_run' is provided when fixing, no changes to the files will be applied.
    """

    CopyrightLinter(debug=debug).assert_compliance(fix=fix, dry_run=dry_run)


@task
def filenames(ctx):
    """
    Scan files to ensure there are no filenames too long or containing illegal characters
    """
    files = ctx.run("git ls-files -z", hide=True).stdout.split("\0")
    failure = False

    if sys.platform == 'win32':
        print("Running on windows, no need to check filenames for illegal characters")
    else:
        print("Checking filenames for illegal characters")
        forbidden_chars = '<>:"\\|?*'
        for file in files:
            if any(char in file for char in forbidden_chars):
                print(f"Error: Found illegal character in path {file}")
                failure = True

    print("Checking filename length")
    # Approximated length of the prefix of the repo during the windows release build
    prefix_length = 160
    # Maximum length supported by the win32 API
    max_length = 255
    for file in files:
        if (
            not file.startswith(
                ('test/kitchen/', 'tools/windows/DatadogAgentInstaller', 'test/workload-checks', 'test/regression')
            )
            and prefix_length + len(file) > max_length
        ):
            print(f"Error: path {file} is too long ({prefix_length + len(file) - max_length} characters too many)")
            failure = True

    if failure:
        raise Exit(code=1)


@task(iterable=['flavors'])
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
    arch="x64",
    cpus=None,
    timeout: int = None,
    golangci_lint_kwargs="",
    headless_mode=False,
    include_sds=False,
):
    """
    Run go linters on the given module and targets.

    A module should be provided as the path to one of the go modules in the repository.

    Targets should be provided as a comma-separated list of relative paths within the given module.
    If targets are provided but no module is set, the main module (".") is used.

    If no module or target is set the tests are run against all modules and targets.

    --timeout is the number of minutes after which the linter should time out.
    --headless-mode allows you to output the result in a single json file.

    Example invokation:
        inv linter.go --targets=./pkg/collector/check,./pkg/aggregator
        inv linter.go --module=.
    """
    _lint_go(
        ctx=ctx,
        module=module,
        targets=targets,
        flavor=flavor,
        build=build,
        build_tags=build_tags,
        build_include=build_include,
        build_exclude=build_exclude,
        rtloader_root=rtloader_root,
        arch=arch,
        cpus=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
        include_sds=include_sds,
    )


# Temporary method to duplicate go linter task not to impact macos jobs.
def _lint_go(
    ctx,
    module,
    targets,
    flavor,
    build,
    build_tags,
    build_include,
    build_exclude,
    rtloader_root,
    arch,
    cpus,
    timeout,
    golangci_lint_kwargs,
    headless_mode,
    include_sds,
):
    if not check_tools_version(ctx, ['go', 'golangci-lint']):
        print("Warning: If you have linter errors it might be due to version mismatches.", file=sys.stderr)

    modules, flavor = process_input_args(module, targets, flavor, headless_mode)

    lint_results = run_lint_go(
        ctx=ctx,
        modules=modules,
        flavor=flavor,
        build=build,
        build_tags=build_tags,
        build_include=build_include,
        build_exclude=build_exclude,
        rtloader_root=rtloader_root,
        arch=arch,
        cpus=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
        include_sds=include_sds,
    )

    success = process_module_results(flavor=flavor, module_results=lint_results)

    if success:
        if not headless_mode:
            print(color_message("All linters passed", "green"))
    else:
        # Exit if any of the modules failed on any phase
        raise Exit(code=1)


def run_lint_go(
    ctx,
    modules=None,
    flavor=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    arch="x64",
    cpus=None,
    timeout=None,
    golangci_lint_kwargs="",
    headless_mode=False,
    include_sds=False,
):
    linter_tags = build_tags or compute_build_tags_for_flavor(
        flavor=flavor,
        build=build,
        arch=arch,
        build_include=build_include,
        build_exclude=build_exclude,
        include_sds=include_sds,
    )

    lint_results = lint_flavor(
        ctx,
        modules=modules,
        flavor=flavor,
        build_tags=linter_tags,
        arch=arch,
        rtloader_root=rtloader_root,
        concurrency=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
    )

    return lint_results


def lint_flavor(
    ctx,
    modules: List[GoModule],
    flavor: AgentFlavor,
    build_tags: List[str],
    arch: str,
    rtloader_root: bool,
    concurrency: int,
    timeout=None,
    golangci_lint_kwargs: str = "",
    headless_mode: bool = False,
):
    """
    Runs linters for given flavor, build tags, and modules.
    """

    def command(module_results, module: GoModule, module_result):
        with ctx.cd(module.full_path()):
            lint_results = run_golangci_lint(
                ctx,
                module_path=module.path,
                targets=module.lint_targets,
                rtloader_root=rtloader_root,
                build_tags=build_tags,
                arch=arch,
                concurrency=concurrency,
                timeout=timeout,
                golangci_lint_kwargs=golangci_lint_kwargs,
                headless_mode=headless_mode,
            )
            for lint_result in lint_results:
                module_result.lint_outputs.append(lint_result)
                if lint_result.exited != 0:
                    module_result.failed = True
        module_results.append(module_result)

    return test_core(modules, flavor, ModuleLintResult, "golangci_lint", command, headless_mode=headless_mode)


@task
def list_ssm_parameters(_):
    """
    List all SSM parameters used in the datadog-agent repository.
    """

    ssm_owner = re.compile(r"^[A-Z].*_SSM_(NAME|KEY): (?P<param>[^ ]+) +# +(?P<owner>.+)$")
    ssm_params = defaultdict(list)
    with open(".gitlab-ci.yml") as f:
        for line in f:
            m = ssm_owner.match(line.strip())
            if m:
                ssm_params[m.group("owner")].append(m.group("param"))
    for owner in ssm_params.keys():
        print(f"Owner:{owner}")
        for param in ssm_params[owner]:
            print(f"  - {param}")


@task
def ssm_parameters(ctx):
    """
    Lint SSM parameters in the datadog-agent repository.
    """
    lint_folders = [".circleci", ".github", ".gitlab", "tasks", "test"]
    repo_files = ctx.run("git ls-files", hide="both")
    error_files = []
    for file in repo_files.stdout.split("\n"):
        if any(file.startswith(f) for f in lint_folders):
            matched = is_get_parameter_call(file)
            if matched:
                error_files.append(matched)
    if error_files:
        print("The following files contain unexpected syntax for aws ssm get-parameter:")
        for file in error_files:
            print(f"  - {file}")
        raise Exit(code=1)


class SSMParameterCall:
    def __init__(self, file, line_nb, with_wrapper=False, with_env_var=False):
        self.file = file
        self.line_nb = line_nb
        self.with_wrapper = with_wrapper
        self.with_env_var = with_env_var

    def __str__(self):
        message = ""
        if not self.with_wrapper:
            message += "Please use the dedicated `aws_ssm_get_wrapper.(sh|ps1)`."
        if not self.with_env_var:
            message += " Save your parameter name as environment variable in .gitlab-ci.yml file."
        return f"{self.file}:{self.line_nb+1}. {message}"

    def __repr__(self):
        return str(self)


def is_get_parameter_call(file):
    ssm_get = re.compile(r"^.+ssm.get.+$")
    aws_ssm_call = re.compile(r"^.+ssm get-parameter.+--name +(?P<param>[^ ]+).*$")
    ssm_wrapper_call = re.compile(r"^.+aws_ssm_get_wrapper.(sh|ps1) +(?P<param>[^ )]+).*$")
    with open(file) as f:
        try:
            for nb, line in enumerate(f):
                is_ssm_get = ssm_get.match(line.strip())
                if is_ssm_get:
                    m = aws_ssm_call.match(line.strip())
                    if m:
                        return SSMParameterCall(
                            file, nb, with_wrapper=False, with_env_var=m.group("param").startswith("$")
                        )
                    m = ssm_wrapper_call.match(line.strip())
                    if m and not m.group("param").startswith("$"):
                        return SSMParameterCall(file, nb, with_wrapper=True, with_env_var=False)
        except UnicodeDecodeError:
            pass


@task
def gitlab_ci(_, test="all", custom_context=None):
    """
    Lint Gitlab CI files in the datadog-agent repository.
    """
    all_contexts = []
    if custom_context:
        all_contexts = load_context(custom_context)
    else:
        all_contexts = get_preset_contexts(test)
    print(f"We will tests {len(all_contexts)} contexts.")
    agent = get_gitlab_repo()
    for context in all_contexts:
        print("Test gitlab configuration with context: ", context)
        config = generate_gitlab_full_configuration(".gitlab-ci.yml", dict(context))
        res = agent.ci_lint.create({"content": config})
        status = color_message("valid", "green") if res.valid else color_message("invalid", "red")
        print(f"Config is {status}")
        if len(res.warnings) > 0:
            print(color_message(f"Warnings: {res.warnings}", "orange"), file=sys.stderr)
        if not res.valid:
            print(color_message(f"Errors: {res.errors}", "red"), file=sys.stderr)
            raise Exit(code=1)


@task
def releasenote(ctx):
    """
    Lint release notes with Reno
    """
    branch = os.environ.get("BRANCH_NAME")
    pr_id = os.environ.get("PR_ID")

    run_check = is_pr_context(branch, pr_id, "release note")
    if run_check:
        github = GithubAPI(repository=GITHUB_REPO_NAME, public_repo=True)
        if github.is_release_note_needed(pr_id):
            if not github.contains_release_note(pr_id):
                print(
                    f"{color_message('Error', 'red')}: No releasenote was found for this PR. Please add one using 'reno'"
                    ", see https://github.com/DataDog/datadog-agent/blob/main/docs/dev/contributing.md#reno"
                    ", or apply the label 'changelog/no-changelog' to the PR.",
                    file=sys.stderr,
                )
                raise Exit(code=1)
            ctx.run("reno lint")
        else:
            print("'changelog/no-changelog' label found on the PR: skipping linting")


@task
def update_go(_):
    _update_references(warn=False, version="1.2.3", dry_run=True)
    _update_go_mods(warn=False, version="1.2.3", include_otel_modules=True, dry_run=True)
