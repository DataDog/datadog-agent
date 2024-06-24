import datetime
import io
import os
from collections import namedtuple
from collections.abc import Iterable

from invoke.context import Context
from invoke.exceptions import Exit
from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.diff import BINARIES
from tasks.flavor import AgentFlavor
from tasks.go import GOARCH_MAPPING, GOOS_MAPPING
from tasks.libs.common.color import color_message
from tasks.libs.common.datadog_api import create_gauge, send_metrics

METRIC_GO_DEPS_ALL_NAME = "datadog.agent.go_dependencies.all"
METRIC_GO_DEPS_EXTERNAL_NAME = "datadog.agent.go_dependencies.external"


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
    cmd = "go list -f '{{ join .Deps \"\\n\"}}'"
    with ctx.cd(entrypoint):
        res = ctx.run(
            f"{cmd} -tags {','.join(build_tags)}",
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
    cmd = "go list -f '{{ join .Deps \"\\n\"}}'"

    res = ctx.run(
        f"{cmd} -tags {','.join(build_tags)}",
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


BINARY_TO_TEST = ["serverless"]
MisMacthBinary = namedtuple('failedBinary', ['binary', 'os', 'arch', 'differences'])


@task
def test_list(
    ctx: Context,
):
    """
    Compare the dependencies list for the binaries in BINARY_TO_TEST with the actual dependencies of the binaries.
    If the lists do not match, the task will raise an error.
    """
    mismatch_binaries = set()

    for binary in BINARY_TO_TEST:
        binary_info = BINARIES[binary]
        entrypoint = binary_info["entrypoint"]
        platforms = binary_info["platforms"]
        flavor = binary_info.get("flavor", AgentFlavor.base)
        build = binary_info.get("build", binary)

        with ctx.cd(entrypoint):
            for platform in platforms:
                platform, arch = platform.split("/")

                goos, goarch = GOOS_MAPPING[platform], GOARCH_MAPPING[arch]

                filename = os.path.join(ctx.cwd, f"dependencies_{goos}_{goarch}.txt")
                if not os.path.isfile(filename):
                    print(
                        f"File {filename} does not exist. To execute the dependencies list check for the {binary} binary, please run the task `inv -e go-deps.generate"
                    )
                    continue

                deps_file = open(filename)
                deps = deps_file.read().strip().splitlines()
                deps_file.close()

                list = compute_binary_dependencies_list(ctx, build, flavor, platform, arch)

                if list != deps:
                    new_dependencies_lines = len(list)
                    recorded_dependencies_lines = len(deps)

                    mismatch_binaries.add(
                        MisMacthBinary(binary, goos, goarch, new_dependencies_lines - recorded_dependencies_lines)
                    )

    if len(mismatch_binaries) > 0:
        message = io.StringIO()

        for mismatch_binary in mismatch_binaries:
            if mismatch_binary.differences > 0:
                message.write(
                    color_message(
                        f"You added some dependencies to {mismatch_binary.binary} ({mismatch_binary.os}/{mismatch_binary.arch}). Adding new dependencies to the binary increases its size. Do we really need to add this dependency?\n",
                        "red",
                    )
                )
            else:
                message.write(
                    color_message(
                        f"You removed some dependencies from {mismatch_binary.binary} ({mismatch_binary.os}/{mismatch_binary.arch}). Congratulations!\n",
                        "green",
                    )
                )

        message.write(
            color_message(
                "To fix this check, please run `inv -e go-deps.generate`",
                "orange",
            )
        )

        raise Exit(
            code=1,
            message=message.getvalue(),
        )


@task
def generate(
    ctx: Context,
):
    for binary in BINARY_TO_TEST:
        binary_info = BINARIES[binary]
        entrypoint = binary_info["entrypoint"]
        platforms = binary_info["platforms"]
        flavor = binary_info.get("flavor", AgentFlavor.base)
        build = binary_info.get("build", binary)

        with ctx.cd(entrypoint):
            for platform in platforms:
                platform, arch = platform.split("/")

                goos, goarch = GOOS_MAPPING[platform], GOARCH_MAPPING[arch]

                filename = os.path.join(ctx.cwd, f"dependencies_{goos}_{goarch}.txt")

                list = compute_binary_dependencies_list(ctx, build, flavor, platform, arch)

                f = open(filename, "w")
                f.write('\n'.join(list))
                f.close()


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
