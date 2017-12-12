"""
Logs tasks
"""
import os
import shutil
from distutils.dir_util import copy_tree

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name
from .utils import REPO_PATH
from .build_tags import get_default_build_tags
from .go import deps

LOGS_BIN_PATH = os.path.join(".", "bin", "logs")
LOGS_BIN_NAME = os.path.join(LOGS_BIN_PATH, bin_name("logs"))
LOGS_DIST_PATH = os.path.join(LOGS_BIN_PATH, "dist")

@task
def build(ctx):
    """
    Build Logs Agent
    """    
    build_tags = get_default_build_tags()
    cmd = "go build -tags \"{build_tags}\" -o {bin_name} {REPO_PATH}/cmd/logs/"
    args = {
        "build_tags": " ".join(build_tags),
        "bin_name": LOGS_BIN_NAME,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))

@task
def run(ctx, skip_build=False, ddconfig=None, ddconfd=None):
    """
    Execute logs-agent binary using ddconfig and ddconfd passed in parameter.
    By default it builds the agent before executing it, unless --skip-build was
    passed.
    """
    if not skip_build:
        build(ctx)

    cmd = "{bin_name} --ddconfig {config_name} --ddconfd {confd_path}"
    args = {
        "bin_name": LOGS_BIN_NAME,
        "config_name": ddconfig,
        "confd_path": ddconfd,
    }
    ctx.run(cmd.format(**args))

@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove logs directory")
    ctx.run("rm -rf {}".format(LOGS_BIN_PATH))
