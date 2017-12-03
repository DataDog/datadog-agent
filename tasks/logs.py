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

LOGS_BIN_PATH = os.path.join(".", "bin", "logs")
LOGS_BIN_NAME = os.path.join(LOGS_BIN_PATH, bin_name("logs"))
LOGS_DIST_PATH = os.path.join(LOGS_BIN_PATH, "dist")

@task
def build(ctx):
    """
    Build Logs Agent
    """    
    cmd = "go build -tags=docker -o {bin_name} {REPO_PATH}/cmd/logs/"
    args = {
        "bin_name": LOGS_BIN_NAME,
        "REPO_PATH": REPO_PATH,
    }
    ctx.run(cmd.format(**args))

    cmd = "go generate {}/cmd/logs"
    ctx.run(cmd.format(REPO_PATH))

    refresh_assets(ctx)

@task
def refresh_assets(ctx):
    """
    Clean up and refresh Logs' assets and config files
    """
    # ensure LOGS_BIN_PATH exists
    if not os.path.exists(LOGS_BIN_PATH):
        os.mkdir(LOGS_BIN_PATH)

    dist_folder = os.path.join(LOGS_BIN_PATH, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    copy_tree("./cmd/logs/dist/", dist_folder)

@task
def run(ctx, skip_build=False, ddconfig=None, ddconfd=None):
    """
    Execute logs-agent binary using default ddconfig and ddconfd if not set.
    By default it builds the agent before executing it, unless --skip-build was
    passed.
    """
    if not skip_build:
        build(ctx)

    if ddconfig is None:
        ddconfig = os.path.join(LOGS_DIST_PATH, "datadog.yaml")

    if ddconfd is None:
        ddconfd = os.path.join(LOGS_DIST_PATH, "conf.d")

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
