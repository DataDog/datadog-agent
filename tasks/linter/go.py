"""Linting-related tasks for go files"""

import os

from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.go import run_golangci_lint
from tasks.libs.common.check_tools_version import check_tools_version
from tasks.libs.common.color import color_message
from tasks.libs.common.utils import gitlab_section
from tasks.modules import GoModule
from tasks.test_core import LintResult, process_input_args, process_result
from tasks.update_go import _update_go_mods, _update_references


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


def run_lint_go(
    ctx,
    modules=None,
    flavor=None,
    build="lint",
    build_tags=None,
    build_include=None,
    build_exclude=None,
    rtloader_root=None,
    cpus=None,
    timeout=None,
    golangci_lint_kwargs="",
    headless_mode=False,
    include_sds=False,
    verbose=False,
):
    linter_tags = build_tags or compute_build_tags_for_flavor(
        flavor=flavor,
        build=build,
        build_include=build_include,
        build_exclude=build_exclude,
        include_sds=include_sds,
    )

    lint_result, execution_times = lint_flavor(
        ctx,
        modules=modules,
        flavor=flavor,
        build_tags=linter_tags,
        rtloader_root=rtloader_root,
        concurrency=cpus,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
        verbose=verbose,
    )

    return lint_result, execution_times


def lint_flavor(
    ctx,
    modules: list[GoModule],
    flavor: AgentFlavor,
    build_tags: list[str],
    rtloader_root: bool,
    concurrency: int,
    timeout=None,
    golangci_lint_kwargs: str = "",
    headless_mode: bool = False,
    verbose: bool = False,
):
    """Runs linters for given flavor, build tags, and modules."""

    # Compute full list of targets to run linters against
    targets = []
    for module in modules:
        # FIXME: Linters also use the `should_test()` condition. Is this expected?
        if not module.should_test():
            continue
        for target in module.lint_targets:
            target_path = os.path.join(module.path, target)
            if not target_path.startswith('./'):
                target_path = f"./{target_path}"
            targets.append(target_path)

    result = LintResult('.')

    lint_results, execution_times = run_golangci_lint(
        ctx,
        base_path=result.path,
        targets=targets,
        rtloader_root=rtloader_root,
        build_tags=build_tags,
        concurrency=concurrency,
        timeout=timeout,
        golangci_lint_kwargs=golangci_lint_kwargs,
        headless_mode=headless_mode,
        verbose=verbose,
    )
    for lint_result in lint_results:
        result.lint_outputs.append(lint_result)
        if lint_result.exited != 0:
            result.failed = True

    return result, execution_times
