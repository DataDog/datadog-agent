import os
import shutil
import sys

from invoke import task
from invoke.exceptions import Exit

from tasks.build_tags import get_default_build_tags
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import REPO_PATH, bin_name, get_version_ldflags

BIN_NAME = "full-host-profiler"
BIN_DIR = os.path.join(".", "bin", "full-host-profiler")
BIN_PATH = os.path.join(BIN_DIR, bin_name("full-host-profiler"))


@task
def build(ctx):
    """
    Build the full host profiler
    """

    if os.path.exists(BIN_PATH):
        os.remove(BIN_PATH)

    env = {"GO111MODULE": "on"}
    build_tags = get_default_build_tags(build="full-host-profiler")
    ldflags = get_version_ldflags(ctx)
    ldflags += "-extldflags='-static'"
    if os.environ.get("DELVE"):
        gcflags = "all=-N -l"
    else:
        gcflags = ""

    # generate windows resources
    if sys.platform == 'win32':
        raise Exit("Windows is not supported for full-host-profiler")

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/host-profiler",
        mod="readonly",
        build_tags=build_tags,
        ldflags=ldflags,
        gcflags=gcflags,
        bin_path=BIN_PATH,
        env=env,
    )

    dist_folder = os.path.join(BIN_DIR, "dist")
    if os.path.exists(dist_folder):
        shutil.rmtree(dist_folder)
    os.mkdir(dist_folder)

    shutil.copy(
        "./cmd/host-profiler/dist/host-profiler-config.yaml",
        os.path.join(dist_folder, "full-host-profiler-config.yaml"),
    )


@task
def update_golden_tests(ctx):
    """
    Update golden test files for host-profiler converters
    """
    print("Updating golden test files...")

    with ctx.cd("comp/host-profiler/collector/impl/converters"):
        ctx.run("go test -tags test -update")

    print("Golden test files updated successfully!")
