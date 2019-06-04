import datetime
import os
import shutil
import sys

from invoke import task
from subprocess import check_output

from .utils import bin_name, get_build_flags, REPO_PATH, get_version, get_git_branch_name, get_go_version, get_git_commit, get_version_numeric_only
from .build_tags import get_default_build_tags

BIN_DIR = os.path.join(".", "bin", "process-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("process-agent", android=False))

@task
def build(ctx, race=False, go110=False, incremental_build=False, puppy=False):
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

    gobin = 'go'
    # Force using go1.10
    if go110:
        version = '1.10.1'
        lines = ctx.run("gimme {version}".format(version=version)).stdout.split("\n")
        # Parse the goroot
        line = next(line for line in lines if "GOROOT" in line)
        root = line.split("=")[-1].split("'")[-2]

        gobin = os.path.join(root, "bin", "go")
        ld_vars["GoVersion"] = version

    ldflags, gcflags, env = get_build_flags(ctx)

    ldflags += ' '.join(["-X '{name}={value}'".format(name=main+key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(puppy=puppy)

    # TODO static option
    cmd = '{gobin} build {race_opt} {build_type} -tags "{go_build_tags}" '
    cmd += '-o {agent_bin} -gcflags="{gcflags}" -ldflags="{ldflags}" {REPO_PATH}/cmd/process-agent'

    args = {
        "gobin": gobin,
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
