import os
import sys

from invoke import Exit, task

from .build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from .flavor import AgentFlavor
from .go import deps
from .utils import REPO_PATH, bin_name, get_build_flags
from .windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_PATH = os.path.join(".", "bin", "trace-agent")


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    major_version='7',
    python_runtimes='3',
    arch="x64",
    go_mod="mod",
):
    """
    Build the trace agent.
    """

    flavor = AgentFlavor[flavor]
    ldflags, gcflags, env = get_build_flags(ctx, major_version=major_version, python_runtimes=python_runtimes)

    # generate windows resources
    if sys.platform == 'win32':
        if arch == "x86":
            env["GOARCH"] = "386"

        build_messagetable(ctx, arch=arch)
        vars = versioninfo_vars(ctx, major_version=major_version, python_runtimes=python_runtimes, arch=arch)
        build_rc(
            ctx,
            "cmd/trace-agent/windows/resources/trace-agent.rc",
            arch=arch,
            vars=vars,
            out="cmd/trace-agent/rsrc.syso",
        )

    build_include = (
        get_default_build_tags(
            build="trace-agent",
            flavor=flavor,
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","), arch=arch)
    )
    build_exclude = [] if build_exclude is None else build_exclude.split(",")

    build_tags = get_build_tags(build_include, build_exclude)

    race_opt = "-race" if race else ""
    build_type = "-a" if rebuild else ""
    go_build_tags = " ".join(build_tags)
    agent_bin = os.path.join(BIN_PATH, bin_name("trace-agent"))
    cmd = f"go build -mod={go_mod} {race_opt} {build_type} -tags \"{go_build_tags}\" "
    cmd += f"-o {agent_bin} -gcflags=\"{gcflags}\" -ldflags=\"{ldflags}\" {REPO_PATH}/cmd/trace-agent"

    # go generate only works if you are in the module the target file is in, so we
    # need to move into the pkg/trace module.
    with ctx.cd("./pkg/trace"):
        ctx.run(f"go generate -mod={go_mod} {REPO_PATH}/pkg/trace/info", env=env)
    ctx.run(cmd, env=env)


@task
def integration_tests(ctx, install_deps=False, race=False, go_mod="mod"):
    """
    Run integration tests for trace agent
    """
    if sys.platform == 'win32':
        raise Exit(message='trace-agent integration tests are not supported on Windows', code=0)

    if install_deps:
        deps(ctx)

    go_build_tags = " ".join(get_default_build_tags(build="test"))
    race_opt = "-race" if race else ""

    go_cmd = f'go test -mod={go_mod} {race_opt} -v -tags "{go_build_tags}"'
    ctx.run(f"{go_cmd} ./cmd/trace-agent/test/testsuite/...", env={"INTEGRATION": "yes"})


@task
def benchmarks(ctx, bench, output="./trace-agent.benchmarks.out"):
    """
    Runs the benchmarks. Use "--bench=X" to specify benchmarks to run. Use the "--output=X" argument to specify where to output results.
    """
    if not bench:
        print("Argument --bench=<bench_regex> is required.")
        return
    with ctx.cd("./pkg/trace"):
        ctx.run(f"go test -run=XXX -bench \"{bench}\" -benchmem -count 1 -benchtime 2s ./... | tee {output}")
