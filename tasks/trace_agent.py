import os
import sys

from invoke import Exit, task

from tasks.agent import build as agent_build
from tasks.build_tags import filter_incompatible_tags, get_build_tags, get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_PATH = os.path.join(".", "bin", "trace-agent")


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    build_include=None,
    build_exclude=None,
    flavor=AgentFlavor.base.name,
    install_path=None,
    major_version='7',
    go_mod="mod",
    bundle=False,
):
    """
    Build the trace agent.
    """

    if bundle:
        return agent_build(
            ctx,
            race=race,
            build_include=build_include,
            build_exclude=build_exclude,
            flavor=flavor,
            major_version=major_version,
            go_mod=go_mod,
        )

    flavor = AgentFlavor[flavor]
    if flavor.is_ot():
        flavor = AgentFlavor.base

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
        major_version=major_version,
    )

    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx, major_version=major_version)
        build_rc(
            ctx,
            "cmd/trace-agent/windows/resources/trace-agent.rc",
            vars=vars,
            out="cmd/trace-agent/rsrc.syso",
        )

    build_include = (
        get_default_build_tags(
            build="trace-agent",
            flavor=flavor,
        )  # TODO/FIXME: Arch not passed to preserve build tags. Should this be fixed?
        if build_include is None
        else filter_incompatible_tags(build_include.split(","))
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
def integration_tests(ctx, race=False, go_mod="mod", timeout="10m"):
    """
    Run integration tests for trace agent
    """
    if sys.platform == 'win32':
        raise Exit(message='trace-agent integration tests are not supported on Windows', code=0)

    go_build_tags = " ".join(get_default_build_tags(build="test"))
    race_opt = "-race" if race else ""
    timeout_opt = f"-timeout {timeout}" if timeout else ""

    go_cmd = f'go test {timeout_opt} -mod={go_mod} {race_opt} -v -tags "{go_build_tags}"'
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
        ctx.run(f"go test -tags=test -run=XXX -bench \"{bench}\" -benchmem -count 1 -benchtime 2s ./... | tee {output}")
