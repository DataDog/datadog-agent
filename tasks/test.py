"""
High level testing tasks
"""
from __future__ import print_function

import os
import fnmatch

from invoke import task

from .utils import pkg_config_path
from .go import fmt, lint, vet
from .build_tags import get_build_tags
from .agent import integration_tests as agent_integration_tests
from .dogstatsd import integration_tests as dsd_integration_tests

PROFILE_COV = "profile.cov"


@task()
def test(ctx, targets=None, coverage=False, race=False, use_embedded_libs=False, fail_on_fmt=False):
    """
    Run all the tests on the given targets. If targets are not specified,
    the value from `invoke.yaml` will be used.

    Example invokation:
        inv test --targets=./pkg/collector/check,./pkg/aggregator --race
    """
    targets_list = ctx.targets if targets is None else targets.split(',')
    build_tags = get_build_tags()  # pass all the build flags for tests

    # explicitly run these tasks instead of using pre-tasks so we can
    # pass the `target` param (pre-tasks are invoked without parameters)
    fmt(ctx, targets=targets, fail_on_fmt=fail_on_fmt)
    lint(ctx, targets=targets)
    vet(ctx, targets=targets)

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
        for target in targets_list:
            for root, _, filenames in os.walk(target):
                if fnmatch.filter(filenames, "*.go"):
                    matches.append(root)
    else:
        matches = ["{}/...".format(t) for t in targets_list]

    for match in matches:
        coverprofile = ""
        if coverage:
            profile_tmp = "{}/profile.tmp".format(match)
            coverprofile = "-coverprofile={}".format(profile_tmp)
        cmd = "go test -tags '{go_build_tags}' {race_opt} -short {covermode_opt} {coverprofile} {pkg_folder}"
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
def integration_tests(ctx, install_deps=False):
    """
    Run all the available integration tests
    """
    agent_integration_tests(ctx, install_deps)
    dsd_integration_tests(ctx, install_deps)
