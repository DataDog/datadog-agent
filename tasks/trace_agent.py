import json
import os
import sys
import tempfile

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import (
    compute_build_tags_for_flavor,
)
from tasks.flavor import AgentFlavor
from tasks.git import get_ancestor
from tasks.gointegrationtest import TRACE_AGENT_IT_CONF, containerized_integration_tests
from tasks.libs.build.bazel import bazel, build_binary_with_bazel
from tasks.libs.common.color import color_message
from tasks.libs.common.git import get_commit_sha, get_current_branch
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_build_flags
from tasks.libs.trace_dupcheck import CORPUS_INPUTS, DUPCHECK_DIR, render_diff, render_without_baseline
from tasks.schema.generate import schema_codegen
from tasks.windows_resources import build_messagetable, build_rc, versioninfo_vars

BIN_PATH = os.path.join(".", "bin", "trace-agent")

BAZEL_TARGET = "//cmd/trace-agent:trace-agent"


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
    enable_bazel=False,
):
    """
    Build the trace agent.
    """

    flavor = AgentFlavor[flavor]

    if enable_bazel:
        if race:
            raise NotImplementedError("--enable-bazel does not support --race.")
        if build_include is not None or build_exclude is not None:
            raise NotImplementedError("--enable-bazel does not support --build-include/--build-exclude.")
        if flavor != AgentFlavor.base:
            raise NotImplementedError(f"--enable-bazel does not support flavor={flavor.name}.")
        if install_path is not None:
            raise NotImplementedError("--enable-bazel does not support --install-path.")

        agent_bin = os.path.join(BIN_PATH, bin_name("trace-agent"))
        build_binary_with_bazel(BAZEL_TARGET, bin_path=agent_bin)
        return

    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

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
    # TODO: remove once Bazel is used to build the Agent
    schema_codegen(ctx)

    # The `test` build tag wires pkg/util/log to write to stdout (pkg/util/log/log_test_init.go),
    # defaulting to the `debug` level. Those lines interleave with the `go test -bench` result
    # lines and make the output unparseable by benchmark tooling, so silence the logger unless
    # the caller explicitly asked for a level.
    env = os.environ.copy()
    env.setdefault("DD_LOG_LEVEL", "off")

    with ctx.cd("./pkg/trace"):
        ctx.run(
            f"go test -tags=test -run=XXX -bench \"{bench}\" -benchmem -count 1 -benchtime 2s ./... | tee {output}",
            env=env,
        )


DUPCHECK_TARGET = f"//{DUPCHECK_DIR}:dupcheck_test"


@task
def dupcheck_diff(ctx, report_file="", baseline_ref=""):
    """
    Compares the trace payload duplicates report of HEAD with the one of its merge base.

    The Markdown comparison is printed and, if --report-file is set, written to that file, which is left empty when
    the reports have the same issues, or when the merge base has no checker and HEAD doesn't change the checker.
    The task fails, writing no file, when the report fails on HEAD or on a merge base that has the checker, so that
    the comment of a previous run isn't replaced by an incomplete comparison. --baseline-ref overrides the merge base.
    The merge base is checked out to run its report, so tracked files must have no local change.
    """
    if not ctx.run("git diff --quiet HEAD", warn=True, hide=True).ok:
        raise Exit("dupcheck-diff checks out the merge base: commit or stash your changes first", code=1)

    head_sha = get_commit_sha(ctx)
    if not baseline_ref:
        baseline_ref = get_ancestor(ctx, os.environ.get("CI_COMMIT_REF_NAME") or get_current_branch(ctx))
    baseline_ref = get_commit_sha(ctx, commit=baseline_ref)

    with tempfile.TemporaryDirectory() as tmpdir:
        head = _dupcheck_report(os.path.join(tmpdir, "head.json"))
        if head is None:
            raise Exit("the dupcheck report failed on HEAD", code=1)

        base = None
        if ctx.run(f"git cat-file -e {baseline_ref}:{DUPCHECK_DIR}/BUILD.bazel", warn=True, hide=True).ok:
            # Restore the branch rather than the commit, so that a local run doesn't end on a detached HEAD.
            current = ctx.run("git symbolic-ref -q --short HEAD", warn=True, hide=True).stdout.strip() or head_sha
            try:
                ctx.run(f"git checkout -q {baseline_ref}")
                base = _dupcheck_report(os.path.join(tmpdir, "base.json"))
            finally:
                ctx.run(f"git checkout -q {current}")
            if base is None:
                raise Exit(f"the dupcheck report failed on the merge base {baseline_ref}", code=1)
        changed_inputs = [
            path
            for path in CORPUS_INPUTS
            if not ctx.run(f"git diff --quiet {baseline_ref} {head_sha} -- {path}", warn=True, hide=True).ok
        ]
        if base is None:
            print(color_message(f"The merge base {baseline_ref} has no dupcheck checker", "orange"))
            # Without a baseline, only a PR changing the checker gets its report: other PRs whose merge base
            # predates the checker would get a comment unrelated to their change.
            body = render_without_baseline(head, baseline_ref) if DUPCHECK_DIR in changed_inputs else ""
        else:
            body = render_diff(base, head, baseline_ref, changed_inputs)

    print(body or f"No trace payload duplicates change to report against the merge base {baseline_ref}")
    if report_file:
        with open(report_file, "w") as f:
            f.write(body)


def _dupcheck_report(path):
    """Runs the dupcheck report of the checked out commit and returns its JSON summary, or None if it failed."""
    result = bazel(
        "run",
        DUPCHECK_TARGET,
        "--",
        "-test.run=^TestNoAgentIntroducedDuplicates$",
        f"-dupcheck.json={path}",
        ignore_errors=True,
    )
    if result.returncode != 0 or not os.path.exists(path):
        return None
    with open(path) as f:
        return json.load(f)
