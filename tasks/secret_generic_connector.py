"""
secret_generic_connector namespaced tasks
"""

import os

from invoke import task

from tasks.libs.common.go import go_build
from tasks.libs.common.utils import bin_name
from tasks.libs.releasing.version import get_version

BINARY_NAME = "secret-generic-connector"
BIN_DIR = os.path.join(".", "bin", "secret-generic-connector")
BIN_PATH = os.path.join(BIN_DIR, bin_name(BINARY_NAME))

FIPS_TAGS = ["goexperiment.systemcrypto", "requirefips"]


@task
def build(
    ctx,
    rebuild=False,
    race=False,
    go_mod="readonly",
    output_bin=None,
    no_strip_binary=False,
    fips_mode=False,
):
    """
    Build the secret-generic-connector binary.
    """

    version = get_version(ctx, include_git=True)

    # ldflags: -s -w to reduce binary size, -s not compatible with FIPS
    # https://github.com/DataDog/datadog-secret-backend/blob/v1/.github/workflows/release.yaml
    ldflags = f"-X main.appVersion={version}"
    if not no_strip_binary:
        if fips_mode:
            ldflags += " -w"
        else:
            ldflags += " -s -w"

    # gcflags: -l disables inlining to reduce binary size
    # https://github.com/DataDog/datadog-secret-backend/blob/v1/.github/workflows/release.yaml
    gcflags = "all=-l"

    # FIPS mode requires CGO for OpenSSL bindings
    # Non-FIPS builds use CGO_ENABLED=0 for static binary
    env = {
        "GO111MODULE": "on",
        "CGO_ENABLED": "1" if fips_mode else "0",
    }

    build_tags = FIPS_TAGS if fips_mode else None

    bin_path = BIN_PATH
    if output_bin:
        bin_path = output_bin

    bin_dir = os.path.dirname(bin_path)
    if bin_dir and not os.path.exists(bin_dir):
        os.makedirs(bin_dir)

    with ctx.cd("cmd/secret-generic-connector"):
        go_build(
            ctx,
            ".",
            mod=go_mod,
            race=race,
            rebuild=rebuild,
            gcflags=gcflags,
            ldflags=ldflags,
            build_tags=build_tags,
            bin_path=os.path.join("..", "..", bin_path),
            env=env,
        )

    print(f"Built secret-generic-connector binary: {bin_path}")


@task
def clean(ctx):
    """
    Remove artifacts for secret-generic-connector
    """
    print("Removing secret-generic-connector binary artifacts")
    ctx.run(f"rm -rf {BIN_DIR}")
