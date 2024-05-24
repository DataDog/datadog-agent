import datetime
import os
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
    tags = [
        f"build:{build}",
        f"flavor:{flavor.name}",
        f"os:{goos}",
        f"arch:{goarch}",
    ]
    tags.extend(extra_tags)

    build_tags = get_default_build_tags(build=build, flavor=flavor, platform=platform)

    env = {"GOOS": goos, "GOARCH": goarch}
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

    metric_count = create_gauge("datadog.agent.go_dependencies.all", timestamp, count, tags=tags)
    metric_external = create_gauge("datadog.agent.go_dependencies.external", timestamp, external, tags=tags)
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
                        f"File {filename} does not exist. To execute the dependencies list check for the {binary} binary, please run the task `inv -e go-deps.generate --binaries {binary}"
                    )
                    continue

                deps_file = open(filename)
                deps = deps_file.read()
                deps_file.close()

                build_tags = get_default_build_tags(build=build, flavor=flavor, platform=platform)

                env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "1"}
                cmd = "go list -f '{{ join .Deps \"\\n\"}}'"

                res = ctx.run(
                    f"{cmd} -tags {','.join(build_tags)}",
                    env=env,
                    hide='out',  # don't hide errors
                )
                assert res

                if res.stdout.strip() != deps:
                    mismatch_binaries.add(binary)

    if len(mismatch_binaries) > 0:
        raise Exit(
            code=1,
            message=color_message(
                f"Dependencies list for {list(mismatch_binaries)} does not match. To fix this check, please run `inv -e go-deps.generate --binaries {','.join(mismatch_binaries)}`",
                "red",
            ),
        )


@task
def generate(
    ctx: Context,
    binaries: str,
):
    for binary in binaries.split(','):
        binary_info = BINARIES.get(binary, None)
        if not binary_info:
            raise Exit(
                code=1,
                message=color_message(
                    f"Binary `{binary}` is not valid. Valid binaries are {list(BINARIES.keys())}", "red"
                ),
            )

        entrypoint = binary_info["entrypoint"]
        platforms = binary_info["platforms"]
        flavor = binary_info.get("flavor", AgentFlavor.base)
        build = binary_info.get("build", binary)

        with ctx.cd(entrypoint):
            for platform in platforms:
                platform, arch = platform.split("/")

                goos, goarch = GOOS_MAPPING[platform], GOARCH_MAPPING[arch]

                filename = os.path.join(ctx.cwd, f"dependencies_{goos}_{goarch}.txt")

                build_tags = get_default_build_tags(build=build, flavor=flavor, platform=platform)

                env = {"GOOS": goos, "GOARCH": goarch, "CGO_ENABLED": "1"}
                cmd = "go list -f '{{ join .Deps \"\\n\"}}'"

                res = ctx.run(
                    f"{cmd} -tags {','.join(build_tags)}",
                    env=env,
                    hide='out',  # don't hide errors
                )
                assert res

                f = open(filename, "w")
                f.write(res.stdout.strip())
                f.close()
