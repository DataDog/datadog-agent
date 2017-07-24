import os
import fnmatch
import platform

import invoke
from invoke import task
from invoke.exceptions import Exit

PROFILE_COV = "profile.cov"

@task
def fmt(ctx, targets=None, fail_on_mod=False):
    """
    Run go fmt on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in targets)

    result = ctx.run("go fmt {}".format(targets))
    if result.stdout:
        files = {x for x in result.stdout.split('\n') if x}
        print "Reformatted the following files: {}".format(','.join(files))
        if fail_on_mod:
            print "Code was not properly formatted, exiting..."
            raise Exit(1)

@task
def lint(ctx, targets=None):
    """
    Run golint on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in targets)

    result = ctx.run("golint {}".format(targets))
    if result.stdout:
        files = {x for x in result.stdout.split('\n') if x}
        print "Linting issues found in files: {}".format(','.join(files))
        raise Exit(1)

@task
def vet(ctx, targets=None):
    """
    Run go vet on targets.
    """
    if targets is None:
        targets = " ".join("./{}/...".format(x) for x in targets)

    ctx.run("go vet {}".format(targets, hide=True))

def bin_name(name):
    """
    Generate platform dependent names for binaries
    """
    if invoke.platform.WINDOWS:
        return "{}.exe".format(name)
    return name

def pkg_config_path():
    path = os.path.join(os.path.dirname("."), "pkg-config", platform.system().lower())
    return os.path.abspath(path)

@task()
def test(ctx, targets=None, race=False):
    if targets is None:
        targets = ctx.targets

    with open(PROFILE_COV, "w") as f:
        f.write("mode: count")

    env = {}
    if not ctx.use_system_py:
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
        for root, dirnames, filenames in os.walk(target):
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
