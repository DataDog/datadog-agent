import datetime
import os
import time

from invoke import task

from .build_tags import get_default_build_tags
from .go import generate
from .utils import (
    REPO_PATH,
    bin_name,
    get_build_flags,
    get_git_branch_name,
    get_git_commit,
    get_go_version,
    get_gopath,
    get_version,
)

BIN_DIR = os.path.join(".", "bin", "security-agent")
BIN_PATH = os.path.join(BIN_DIR, bin_name("security-agent", android=False))
GIMME_ENV_VARS = ['GOROOT', 'PATH']


def get_go_env(ctx, go_version):
    goenv = {}
    if go_version:
        lines = ctx.run("gimme {version}".format(version=go_version)).stdout.split("\n")
        for line in lines:
            for env_var in GIMME_ENV_VARS:
                if env_var in line:
                    goenv[env_var] = line[line.find(env_var) + len(env_var) + 1 : -1].strip('\'\"')

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]

    return goenv


@task
def build(
    ctx,
    race=False,
    go_version=None,
    incremental_build=False,
    major_version='7',
    arch="x64",
    go_mod="vendor",
    skip_assets=False,
):
    """
    Build the security agent
    """
    ldflags, gcflags, env = get_build_flags(ctx, arch=arch, major_version=major_version, python_runtimes='3')

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
                    goenv[env_var] = line[line.find(env_var) + len(env_var) + 1 : -1].strip('\'\"')
        ld_vars["GoVersion"] = go_version

    # Generating go source from templates by running go generate on ./pkg/status
    generate(ctx)

    # extend PATH from gimme with the one from get_build_flags
    if "PATH" in os.environ and "PATH" in goenv:
        goenv["PATH"] += ":" + os.environ["PATH"]
    env.update(goenv)

    ldflags += ' '.join(["-X '{name}={value}'".format(name=main + key, value=value) for key, value in ld_vars.items()])
    build_tags = get_default_build_tags(
        build="security-agent"
    )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?

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

    if not skip_assets:
        dist_folder = os.path.join(BIN_DIR, "dist", "runtime-security.d")
        if not os.path.exists(dist_folder):
            os.makedirs(dist_folder)


@task()
def gen_mocks(ctx):
    """
    Generate mocks.
    """

    gopath = get_gopath(ctx)
    if not os.path.exists(os.path.join(gopath, "bin/mockery")):
        with ctx.cd(gopath):
            ctx.run("go get -u github.com/vektra/mockery/cmd/mockery", env={'GO111MODULE': 'on'})

    with ctx.cd("./pkg/compliance"):
        ctx.run("./gen_mocks.sh")


@task
def functional_tests(
    ctx,
    race=False,
    verbose=False,
    go_version=None,
    arch="x64",
    major_version='7',
    pattern='',
    output='',
    build_tags='',
    one_by_one=False,
):
    ldflags, gcflags, env = get_build_flags(ctx, arch=arch, major_version=major_version)

    goenv = get_go_env(ctx, go_version)
    env.update(goenv)

    env["CGO_ENABLED"] = "1"
    if arch == "x86":
        env["GOARCH"] = "386"

    cmd = 'go test -tags functionaltests,linux_bpf,{build_tags} {output_opt} {verbose_opt} {run_opt} {repo_path}/pkg/security/tests'
    if os.getuid() != 0 and not output:
        cmd = 'sudo -E PATH={path} ' + cmd

    args = {
        "verbose_opt": "-v" if verbose else "",
        "race_opt": "-race" if race else "",
        "output_opt": "-c -o " + output if output else "",
        "run_opt": "-run " + pattern if pattern else "",
        "build_tags": build_tags,
        "path": os.environ['PATH'],
        "repo_path": REPO_PATH,
    }

    if one_by_one:
        all_tests = ctx.run(
            "grep -e 'func Test' pkg/security/tests/*.go | perl -pe 's|.*func (.*?)\\(.*|\\1|g'"
        ).stdout.split()

        for i, test in enumerate(all_tests):
            args["run_opt"] = "-run ^{test}$".format(test=test)
            ctx.run(cmd.format(**args), env=env)
            if i != len(all_tests) - 1:
                time.sleep(2)

    else:
        ctx.run(cmd.format(**args), env=env)


@task
def docker_functional_tests(ctx, race=False, verbose=False, go_version=None, arch="x64", major_version='7', pattern=''):
    functional_tests(
        ctx,
        race=race,
        verbose=verbose,
        go_version=go_version,
        arch=arch,
        major_version=major_version,
        output="pkg/security/tests/testsuite",
    )

    container_name = 'security-agent-tests'
    capabilities = ['SYS_ADMIN', 'SYS_RESOURCE', 'SYS_PTRACE', 'NET_ADMIN', 'IPC_LOCK', 'ALL']

    cmd = 'docker run --name {container_name} {caps} -d '
    cmd += '-v {GOPATH}/src/{REPO_PATH}/pkg/security/tests:/tests debian:bullseye sleep 3600'

    args = {
        "GOPATH": get_gopath(ctx),
        "REPO_PATH": REPO_PATH,
        "container_name": container_name,
        "caps": ' '.join(['--cap-add ' + cap for cap in capabilities]),
    }

    ctx.run(cmd.format(**args))

    cmd = 'docker exec {container_name} mount -t debugfs none /sys/kernel/debug'
    ctx.run(cmd.format(**args))

    cmd = 'docker exec {container_name} /tests/testsuite {pattern}'
    if verbose:
        cmd += ' -test.v'
    try:
        ctx.run(cmd.format(pattern='-test.run ' + pattern if pattern else '', **args))
    finally:
        cmd = 'docker rm -f {container_name}'
        ctx.run(cmd.format(**args))
