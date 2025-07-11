"""Linting-related tasks for go files"""

import os

from tasks.build_tags import compute_build_tags_for_flavor
from tasks.flavor import AgentFlavor
from tasks.go import run_golangci_lint
from tasks.modules import GoModule
from tasks.test_core import LintResult


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
    recursive=True,
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
        recursive=recursive,
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
    recursive: bool = True,
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
        recursive=recursive,
    )
    for lint_result in lint_results:
        result.lint_outputs.append(lint_result)
        if lint_result.exited != 0:
            result.failed = True

    return result, execution_times
