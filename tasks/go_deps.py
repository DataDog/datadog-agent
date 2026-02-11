import datetime
import os
import shutil
import tempfile
from collections.abc import Iterable

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.go import GOARCH_MAPPING, GOOS_MAPPING
from tasks.go import version as dot_go_version
from tasks.libs.common.color import color_message
from tasks.libs.common.datadog_api import create_gauge, send_metrics
from tasks.libs.common.git import check_uncommitted_changes, get_commit_sha, get_common_ancestor, get_current_branch
from tasks.release import _get_release_json_value

METRIC_GO_DEPS_ALL_NAME = "datadog.agent.go_dependencies.all"
METRIC_GO_DEPS_EXTERNAL_NAME = "datadog.agent.go_dependencies.external"

BINARIES: dict[str, dict] = {
    "agent": {
        "entrypoint": "cmd/agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "iot-agent": {
        "build": "agent",
        "entrypoint": "cmd/iot-agent",
        "flavor": AgentFlavor.iot,
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "heroku-agent": {
        "build": "agent",
        "entrypoint": "cmd/agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "cluster-agent": {"entrypoint": "cmd/cluster-agent", "platforms": ["linux/x64", "linux/arm64"]},
    "cluster-agent-cloudfoundry": {
        "entrypoint": "cmd/cluster-agent-cloudfoundry",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "dogstatsd": {"entrypoint": "cmd/dogstatsd", "platforms": ["linux/x64", "linux/arm64"]},
    "process-agent": {
        "entrypoint": "cmd/process-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "heroku-process-agent": {
        "build": "process-agent",
        "entrypoint": "cmd/process-agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "security-agent": {
        "entrypoint": "cmd/security-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64"],
    },
    "sbomgen": {
        "entrypoint": "cmd/sbomgen",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "system-probe": {
        "entrypoint": "cmd/system-probe",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "cws-instrumentation": {
        "entrypoint": "cmd/cws-instrumentation",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "trace-agent": {
        "entrypoint": "cmd/trace-agent",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "heroku-trace-agent": {
        "build": "trace-agent",
        "entrypoint": "cmd/trace-agent",
        "flavor": AgentFlavor.heroku,
        "platforms": ["linux/x64"],
    },
    "otel-agent": {
        "entrypoint": "cmd/otel-agent",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "full-host-profiler": {
        "entrypoint": "cmd/host-profiler",
        "platforms": ["linux/x64", "linux/arm64"],
    },
    "loader": {
        "entrypoint": "cmd/loader",
        "platforms": ["linux/x64", "linux/arm64", "darwin/x64", "darwin/arm64"],
    },
    "installer": {
        "entrypoint": "cmd/installer",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64"],
    },
    "privateactionrunner": {
        "entrypoint": "cmd/privateactionrunner",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
    "secret-generic-connector": {
        "entrypoint": "cmd/secret-generic-connector",
        "platforms": ["linux/x64", "linux/arm64", "win32/x64", "darwin/x64", "darwin/arm64"],
    },
}

METRIC_GO_DEPS_DIFF = "datadog.agent.go_dependencies.difference"


@task
def diff(
    ctx,
    baseline_ref=None,
    report_file=None,
    report_metrics: bool = False,
    git_ref: str | None = None,
):
    if check_uncommitted_changes(ctx):
        raise Exit(
            color_message(
                "There are uncomitted changes in your repository. Please commit or stash them before trying again.",
                "red",
            ),
            code=1,
        )

    if report_metrics and not os.environ.get("DD_API_KEY"):
        raise Exit(
            code=1,
            message=color_message(
                "DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend", "red"
            ),
        )

    timestamp = int(datetime.datetime.now(datetime.UTC).timestamp())

    current_branch = get_current_branch(ctx)
    commit_sha = os.getenv("CI_COMMIT_SHA")
    if commit_sha is None:
        commit_sha = get_commit_sha(ctx)

    if not baseline_ref:
        base_branch = f'origin/{_get_release_json_value("base_branch")}'
        baseline_ref = get_common_ancestor(ctx, commit_sha, base_branch)

    diffs = {}
    dep_cmd = "go list -buildvcs=false -f '{{ range .Deps }}{{ printf \"%s\\n\" . }}{{end}}'"

    with tempfile.TemporaryDirectory() as tmpdir:
        try:
            # generate list of imports for each target+branch combo
            branches = {"current": None, "main": baseline_ref}
            for branch_name, branch_ref in branches.items():
                if branch_ref:
                    ctx.run(f"git checkout -q {branch_ref}")

                # Run all go list commands in parallel for this branch
                promises = []
                for binary, details in BINARIES.items():
                    with ctx.cd(details.get("entrypoint")):
                        for combo in details["platforms"]:
                            platform, arch = combo.split("/")
                            goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                            target = f"{binary}-{goos}-{goarch}"

                            depsfile = os.path.join(tmpdir, f"{target}-{branch_name}")
                            flavor = details.get("flavor", AgentFlavor.base)
                            build = details.get("build", binary)
                            build_tags = get_default_build_tags(build=build, platform=platform, flavor=flavor)
                            # need to explicitly enable CGO to also include CGO-only deps when checking different platforms
                            env = {
                                "GOOS": goos,
                                "GOARCH": goarch,
                                "CGO_ENABLED": "1",
                                "GOTOOLCHAIN": f"go{dot_go_version(ctx)}",
                            }
                            promise = ctx.run(
                                f"{dep_cmd} -tags \"{' '.join(build_tags)}\" > {depsfile}",
                                env=env,
                                asynchronous=True,
                            )
                            promises.append(promise)

                # Wait for all commands to complete
                for promise in promises:
                    promise.join()
        finally:
            ctx.run(f"git checkout -q {current_branch}")

        # compute diffs for each target
        for binary, details in BINARIES.items():
            for combo in details["platforms"]:
                platform, arch = combo.split("/")
                goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                target = f"{binary}-{goos}-{goarch}"

                prdeps = os.path.join(tmpdir, f"{target}-current")
                maindeps = os.path.join(tmpdir, f"{target}-main")
                # filter out internal packages to avoid noise
                res = ctx.run(
                    f"diff -u0 {maindeps} {prdeps} | grep -v '^@@' | grep -v '^[+-][+-]' | grep -v -E '(^[+-]|/)internal/'",
                    hide=True,
                    warn=True,
                )
                if len(res.stdout) > 0:
                    diffs[target] = res.stdout.strip()

        # output, also to file if requested
        if len(diffs) == 0:
            print("no changes for all binaries")
            if report_file:
                # touch file
                open(report_file, 'w').close()
            return

        pr_comment = [
            f"Baseline: {baseline_ref}",
            f"Comparison: {commit_sha}\n",
            "<table><thead><tr><th>binary</th><th>os</th><th>arch</th><th>change</th></tr></thead><tbody>",
        ]
        for binary, details in BINARIES.items():
            for combo in details["platforms"]:
                flavor = details.get("flavor", AgentFlavor.base)
                build = details.get("build", binary)
                platform, arch = combo.split("/")
                goos, goarch = GOOS_MAPPING.get(platform), GOARCH_MAPPING.get(arch)
                target = f"{binary}-{goos}-{goarch}"
                prettytarget = f"{binary} {goos}/{goarch}"

                if target in diffs:
                    targetdiffs = diffs[target]
                    add, remove = _patch_summary(targetdiffs)

                    if report_metrics:
                        tags = [
                            f"build:{build}",
                            f"flavor:{flavor.name}",
                            f"os:{goos}",
                            f"arch:{goarch}",
                            f"git_sha:{commit_sha}",
                        ]

                        if git_ref:
                            tags.append(f"git_ref:{git_ref}")

                        dependency_diff = create_gauge(METRIC_GO_DEPS_DIFF, timestamp, (add - remove), tags)
                        send_metrics([dependency_diff])

                    color_add = color_message(f"+{add}", "green")
                    color_remove = color_message(f"-{remove}", "red")
                    print(f"== {prettytarget} {color_add}, {color_remove} ==")
                    print(f"{_color_patch(targetdiffs)}\n")

                    summary = f"<summary>+{add}, -{remove}</summary>"
                    diff_block = f"<pre lang='diff'>\n{targetdiffs}\n</pre>"
                    pr_comment.append(
                        f"<tr><td>{binary}</td><td>{goos}</td><td>{goarch}</td><td><details>{summary}\n{diff_block}</details></td></tr>"
                    )
                else:
                    print(f"== {prettytarget} ==\nno changes\n")

        pr_comment.append("</tbody></table>")
        if report_file:
            with open(report_file, 'w') as f:
                f.write("\n".join(pr_comment))


def _color_patch(diff):
    out = []
    for line in diff.splitlines():
        if line.startswith("+"):
            out.append(color_message(line, 'green'))
        elif line.startswith("-"):
            out.append(color_message(line, 'red'))
        else:
            out.append(line)
    return "\n".join(out)


def _patch_summary(diff):
    add_count, remove_count = 0, 0
    for line in diff.splitlines():
        if line.startswith("+"):
            add_count += 1
        elif line.startswith("-"):
            remove_count += 1
    return add_count, remove_count


def compute_count_metric(
    ctx: Context,
    build: str,
    flavor: AgentFlavor,
    platform: str,
    arch: str,
    entrypoint: str,
    timestamp: int | None = None,
    extra_tags: Iterable[str] = (),
):
    """
    Compute a metric representing the number of Go dependencies of the given build/flavor/platform/arch,
    and one with only dependencies outside of the agent repository.
    """

    if not timestamp:
        timestamp = int(datetime.datetime.now(datetime.UTC).timestamp())

    goos, goarch = GOOS_MAPPING[platform], GOARCH_MAPPING[arch]
    build_tags = get_default_build_tags(build=build, flavor=flavor, platform=platform)

    # need to explicitly enable CGO to also include CGO-only deps when checking different platforms
    env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "1"}
    cmd = "go list -buildvcs=false -f '{{ join .Deps \"\\n\"}}'"
    with ctx.cd(entrypoint):
        res = ctx.run(
            f"{cmd} -tags \"{','.join(build_tags)}\"",
            env=env,
            hide='out',  # don't hide errors
        )
        assert res

    deps = res.stdout.strip().split("\n")
    count = len(deps)
    external = sum(1 for dep in deps if not dep.startswith("github.com/DataDog/datadog-agent/"))

    tags = [
        f"build:{build}",
        f"flavor:{flavor.name}",
        f"os:{goos}",
        f"arch:{goarch}",
    ]
    tags.extend(extra_tags)

    metric_count = create_gauge(METRIC_GO_DEPS_ALL_NAME, timestamp, count, tags=tags)
    metric_external = create_gauge(METRIC_GO_DEPS_EXTERNAL_NAME, timestamp, external, tags=tags)
    return metric_count, metric_external


def compute_all_count_metrics(ctx: Context, extra_tags: Iterable[str] = ()):
    """
    Compute metrics representing the number of Go dependencies of every build/flavor/platform/arch.
    """

    timestamp = int(datetime.datetime.now(datetime.UTC).timestamp())

    series = []
    for binary, details in BINARIES.items():
        for combo in details["platforms"]:
            platform, arch = combo.split("/")
            flavor = details.get("flavor", AgentFlavor.base)
            build = details.get("build", binary)
            entrypoint = details["entrypoint"]

            metric_count, metric_external = compute_count_metric(
                ctx, build, flavor, platform, arch, entrypoint, timestamp, extra_tags=extra_tags
            )
            series.append(metric_count)
            series.append(metric_external)

    return series


def compute_binary_dependencies_list(
    ctx: Context,
    build: str,
    flavor: AgentFlavor,
    platform: str,
    arch: str,
) -> list[str]:
    """
    Compute binary import list for the given build/flavor/platform/arch.
    """
    goos, goarch = GOOS_MAPPING[platform], GOARCH_MAPPING[arch]

    build_tags = get_default_build_tags(build=build, flavor=flavor, platform=platform)

    env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "1"}
    cmd = "go list -buildvcs=false -f '{{ join .Deps \"\\n\"}}'"

    res = ctx.run(
        f"{cmd} -tags \"{','.join(build_tags)}\"",
        env=env,
        hide='out',  # don't hide errors
    )
    assert res

    return [dep for dep in res.stdout.strip().splitlines() if not dep.startswith("internal/")]


@task
def send_count_metrics(
    ctx: Context,
    git_sha: str,
    git_ref: str | None = None,
    send_series: bool = True,
):
    if send_series and not os.environ.get("DD_API_KEY"):
        raise Exit(
            code=1,
            message=color_message(
                "DD_API_KEY environment variable not set, cannot send pipeline metrics to the backend", "red"
            ),
        )

    extra_tags = [
        f"git_sha:{git_sha}",
    ]
    if git_ref:
        extra_tags.append(f"git_ref:{git_ref}")

    series = compute_all_count_metrics(ctx, extra_tags=extra_tags)
    print(color_message("Data collected:", "blue"))
    print(series)

    if send_series:
        print(color_message("Sending metrics to Datadog", "blue"))
        send_metrics(series=series)
        print(color_message("Done", "green"))


def key_for_value(map: dict[str, str], value: str) -> str:
    """Return the key from a value in a dictionary."""
    for k, v in map.items():
        if v == value:
            return k
    raise ValueError(f"Unknown value {value}")


@task(
    help={
        'build': f'The agent build to use, one of {", ".join(BINARIES.keys())}',
        'flavor': f'The agent flavor to use, one of {", ".join(AgentFlavor.__members__.keys())}. Defaults to base',
        'os': f'The OS to use, one of {", ".join(GOOS_MAPPING.keys())}. Defaults to host platform',
        'arch': f'The architecture to use, one of {", ".join(GOARCH_MAPPING.keys())}. Defaults to host architecture',
    }
)
def show(ctx: Context, build: str, flavor: str = AgentFlavor.base.name, os: str | None = None, arch: str | None = None):
    """
    Print the Go dependency list for the given agent build/flavor/os/arch.
    """

    if os is None:
        goos = ctx.run("go env GOOS", hide=True)
        assert goos
        os = key_for_value(GOOS_MAPPING, goos.stdout.strip())

    if arch is None:
        goarch = ctx.run("go env GOARCH", hide=True)
        assert goarch
        arch = key_for_value(GOARCH_MAPPING, goarch.stdout.strip())

    entrypoint = BINARIES[build]["entrypoint"]
    with ctx.cd(entrypoint):
        deps = compute_binary_dependencies_list(ctx, build, AgentFlavor[flavor], os, arch)

    for dep in deps:
        print(dep)


@task(
    help={
        'build': f'The agent build to use, one of {", ".join(BINARIES.keys())}',
        'flavor': f'The agent flavor to use, one of {", ".join(AgentFlavor.__members__.keys())}. Defaults to base',
        'os': f'The OS to use, one of {", ".join(GOOS_MAPPING.keys())}. Defaults to host platform',
        'arch': f'The architecture to use, one of {", ".join(GOARCH_MAPPING.keys())}. Defaults to host architecture',
        'entrypoint': 'A Go package path, defaults to the entrypoint of the build.',
        'target': 'A Go package path. If specified, generate a graph from the entrypoint to the target.',
        'std': 'Whether to include the standard library in the graph',
        'cluster': 'Whether to group packages by cluster.',
        'fmt': 'The format of the generated image. Must be accepted by "dot -T". Defaults to "svg".',
        'auto_open': 'Whether to open the generated graph automatically.',
    }
)
def graph(
    ctx: Context,
    build: str,
    flavor: str = AgentFlavor.base.name,
    os: str | None = None,
    arch: str | None = None,
    entrypoint: str | None = None,
    target: str | None = None,
    std: bool = False,
    cluster: bool = False,
    fmt: str = "svg",
    auto_open: bool = True,
):
    """
    Generate a dependency graph of the given build.
    Requires https://github.com/loov/goda and Graphviz's `dot` tools to be in the PATH.

    Usage:
        Dependency graph of the trace-agent on Linux
          dda inv -e go-deps.graph --build trace-agent --os linux
        Reachability graph of the process-agent on Linux to k8s.io/... dependencies
          dda inv -e go-deps.graph --build process-agent --os linux --target k8s.io/...
        Dependency graph of workloadmeta on Linux when using the same build tags as the core-agent
          dda inv -e go-deps.graph --build agent --os linux --entrypoint github.com/DataDog/datadog-agent/comp/core/workloadmeta/...:all
    """
    if shutil.which("goda") is None:
        raise Exit(
            code=1,
            message=color_message(
                "'goda' not found in PATH. Please install it with `go install github.com/loov/goda@latest`", "red"
            ),
        )

    if os is None:
        goos = ctx.run("go env GOOS", hide=True)
        assert goos is not None
        os = goos.stdout.strip()
    assert os in GOOS_MAPPING.values()

    if arch is None:
        goarch = ctx.run("go env GOARCH", hide=True)
        assert goarch is not None
        arch = goarch.stdout.strip()
    assert arch in GOARCH_MAPPING.values()

    stdarg = "-std" if std else ""
    clusterarg = "-cluster" if cluster else ""

    if entrypoint is None:
        binaries_build = "iot-agent" if flavor == AgentFlavor.iot.name else build
        entrypoint = BINARIES[binaries_build]["entrypoint"]
        entrypoint = f"github.com/DataDog/datadog-agent/{entrypoint}:all"

    build_tags = get_default_build_tags(
        build=build, flavor=AgentFlavor[flavor], platform=key_for_value(GOOS_MAPPING, os)
    )
    for tag in build_tags:
        entrypoint = f"{tag}=1({entrypoint})"

    expr = entrypoint if target is None else f"reach({entrypoint}, {target})"

    cmd = f"goda graph {stdarg} {clusterarg} \"{expr}\""

    env = {"GOOS": os, "GOARCH": arch}
    res = ctx.run(cmd, env=env, hide='out')
    assert res

    tmpfile = tempfile.mktemp(prefix="graph-")

    dotfile = tmpfile + ".dot"
    print("Saving dot file in " + dotfile)
    with open(dotfile, "w") as f:
        f.write(res.stdout)

    if shutil.which("dot") is None:
        raise Exit(
            code=1,
            message=color_message(
                "'dot' not found in PATH. Please follow instructions on https://graphviz.org/download/ to install it.",
                "red",
            ),
        )

    fmtfile = tmpfile + "." + fmt
    print(f"Rendering {fmt} in {fmtfile}")
    ctx.run(f"dot -T{fmt} -o {fmtfile} {dotfile}")

    if auto_open:
        print(f"Opening {fmt} file")
        ctx.run(f"open {fmtfile}")
