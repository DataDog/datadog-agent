"""
Pylauncher tasks
"""
from __future__ import print_function
import os

from invoke import task

from .build_tags import get_default_build_tags
from .utils import REPO_PATH, bin_name, get_root


#constants
RACE_PROCESS_BIN_PATH = os.path.join(get_root(), "bin", "race-process")

@task
def build(ctx, rebuild=False):
    """
    Build the race-process executable
    """
    build_tags = get_default_build_tags()  # pass all the build flags

    cmd = "go build {build_type} -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/cmd/race-process/"
    args = {
        "build_type": "-a" if rebuild else "-i",
        "build_tags": " ".join(build_tags),
        "bin_name": os.path.join(RACE_PROCESS_BIN_PATH, bin_name("race-process")),
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))
