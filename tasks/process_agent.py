import datetime
import os
import re
import shutil
import sys

from invoke import task
from subprocess import check_output

from .utils import bin_name, get_build_flags, REPO_PATH, get_version, get_git_branch_name, get_go_version, get_git_commit, get_version_numeric_only
from .build_tags import get_default_build_tags

BIN_DIR = os.path.join(".", "bin", "process-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("process-agent", android=False))
GIMME_ENV_VARS = ['GOROOT', 'PATH']

@task
def build(ctx, race=False, go_version=None, incremental_build=False, puppy=False):
    """
    Build the process agent
    """

    # generate windows resources
    if sys.platform == 'win32':
        ver = get_version_numeric_only(ctx)
        maj_ver, min_ver, patch_ver = ver.split(".")
        resdir = os.path.join(".", "cmd", "process-agent", "windows_resources")

        ctx.run("windmc --target pe-x86-64 -r {resdir} {resdir}/process-agent-msg.mc".format(resdir=resdir))

        ctx.run("windres --define MAJ_VER={maj_ver} --define MIN_VER={min_ver} --define PATCH_VER={patch_ver} -i cmd/process-agent/windows_resources/process-agent.rc --target=pe-x86-64 -O coff -o cmd/process-agent/rsrc.syso".format(
            maj_ver=maj_ver,
            min_ver=min_ver,
            patch_ver=patch_ver
        ))

    # TODO use pkg/version for this
    main = "main."
    ld_vars = {
        "Version": get_version(ctx),
        "GoVersion": get_go_version(),
        "GitBranch": get_git_branch_name(),
        "GitCommit": get_git_commit(),
        "BuildDate": datetime.datetime.now().strftime("%Y-%m-%dT%H:%M:%S"),
    }

    goenv = {}
    # TODO: this is a temporary workaround to avoid the garbage collection issues that the process-agent+go1.11 have had.
    # Once we have upgraded the go version to 1.12, this can be removed (from here and system_probe.py)
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var)+len(env_var)+1:-1].strip('\'\"')
        ld_vars["GoVersion"] = go_version

    ldflags, gcflags, env = get_build_flags(ctx)

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    ldflags += ' '.join(["-X '{name}={value}'".format(name=main+key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(puppy=puppy)

    ## secrets is not supported on windows because the process agent still runs as
    ## root.  No matter what `get_default_build_tags()` returns, take secrets out.
    if sys.platform == 'win32' and "secrets" in build_tags:
        build_tags.remove("secrets")

    # TODO static option
    cmd = 'go build {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/process-agent'

    args = {
        "race_opt": "-race" if race else "",
        "build_type": "-i" if incremental_build else "-a",
        "go_build_tags": " ".join(build_tags),
        "agent_bin": BIN_PATH,
        "gcflags": gcflags,
        "ldflags": ldflags,
        "REPO_PATH": REPO_PATH,
    }

    ctx.run(cmd.format(**args), env=env)


@task
def protobuf(ctx):
    """
    Compile the protobuf files for the process agent
    """

    expected = "libprotoc 3.3.0"

    protoc_version = check_output(["protoc", "--version"]).decode('utf-8').strip()

    if protoc_version != expected:
        raise Exception(
            "invalid version for protoc got '{version}' expected '{expected}'".format(
                version=protoc_version,
                expected=expected,
            )
        )

    cmd = "protoc {proto_dir}/agent.proto -I {gopath}/src -I vendor -I {proto_dir} --gogofaster_out {gopath}/src"
    proto_dir = os.path.join(".", "pkg", "process", "proto")

    ctx.run(cmd.format(gopath=os.environ["GOPATH"], proto_dir=proto_dir))
