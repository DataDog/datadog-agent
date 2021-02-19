"""
Pylauncher tasks
"""


import os

from invoke import task

from .build_tags import get_default_build_tags
from .utils import REPO_PATH, bin_name, get_root

# constants
PYLAUNCHER_BIN_PATH = os.path.join(get_root(), "bin", "pylauncher")


@task
def build(ctx, rebuild=False, arch="x64"):
    """
    Build the pylauncher executable
    """
    build_tags = get_default_build_tags(build="test", arch=arch)  # pass all the build flags

    cmd = "go build -mod={go_mod} {build_type} -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/cmd/py-launcher/"
    args = {
        "go_mod": "mod",
        "build_type": "-a" if rebuild else "",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(PYLAUNCHER_BIN_PATH, bin_name("pylauncher")),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))


@task()
def system_tests(ctx, skip_build=False):
    """
    Run the system testsuite.
    """
    if not skip_build:
        print("Building pylauncher...")
        build(ctx)

    env = {"PYLAUNCHER_BIN": os.path.join(PYLAUNCHER_BIN_PATH, bin_name("pylauncher"))}
    with ctx.cd("./test/system/python_binding"):
        ctx.run("./test.sh", env=env)
