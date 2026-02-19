"""
secret_generic_connector namespaced tasks
"""

import os

from invoke import task

from tasks.build_tags import get_default_build_tags
from tasks.flavor import AgentFlavor
from tasks.libs.common.constants import REPO_PATH
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
    strip_binary=True,
    fips_mode=False,
):
    """
    Build the secret-generic-connector binary.
    """

    version = get_version(ctx, include_git=True)

    # ldflags: -s -w to reduce binary size, -s not compatible with FIPS
    # https://github.com/DataDog/datadog-secret-backend/blob/v1/.github/workflows/release.yaml
    ldflags = f"-X main.appVersion={version}"
    if strip_binary:
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

    build_tags = get_default_build_tags(
        build="secret-generic-connector", flavor=AgentFlavor.fips if fips_mode else AgentFlavor.base
    )
    bin_path = output_bin or BIN_PATH

    go_build(
        ctx,
        f"{REPO_PATH}/cmd/secret-generic-connector",
        mod=go_mod,
        race=race,
        rebuild=rebuild,
        gcflags=gcflags,
        ldflags=ldflags,
        build_tags=build_tags,
        bin_path=bin_path,
        env=env,
        check_deadcode=os.getenv("DEPLOY_AGENT") == "true",
    )


@task
def clean(ctx):
    """
    Remove artifacts for secret-generic-connector
    """
    print("Removing secret-generic-connector binary artifacts")
    ctx.run(f"rm -rf {BIN_DIR}")
