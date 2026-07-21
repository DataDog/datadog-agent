import os

from invoke.tasks import task

from tasks.devcontainer import run_on_devcontainer
from tasks.libs.common.go import go_build
from tasks.libs.common.utils import get_build_flags

BIN_DIR = os.path.join(".", "bin", "rshell")
BIN_PATH = os.path.join(BIN_DIR, "rshell")


@task
@run_on_devcontainer
def build(ctx, install_path=None, rebuild=False, go_mod="readonly"):
    """Build the standalone rshell binary used by the privileged helper."""
    ldflags, gcflags, env = get_build_flags(ctx, install_path=install_path)
    # The helper changes Linux credentials with Go's all-thread syscall. Go
    # deliberately disables that primitive in cgo binaries because it cannot
    # update threads created outside the runtime. Enforce a pure-Go binary so
    # the initial privilege drop and each narrowly scoped elevation apply to
    # every thread or fail closed.
    env["CGO_ENABLED"] = "0"
    go_build(
        ctx,
        "github.com/DataDog/rshell/cmd/rshell",
        ldflags=ldflags,
        gcflags=gcflags,
        rebuild=rebuild,
        env=env,
        bin_path=BIN_PATH,
        mod=go_mod,
    )
