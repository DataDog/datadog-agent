import os

from invoke.tasks import task

from tasks.build_tags import get_default_build_tags
from tasks.devcontainer import run_on_devcontainer
from tasks.flavor import AgentFlavor
from tasks.libs.build.bazel import build_binary_with_bazel
from tasks.libs.common.constants import REPO_PATH
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name, get_build_flags

BIN_DIR = os.path.join(".", "bin", "trace-loader")
BIN_PATH = os.path.join(BIN_DIR, bin_name("trace-loader"))


@task
@run_on_devcontainer
def build(
    ctx,
    install_path=None,
    go_mod="readonly",
    enable_bazel=False,
):
    """Build the loader binary.

    enable_bazel: build via `bazel build //cmd/loader:loader` instead of `go build`,
    then copy the result to the same place. Developer opt-in only; defaults to off.
    """
    if enable_bazel:
        if install_path is not None:
            raise NotImplementedError("--enable-bazel does not support --install-path.")
        build_binary_with_bazel("//cmd/loader:loader", bin_path=BIN_PATH)
        return

    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path)
    build_tags = get_default_build_tags(build="loader", flavor=AgentFlavor.base)
    go_build(
        ctx,
        f"{REPO_PATH}/cmd/loader",
        build_tags=build_tags,
        ldflags=ldflags,
        gcflags=gcflags,
        env=env,
        bin_path=BIN_PATH,
        mod=go_mod,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
    )
