"""
Logs tasks
"""
import os

import invoke
from invoke import task
from invoke.exceptions import Exit

from .utils import bin_name
from .utils import REPO_PATH

AGENT_BIN_PATH = os.path.join(".", "bin", "agent")
LOGS_BIN_NAME = os.path.join(AGENT_BIN_PATH, bin_name("logs-agent"))

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

@task
def run(ctx, skip_build=False):
    """
    Execute logs-agent binary.

    By default it builds the agent before executing it, unless --skip-build was
    passed.
    """
    if not skip_build:
        build(ctx)

    target = LOGS_BIN_NAME
    ctx.run("{} start".format(target))

@task
def clean(ctx):
    """
    Remove temporary objects and binary artifacts
    """
    # go clean
    print("Executing go clean")
    ctx.run("go clean")

    # remove the bin/agent folder
    print("Remove logs-agent binary")
    cmd = "rm {bin_name}"
    args = {
        "bin_name": LOGS_BIN_NAME,        
    }
    ctx.run(cmd.format(**args))
