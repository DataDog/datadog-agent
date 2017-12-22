"""
Pylauncher tasks
"""
from __future__ import print_function
import os

from invoke import task

from .build_tags import get_default_build_tags
from .utils import REPO_PATH, bin_name, get_build_flags, get_root, pkg_config_path


#constants
PYLAUNCHER_BIN_PATH = os.path.join(get_root(), "bin", "pylauncher")

@task
def build(ctx, rebuild=False, use_embedded_libs=False):
    """
    Build the pylauncher executable
    """
    build_tags = get_default_build_tags()  # pass all the build flags
    env = {
        "PKG_CONFIG_PATH": pkg_config_path(use_embedded_libs)
    }
    ldflags, gcflags = get_build_flags(ctx, use_embedded_libs=use_embedded_libs)

    cmd = "go build {build_type} -tags \"{build_tags}\" -o {bin_name} "
    cmd += "-gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/py-launcher/"
    args = {
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(PYLAUNCHER_BIN_PATH, bin_name("pylauncher")),
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args), env=env)


@task()
def system_tests(ctx, skip_build=False):
    """
    Run the system testsuite.
    """
    if not skip_build:
        print("Building pylauncher...")
        build(ctx)

    env = {
        "PYLAUNCHER_BIN": os.path.join(PYLAUNCHER_BIN_PATH, bin_name("pylauncher"))
    }
    with ctx.cd("./test/system/python_binding"):
        ctx.run("./test.sh", env=env)
