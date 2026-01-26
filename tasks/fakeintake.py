"""
Build or use the fake intake client CLI
"""

from invoke import task

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import gitlab_section
from tasks.rust_compression import build as rust_compression_build


@task
def build(ctx, exclude_rust_compression=False):
    """
    Build the fake intake
    """
    if not exclude_rust_compression:
        with gitlab_section("Build Rust compression library", collapsed=True):
            rust_compression_build(ctx, release=True)

    with ctx.cd("test/fakeintake"):
        go_build(ctx, "cmd/server/main.go", bin_path="build/fakeintake")
        go_build(ctx, "cmd/client/main.go", bin_path="build/fakeintakectl")


@task
def test(ctx, exclude_rust_compression=False):
    """
    Run the fake intake tests
    """
    if not exclude_rust_compression:
        with gitlab_section("Build Rust compression library", collapsed=True):
            rust_compression_build(ctx, release=True)

    with ctx.cd("test/fakeintake"):
        ctx.run("go test ./...")
