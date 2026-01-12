import os
import sys

from invoke import task

from tasks.build_tags import (
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor
from tasks.gointegrationtest import TRACE_AGENT_IT_CONF, containerized_integration_tests
from tasks.libs.common.go import go_build
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
    go_mod="readonly",
):
    """
    Build the trace agent.
    """

    flavor = AgentFlavor[flavor]

    ldflags, gcflags, env = get_build_flags(
        ctx,
        install_path=install_path,
    )

    # generate windows resources
    if sys.platform == 'win32':
        build_messagetable(ctx)
        vars = versioninfo_vars(ctx)
        build_rc(
            ctx,
            "cmd/trace-agent/windows/resources/trace-agent.rc",
            vars=vars,
            out="cmd/trace-agent/rsrc.syso",
        )

    build_tags = compute_build_tags_for_flavor(
        build="trace-agent", flavor=flavor, build_include=build_include, build_exclude=build_exclude
    )
    agent_bin = os.path.join(BIN_PATH, bin_name("trace-agent"))

    # go generate only works if you are in the module the target file is in, so we
    # need to move into the pkg/trace module.
    with ctx.cd("./pkg/trace"):
        ctx.run(f"go generate -mod={go_mod} {REPO_PATH}/pkg/trace/info", env=env)
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/trace-agent",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        build_tags=build_tags,
        bin_path=agent_bin,
        ldflags=ldflags,
        gcflags=gcflags,
        env=env,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
        coverage=os.getenv("E2E_COVERAGE_PIPELINE") == "true",
    )


@task
def integration_tests(ctx, race=False, go_mod="readonly", timeout="10m"):
    """
    Run integration tests for trace agent
    """
    containerized_integration_tests(
        ctx,
        TRACE_AGENT_IT_CONF,
        race=race,
        go_mod=go_mod,
        timeout=timeout,
    )


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
