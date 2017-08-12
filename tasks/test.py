"""
High level testing tasks
"""
from __future__ import print_function

import os
import fnmatch

from invoke import task

from .utils import pkg_config_path
from .go import fmt, lint, vet

PROFILE_COV = "profile.cov"


@task(pre=[fmt, lint, vet])
def test(ctx, targets=None, race=False):
    """
    Run all the tests
    """
    if targets is None:
        targets = ctx.targets

    with open(PROFILE_COV, "w") as f:
        f.write("mode: count")

    env = {}
    if not ctx.use_system_libs:
        env["PKG_CONFIG_LIBDIR"] = pkg_config_path()

    if race:
        # atomic is quite expensive but it's the only way to run
        # both the coverage and the race detector at the same time
        # without getting false positives from the cover counter
        covermode_opt = "-covermode=atomic"
        race_opt = "-race"
    else:
        covermode_opt = "-covermode=count"
        race_opt = ""

    matches = []
    for target in targets:
        for root, _, filenames in os.walk(target):
            if fnmatch.filter(filenames, "*.go"):
                matches.append(root)

    for match in matches:
        profile_tmp = "{}/profile.tmp".format(match)
        cmd = "go test -tags '{go_build_tags}' {race_opt} -short {covermode_opt} -coverprofile={profile_tmp} ./{pkg_folder}"
        args = {
            "go_build_tags": "",
            "race_opt": race_opt,
            "covermode_opt": covermode_opt,
            "profile_tmp": profile_tmp,
            "pkg_folder": match,
        }
        ctx.run(cmd.format(**args), env=env)

        if os.path.exists(profile_tmp):
            ctx.run("cat {} | tail -n +2 >> {}".format(profile_tmp, PROFILE_COV))
            os.remove(profile_tmp)

    ctx.run("go tool cover -func {}".format(PROFILE_COV))
