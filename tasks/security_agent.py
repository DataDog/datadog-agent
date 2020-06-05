import datetime
import os
import re
import shutil
import sys
import shutil
import tempfile
from invoke import task
from invoke.exceptions import Exit
from subprocess import check_output

from .utils import bin_name, get_gopath, get_build_flags, REPO_PATH, get_version, get_git_branch_name, get_go_version, get_git_commit, get_version_numeric_only
from .build_tags import get_default_build_tags

BIN_DIR = os.path.join(".", "bin", "security-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("security-agent", android=False))
GIMME_ENV_VARS = ['GOROOT', 'PATH']

@task
def build(ctx, race=False, go_version=None, incremental_build=False,
          major_version='7', arch="x64", go_mod="vendor"):
    """
    Build the security agent
    """
    ldflags, gcflags, env = get_build_flags(ctx, arch=arch, major_version=major_version)


    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx, major_version=major_version),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    goenv = {}
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var)+len(env_var)+1:-1].strip('\'\"')
        ld_vars["GoVersion"] = go_version


    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    ldflags += ' '.join(["-X '{name}={value}'".format(name=main+key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(process=False, arch=arch)

    # TODO static option
    cmd = 'go build -mod={go_mod} {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/security-agent'

    args = {
        "go_mod": go_mod,
        "race_opt": "-race" if race else "",
        "build_type": "" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)

@task()
def gen_mocks(ctx):
    """
    Generate mocks.
    """

    if not os.path.exists(os.path.join(get_gopath(ctx), "bin/mockery")):
        ctx.run("go get github.com/vektra/mockery/.../")

    with ctx.cd("./pkg/compliance"):
        ctx.run("./gen_mocks.sh")
