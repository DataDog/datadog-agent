"""
High level testing tasks
"""
from __future__ import print_function

import os
import fnmatch

import invoke
from invoke import task

from .utils import pkg_config_path, get_version
from .go import fmt, lint, vet, misspell, ineffassign
from .build_tags import get_default_build_tags
from .agent import integration_tests as agent_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests

PROFILE_COV = "profile.cov"

# List of packages to ignore when running tests on Windows platform
WIN_PKG_BLACKLIST = [
    "./pkg\\util\\xc",
]

DEFAULT_TOOL_TARGETS = [
    "./pkg",
    "./cmd",
]

DEFAULT_TEST_TARGETS = [
    "./pkg",
]


@task()
def test(ctx, targets=None, coverage=False, race=False, use_embedded_libs=False, fail_on_fmt=False):
    """
    Run all the tools and tests on the given targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
    """
    if isinstance(targets, basestring):
        # when this function is called from the command line, targets are passed
        # as comma separated tokens in a string
        tool_targets = test_targets = targets.split(',')
    elif targets is None:
        tool_targets = DEFAULT_TOOL_TARGETS
        test_targets = DEFAULT_TEST_TARGETS
    else:
        tool_targets = test_targets = targets

    build_tags = get_default_build_tags()

    # explicitly run these tasks instead of using pre-tasks so we can
    # pass the `target` param (pre-tasks are invoked without parameters)
    fmt(ctx, targets=tool_targets, fail_on_fmt=fail_on_fmt)
    lint(ctx, targets=tool_targets)
    vet(ctx, targets=tool_targets)
    misspell(ctx, targets=tool_targets)
    ineffassign(ctx, targets=tool_targets)

    with open(PROFILE_COV, "w") as f_cov:
        f_cov.write("mode: count")

    env = {
        "PKG_CONFIG_PATH": pkg_config_path(use_embedded_libs)
    }

    race_opt = ""
    covermode_opt = ""
    if race:
        race_opt = "-race"
    if coverage:
        if race:
            # atomic is quite expensive but it's the only way to run
            # both the coverage and the race detector at the same time
            # without getting false positives from the cover counter
            covermode_opt = "-covermode=atomic"
        else:
            covermode_opt = "-covermode=count"

    if race or coverage:
        matches = []
        for target in test_targets:
            for root, _, filenames in os.walk(target):
                if fnmatch.filter(filenames, "*.go"):
                    matches.append(root)
    else:
        matches = ["{}/...".format(t) for t in test_targets]

    for match in matches:
        if invoke.platform.WINDOWS:
            if match in WIN_PKG_BLACKLIST:
                print("Skipping blacklisted directory {}\n".format(match))
                continue

        coverprofile = ""
        if coverage:
            profile_tmp = "{}/profile.tmp".format(match)
            coverprofile = "-coverprofile={}".format(profile_tmp)
        cmd = 'go test -tags "{go_build_tags}" {race_opt} -short {covermode_opt} {coverprofile} {pkg_folder}'
        args = {
            "go_build_tags": " ".join(build_tags),
            "race_opt": race_opt,
            "covermode_opt": covermode_opt,
            "coverprofile": coverprofile,
            "pkg_folder": match,
        }
        ctx.run(cmd.format(**args), env=env)

        if coverage:
            if os.path.exists(profile_tmp):
                ctx.run("cat {} | tail -n +2 >> {}".format(profile_tmp, PROFILE_COV))
                os.remove(profile_tmp)

    if coverage:
        ctx.run("go tool cover -func {}".format(PROFILE_COV))

@task
def lint_releasenote(ctx):
    """
    Lint release notes with Reno
    """
    ctx.run("reno lint")

@task
def integration_tests(ctx, install_deps=False, race=False, remote_docker=False):
    """
    Run all the available integration tests
    """
    agent_integration_tests(ctx, install_deps, race, remote_docker)
    dsd_integration_tests(ctx, install_deps, race, remote_docker)


@task
def version(ctx):
    print(get_version(ctx, include_git=True))
